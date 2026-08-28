"""Failure diagnosis for Superopen benchmark numbers."""

from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from typing import Any


def _load(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {}
    try:
        return json.loads(path.read_text())
    except json.JSONDecodeError:
        return {}


def _episode_titles(store: Path) -> list[str]:
    db = store / ".so" / "db" / "so.db"
    if not db.is_file():
        return []
    con = sqlite3.connect(db)
    try:
        rows = con.execute("SELECT title, faded FROM memory_episodes").fetchall()
    except sqlite3.Error:
        return []
    finally:
        con.close()
    return [str(t[0]) for t in rows]


def _pct(hits: Any, total: Any) -> str | None:
    try:
        h, n = int(hits), int(total)
    except (TypeError, ValueError):
        return None
    if n <= 0:
        return None
    return f"{h}/{n} ({h / n:.1%})"


def _vs_bm25(name: str, so: dict[str, Any] | None, bm: dict[str, Any] | None, target: float | None = None) -> str:
    so = so or {}
    bm = bm or {}
    so_r = so.get("recall_at_10")
    bm_r = bm.get("recall_at_10")
    so_n = _pct(so.get("hits"), so.get("total"))
    bm_n = _pct(bm.get("hits"), bm.get("total"))
    if so_r is None or bm_r is None:
        return f"{name}: Superopen or BM25 recall missing."
    if so_r > bm_r:
        gate = f"beats BM25 ({so_n} vs {bm_n})"
    elif so_r == bm_r:
        gate = f"ties BM25 ({so_n})"
    else:
        gate = f"behind BM25 ({so_n} vs {bm_n})"
    if target is not None and so_r < target:
        gate += f"; below {target:.0%} target"
    elif target is not None:
        gate += f"; meets {target:.0%} target"
    return f"{name} R@10 Superopen {so_r:.1%} {gate}."


def _ingest_note(label: str, ingest: dict[str, Any]) -> str | None:
    if not ingest:
        return None
    attempted = ingest.get("episodes_attempted")
    stored = ingest.get("stored_episode_ids")
    skipped = ingest.get("skipped")
    collapsed = ingest.get("collapsed_near_duplicate")
    embedder = ingest.get("embedder_id")
    parts = [f"{label} ingest"]
    if ingest.get("skipped_reingest"):
        parts.append(f"reused store ({stored} episode ids, embedder={embedder})")
        return " ".join(str(p) for p in parts) + "."
    if attempted is not None:
        parts.append(f"{attempted} captures → {stored} stored ids")
    if skipped:
        parts.append(f"{skipped} skipped")
    if collapsed:
        parts.append(f"{collapsed} near-dupes")
    if embedder:
        parts.append(f"embedder={embedder}")
    return " ".join(str(p) for p in parts) + "."


def _qa_note(label: str, payload: dict[str, Any]) -> list[str]:
    out: list[str] = []
    qa = payload.get("qa") or {}
    if qa:
        so_qa = (qa.get("superopen") or {}).get("accuracy")
        bm_qa = (qa.get("bm25") or {}).get("accuracy")
        if so_qa is not None:
            line = f"{label} extractive QA Superopen {so_qa:.1%}"
            if bm_qa is not None:
                line += f" vs BM25 {bm_qa:.1%} (debug substring; headline is qa_llm)"
            out.append(line + ".")
    llm = payload.get("qa_llm")
    if isinstance(llm, dict):
        so = llm.get("superopen") or llm
        acc = so.get("accuracy")
        method = so.get("method") or "coding_agent_session"
        if acc is not None:
            hits = _pct(so.get("hits"), so.get("total"))
            extra = f" ({hits})" if hits else ""
            out.append(f"{label} coding-agent QA {acc:.1%}{extra} via {method}.")
        if so.get("stopped_on_spend"):
            out.append(f"{label} QA stopped on spend: {so['stopped_on_spend']}")
        if so.get("timed_out"):
            out.append(f"{label} QA timeouts={so.get('timed_out')} empty_pack={so.get('empty_pack')}")
        tools = so.get("tools_likely")
        total = so.get("total")
        if tools is not None and total:
            out.append(
                f"{label} agent used tool-sized output on {tools}/{total} sessions "
                "(low tools_likely means it answered from weights, not so memory recall)."
            )
        misses = [r for r in (so.get("rows") or []) if isinstance(r, dict) and not r.get("hit")]
        for r in misses[:8]:
            out.append(
                f"{label} miss {r.get('id')}: tools_likely={r.get('tools_likely')} "
                f"out={r.get('output_tokens')} cache_read={r.get('cache_read_tokens')} "
                f"gold={r.get('answer_gold')!r} got={r.get('result_head')!r}"
            )
    elif isinstance(llm, str) and llm.strip():
        out.append(f"{label} coding-agent QA: {llm}")
    return out


def _debug_memory(out: Path, so_bin: str) -> dict[str, Any]:
    locomo = _load(out / "memory_locomo.json")
    lme = _load(out / "memory_longmemeval.json")
    locomo_store = out / "memory" / "locomo"
    lme_store = out / "memory" / "longmemeval"
    locomo_titles = _episode_titles(locomo_store)
    lme_titles = _episode_titles(lme_store)
    reasons: list[str] = []

    loc_ad = locomo.get("adapters") or {}
    if loc_ad:
        reasons.append(_vs_bm25("LOCOMO", loc_ad.get("superopen"), loc_ad.get("bm25")))
    loc_ing = _ingest_note("LOCOMO", locomo.get("ingest") or {})
    if loc_ing:
        reasons.append(loc_ing)
    if locomo_titles and (locomo.get("ingest") or {}).get("episodes_attempted"):
        attempted = int((locomo.get("ingest") or {}).get("episodes_attempted") or 0)
        if attempted and len(locomo_titles) < attempted:
            reasons.append(
                f"LOCOMO store has {len(locomo_titles)} episodes vs {attempted} attempted; "
                "missing golds cannot hit R@10."
            )
    reasons.extend(_qa_note("LOCOMO", locomo))

    lme_ad = (lme.get("adapters") or {}) if isinstance(lme, dict) else {}
    if lme_ad:
        reasons.append(_vs_bm25("LongMemEval-S", lme_ad.get("superopen"), lme_ad.get("bm25"), target=0.90))
    lme_ing = _ingest_note("LongMemEval-S", lme.get("ingest") or {})
    if lme_ing:
        reasons.append(lme_ing)
    if lme_titles and (lme.get("ingest") or {}).get("episodes_attempted"):
        attempted = int((lme.get("ingest") or {}).get("episodes_attempted") or 0)
        skipped = int((lme.get("ingest") or {}).get("skipped") or 0)
        if skipped:
            reasons.append(
                f"LongMemEval-S dropped {skipped} captures ({len(lme_titles)} rows kept); "
                "a skipped gold session is a hard miss."
            )
    reasons.extend(_qa_note("LongMemEval-S", lme))

    if not loc_ad and not lme_ad:
        reasons.append("No memory_locomo.json / memory_longmemeval.json in this work dir.")

    return {
        "suite": "memory",
        "so_bin": so_bin,
        "stored_episodes": {"locomo": len(locomo_titles), "longmemeval": len(lme_titles)},
        "stored_titles_sample": {"locomo": locomo_titles[:12], "longmemeval": lme_titles[:12]},
        "locomo": loc_ad or None,
        "locomo_qa": locomo.get("qa") or locomo.get("qa_llm"),
        "longmemeval": lme_ad or lme or None,
        "verdict": reasons,
    }


def _debug_graph(out: Path) -> dict[str, Any]:
    graph = _load(out / "graph.json")
    if not graph:
        return {"suite": "graph", "pct": None, "weak": [], "verdict": []}
    probes = (graph.get("graph") or {}).get("probes") or graph.get("probes") or []
    weak = [p for p in probes if float(p.get("score") or 0) < 1.0]
    reasons: list[str] = []
    for p in weak:
        notes = str(p.get("notes") or "")[:240]
        reasons.append(f"{p.get('id')} {p.get('category')} score={p.get('score')}: {notes}")
    pct = (graph.get("graph") or graph).get("pct")
    hits = (graph.get("graph") or graph).get("hits")
    total = (graph.get("graph") or graph).get("total")
    if pct is None and hits is not None and total:
        pct = 100.0 * float(hits) / float(total)
    if pct is not None and pct < 100 and not reasons:
        reasons.append(f"graph-tools {pct}% — inspect probe notes")
    if pct is not None and pct >= 100:
        reasons.append("graph-tools 12/12.")
    elif pct is not None and pct >= 90:
        reasons.append(f"graph-tools {pct}% is close; remaining misses are usually snippet format, not an empty index.")
    return {"suite": "graph", "pct": pct, "weak": weak, "verdict": reasons}


def _debug_compare(out: Path) -> dict[str, Any]:
    cmp_ = _load(out / "compare.json")
    if not cmp_:
        return {"suite": "compare", "summary": {}, "verdict": []}
    summary = cmp_.get("summary") or {}
    rows = cmp_.get("rows") or []
    reasons: list[str] = []
    native = [r for r in rows if r.get("arm") == "native"]
    so_rows = [r for r in rows if r.get("arm") == "superopen"]
    cov_n = summary.get("native_coverage_avg", summary.get("native_coverage"))
    cov_s = summary.get("superopen_coverage_avg", summary.get("superopen_coverage"))
    if cov_n is not None and cov_s is not None:
        if cov_s + 1e-9 < float(cov_n):
            reasons.append(
                f"coverage Superopen {cov_s} < native {cov_n} — token savings do not count unless coverage ≥ native."
            )
        else:
            reasons.append(f"coverage Superopen {cov_s} ≥ native {cov_n} (effectiveness bar met).")
    n_in = int(summary.get("native_uncached_input") or 0)
    s_in = int(summary.get("superopen_uncached_input") or 0)
    n_cr = int(summary.get("native_cache_read_tokens") or 0)
    s_cr = int(summary.get("superopen_cache_read_tokens") or 0)
    n_out = int(summary.get("native_output_tokens") or 0)
    s_out = int(summary.get("superopen_output_tokens") or 0)
    if n_in or s_in or s_cr:
        reasons.append(
            f"uncached input native {n_in} vs Superopen {s_in}; "
            f"cache_read native {n_cr} vs Superopen {s_cr}; "
            f"output native {n_out} vs Superopen {s_out}."
        )
    if s_cr > max(n_cr, 1) * 10:
        reasons.append(
            "Superopen cache_read dominates billed tokens. This is usually tool-result cache "
            "(so graph query / snippet / Read), not the small CLAUDE.md graph-first block. "
            "Product knobs: graph query row cap, snippet vs dump, PreToolUse nudges that force extra tool turns."
        )
    if s_out > max(n_out, 1) * 5:
        reasons.append(
            f"Superopen output {s_out} vs native {n_out}: the Superopen agent explored (tools_likely); "
            "native often answers from weights with almost no tools. That is a real product gap on this bank, "
            "not a harness packing trick."
        )
    native_usd = summary.get("native_cost_usd")
    so_usd = summary.get("superopen_cost_usd")
    if native_usd is not None and so_usd is not None:
        if float(so_usd) > float(native_usd):
            reasons.append(
                f"USD Superopen {so_usd:.4f} > native {native_usd:.4f} — misses the cheaper-agent goal "
                "(target ~50% fewer tokens at coverage ≥ native)."
            )
        else:
            reasons.append(f"USD Superopen {so_usd:.4f} ≤ native {native_usd:.4f}.")
    if s_in and n_in and s_in < n_in and s_cr > n_in * 20:
        reasons.append(
            "Uncached input looks cheaper because Claude billed graph/tool payloads as cache_read. "
            "Count cache_read+output when judging whether Superopen made the agent cheaper."
        )
    claude_md = out / "compare" / "arms" / "superopen" / "home" / ".claude" / "CLAUDE.md"
    if claude_md.is_file():
        n = len(claude_md.read_text())
        reasons.append(f"installed CLAUDE.md Superopen block {n} bytes (durable steer; should stay small).")
    ranked = sorted(so_rows, key=lambda r: int(r.get("cache_read_tokens") or 0), reverse=True)
    for r in ranked[:3]:
        reasons.append(
            f"{r.get('id')}: uncached {r.get('input_tokens')} cache_read {r.get('cache_read_tokens')} "
            f"cache_write {r.get('cache_creation_tokens')} out {r.get('output_tokens')} usd {r.get('cost_usd')} "
            f"tools_likely={r.get('tools_likely')} ok={r.get('ok')}"
        )
    for n in native:
        s = next((x for x in so_rows if x.get("id") == n.get("id")), None)
        if not s:
            reasons.append(f"missing Superopen row for {n.get('id')}")
            continue
        if not s.get("ok"):
            reasons.append(f"{s.get('id')} superopen arm failed: {str(s.get('stderr') or s.get('error') or '')[:160]}")
    if summary.get("graph_first_unproven"):
        reasons.append("graph_first_unproven: neither arm showed tool-sized output; coverage may be training-data.")
    native_tools = sum(1 for r in native if r.get("tools_likely"))
    so_tools = sum(1 for r in so_rows if r.get("tools_likely"))
    reasons.append(f"tools_likely sessions native {native_tools}/{len(native)} Superopen {so_tools}/{len(so_rows)}.")
    return {
        "suite": "compare",
        "summary": summary,
        "verdict": reasons,
    }


def _debug_contradict(out: Path) -> dict[str, Any]:
    c = _load(out / "contradiction.json")
    if not c:
        return {"suite": "contradict", "raw": {}, "verdict": []}
    reasons = []
    if c.get("ok") is False:
        reasons.append("contradict Go gate failed")
    elif c.get("ok") is True:
        reasons.append("contradict Go gate passed (Rescue@10 / historical-verbatim unit tests).")
    if c.get("note"):
        reasons.append(str(c.get("note")))
    return {"suite": "contradict", "raw": c, "verdict": reasons}


def _debug_index(out: Path) -> dict[str, Any]:
    idx = _load(out / "index.json")
    temporal = _load(out / "temporal.json")
    reasons = []
    if idx.get("ok"):
        reasons.append(
            f"Index ok in {idx.get('elapsed_sec')}s nodes={idx.get('nodes')} edges={idx.get('edges')} files={idx.get('files')}."
        )
    elif idx:
        reasons.append(f"Index not ok: {idx.get('error') or idx.get('stderr') or idx}")
    if temporal:
        reasons.append("temporal checkpoint file present; inspect temporal.json for per-tag counts.")
    return {"suite": "index/temporal", "index": idx, "temporal": temporal.get("checkpoints") or temporal, "verdict": reasons}


def _debug_latency(out: Path) -> dict[str, Any]:
    lat = _load(out / "latency.json")
    if not lat:
        return {"suite": "latency", "raw": {}, "verdict": []}
    return {"suite": "latency", "raw": lat, "verdict": ["Hardware-local memory search latency; not a quality score."]}


def run_debug_mode(out: Path, so_bin: str) -> dict[str, Any]:
    payload = {
        "out": str(out),
        "so_bin": so_bin,
        "suites": [
            _debug_memory(out, so_bin),
            _debug_graph(out),
            _debug_compare(out),
            _debug_contradict(out),
            _debug_index(out),
            _debug_latency(out),
        ],
    }
    bad: list[str] = []
    for suite in payload["suites"]:
        for line in suite.get("verdict") or []:
            bad.append(f"[{suite.get('suite')}] {line}")
    payload["why_bad"] = bad
    (out / "debug.json").write_text(json.dumps(payload, indent=2) + "\n")
    print("=== why numbers are weak ===", flush=True)
    if not bad:
        print("(no suite JSON in this work dir yet)", flush=True)
    for line in bad:
        print(line, flush=True)
    return payload
