#!/usr/bin/env python3
"""Loopback BGE-small embed HTTP. WordPiece from tokenizer.json — not character ords."""
import argparse, json, os, re, sys, unicodedata
from http.server import BaseHTTPRequestHandler, HTTPServer

try:
    import numpy as np
    import onnxruntime as ort
except Exception as exc:
    sys.stderr.write(
        "so embed worker: install numpy and onnxruntime in this Python (%s)\n" % exc
    )
    sys.exit(1)

MAX_LEN = 512
QUERY_PREFIX = "Represent this sentence for searching relevant passages: "


def load_tokenizer(path):
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
    vocab = data.get("model", {}).get("vocab") or {}
    unk = data.get("model", {}).get("unk_token") or "[UNK]"
    return vocab, unk


def basic_tokenize(text):
    text = unicodedata.normalize("NFD", (text or "").lower())
    text = "".join(ch for ch in text if unicodedata.category(ch) != "Mn")
    parts = re.findall(r"\w+|[^\w\s]", text, flags=re.UNICODE)
    return [p for p in parts if p.strip()]


def wordpiece(token, vocab, unk):
    if token in vocab:
        return [token]
    chars = list(token)
    if len(chars) > 100:
        return [unk]
    start = 0
    sub = []
    while start < len(chars):
        end = len(chars)
        cur = None
        while start < end:
            substr = "".join(chars[start:end])
            if start > 0:
                substr = "##" + substr
            if substr in vocab:
                cur = substr
                break
            end -= 1
        if cur is None:
            return [unk]
        sub.append(cur)
        start = end
    return sub


def encode(text, vocab, unk):
    cls_id = int(vocab.get("[CLS]", 101))
    sep_id = int(vocab.get("[SEP]", 102))
    unk_id = int(vocab.get(unk, vocab.get("[UNK]", 100)))
    ids = [cls_id]
    for tok in basic_tokenize(text):
        for wp in wordpiece(tok, vocab, unk):
            ids.append(int(vocab.get(wp, unk_id)))
            if len(ids) >= MAX_LEN - 1:
                break
        if len(ids) >= MAX_LEN - 1:
            break
    ids.append(sep_id)
    ids = ids[:MAX_LEN]
    mask = [1] * len(ids)
    while len(ids) < MAX_LEN:
        ids.append(0)
        mask.append(0)
    return ids, mask


def mean_pool(last, mask):
    mask = np.expand_dims(mask, -1)
    return (last * mask).sum(axis=1) / np.clip(mask.sum(axis=1), 1e-9, None)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        return

    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"ok":true,"model":"bge"}')

    def do_POST(self):
        n = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(n) or b"{}")
        texts = body.get("texts") or []
        prefix = ""
        if (body.get("input_type") or "document") == "query":
            prefix = QUERY_PREFIX
        vecs = []
        for t in texts:
            ids, mask = encode(prefix + (t or ""), VOCAB, UNK)
            ids_a = np.array([ids], dtype=np.int64)
            mask_a = np.array([mask], dtype=np.int64)
            feeds = {}
            for inp in sess.get_inputs():
                name = inp.name
                low = name.lower()
                if "mask" in low:
                    feeds[name] = mask_a
                elif "type" in low:
                    feeds[name] = np.zeros_like(ids_a)
                else:
                    feeds[name] = ids_a
            out = sess.run(None, feeds)[0]
            pooled = mean_pool(out, mask_a)[0]
            nrm = np.linalg.norm(pooled) or 1.0
            vecs.append((pooled / nrm).astype(float).tolist()[:384])
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps({"vectors": vecs, "model": "bge"}).encode())


if __name__ == "__main__":
    p = argparse.ArgumentParser()
    p.add_argument("--listen", required=True)
    p.add_argument("--model-dir", required=True)
    args = p.parse_args()
    VOCAB, UNK = load_tokenizer(args.model_dir + "/tokenizer.json")
    onnx_path = os.environ.get("SO_BGE_ONNX") or (args.model_dir + "/model.onnx")
    if not os.path.exists(onnx_path):
        alt = args.model_dir + "/model_quantized.onnx"
        if os.path.exists(alt):
            onnx_path = alt
    sess = ort.InferenceSession(onnx_path, providers=["CPUExecutionProvider"])
    host, port = args.listen.rsplit(":", 1)
    HTTPServer((host, int(port)), Handler).serve_forever()
