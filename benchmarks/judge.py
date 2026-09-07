"""LLM fact judge for memory QA. Sees (question, gold, answer) only.

Never receives retrieved context, so it cannot reward retrieval. Temperature 0.
The strict substring grader is always reported beside this verdict.
"""

from __future__ import annotations

import hashlib
import json
import os
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

from spend import SpendLedger

JUDGE_MODEL = os.environ.get("SUPEROPEN_JUDGE_MODEL", "claude-haiku-4-5-20251001")
ANTHROPIC_URL = "https://api.anthropic.com/v1/messages"

SYSTEM = (
    "You judge whether an assistant answer contains the gold fact. "
    "Reply with JSON only: {\"hit\": true} or {\"hit\": false}. "
    "hit is true when the gold fact is present, including reasonable paraphrases "
    "(for example 7 May 2023 vs May 7, 2023, or a nickname vs a full name). "
    "hit is false when the answer is a refusal, says it has no context, or states a different fact."
)


def cache_key(item_id: str, answer: str) -> str:
    digest = hashlib.sha256(answer.encode("utf-8", errors="replace")).hexdigest()[:16]
    safe = "".join(ch if ch.isalnum() or ch in "-_." else "_" for ch in str(item_id))
    return f"{safe}-{digest}.json"


def parse_verdict(text: str) -> bool | None:
    raw = (text or "").strip()
    if not raw:
        return None
    if raw.startswith("```"):
        raw = raw.strip("`")
        if raw.lower().startswith("json"):
            raw = raw[4:].strip()
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError:
        lower = raw.lower()
        if '"hit": true' in lower or lower == "true":
            return True
        if '"hit": false' in lower or lower == "false":
            return False
        return None
    if isinstance(payload, dict) and "hit" in payload:
        return bool(payload["hit"])
    return None


def judge(
    question: str,
    gold: str,
    answer: str,
    item_id: str,
    ledger: SpendLedger,
    cache_dir: Path,
) -> dict[str, Any]:
    """Return {hit, usd, cached, skipped} for one (question, gold, answer)."""
    out: dict[str, Any] = {"hit": None, "usd": 0.0, "cached": False, "skipped": None}
    if not gold or not answer:
        out["skipped"] = "empty"
        return out
    cache_dir.mkdir(parents=True, exist_ok=True)
    path = cache_dir / cache_key(item_id, answer)
    if path.is_file():
        try:
            cached = json.loads(path.read_text())
        except json.JSONDecodeError:
            cached = {}
        if "hit" in cached:
            out["hit"] = bool(cached["hit"])
            out["cached"] = True
            return out

    key = os.environ.get("ANTHROPIC_API_KEY") or ""
    if not key:
        out["skipped"] = "no_api_key"
        return out

    user = (
        f"Question: {question}\n"
        f"Gold fact: {gold}\n"
        f"Assistant answer: {answer}\n"
    )
    body = json.dumps(
        {
            "model": JUDGE_MODEL,
            "max_tokens": 64,
            "temperature": 0,
            "system": SYSTEM,
            "messages": [{"role": "user", "content": user}],
        }
    ).encode()
    req = urllib.request.Request(
        ANTHROPIC_URL,
        data=body,
        headers={
            "content-type": "application/json",
            "x-api-key": key,
            "anthropic-version": "2023-06-01",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            payload = json.loads(resp.read().decode())
    except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as exc:
        out["skipped"] = str(exc)
        return out

    text_parts: list[str] = []
    for block in payload.get("content") or []:
        if isinstance(block, dict) and block.get("type") == "text":
            text_parts.append(str(block.get("text") or ""))
    hit = parse_verdict("\n".join(text_parts))
    usage = payload.get("usage") or {}
    inp = int(usage.get("input_tokens") or 0)
    out_tok = int(usage.get("output_tokens") or 0)
    # Haiku-class judge: ~$1 / $5 per MTok. Conservative enough for the spend cap.
    usd = (inp / 1_000_000.0) * 1.0 + (out_tok / 1_000_000.0) * 5.0
    ledger.record("memory_qa_judge", usd, {"id": item_id})
    out["usd"] = usd
    out["hit"] = hit
    path.write_text(json.dumps({"hit": hit, "raw": "\n".join(text_parts)[:500]}, indent=2) + "\n")
    return out
