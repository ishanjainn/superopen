#!/usr/bin/env python3
"""Write repo-root BENCHMARKS.md from a harness summary (in-memory or a dir).

Product runs keep only this markdown file. Work directories are deleted after
the report is written.
"""

from __future__ import annotations

import argparse
import json
from copy import deepcopy
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

BENCH = Path(__file__).resolve().parent
REPO = BENCH.parent
LAST_SUMMARY = BENCH / ".last-summary.json"

PENDING = "pending"


def _load(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {}
    try:
        data = json.loads(path.read_text())
    except json.JSONDecodeError:
        return {}
    return data if isinstance(data, dict) else {}


def _stamp_json(stamp: Path, name: str) -> dict[str, Any]:
    return _load(stamp / name)


def _has_recall(block: Any) -> bool:
    if not isinstance(block, dict):
        return False
    adapters = block.get("adapters") or {}
    so = adapters.get("superopen") if isinstance(adapters, dict) else None
    return isinstance(so, dict) and so.get("recall_at_10") is not None


def _memory_split(payload: dict[str, Any], split: str) -> dict[str, Any]:
    if split == "longmemeval":
        block = payload.get("memory_longmemeval")
        if _has_recall(block):
            return block  # type: ignore[return-value]
        mem = payload.get("memory")
        if isinstance(mem, dict) and mem.get("split") == "longmemeval" and _has_recall(mem):
            return mem
        return {}
    locomo_file = payload.get("memory_locomo")
    if _has_recall(locomo_file):
        return locomo_file  # type: ignore[return-value]
    mem = payload.get("memory")
    if isinstance(mem, dict) and mem.get("split") in (None, "locomo") and _has_recall(mem):
        return mem
    return {}


def load_stamp(stamp: Path) -> dict[str, Any]:
    payload = _stamp_json(stamp, "summary.json")
    payload["_stamp"] = str(stamp.resolve())
    siblings = {
        "graph": "graph.json",
        "compare": "compare.json",
        "swe": "swe.json",
        "contradict": "contradiction.json",
        "latency": "latency.json",
        "index": "index.json",
        "temporal": "temporal.json",
        "memory_locomo": "memory_locomo.json",
        "memory_longmemeval": "memory_longmemeval.json",
        "offline": "offline.json",
    }
    for key, name in siblings.items():
        data = _stamp_json(stamp, name)
        if not data:
            continue
        if key == "contradict" and not payload.get("contradict"):
            payload["contradict"] = data
        elif key.startswith("memory_"):
            payload.setdefault(key, data)
        elif not payload.get(key):
            payload[key] = data
    mem = payload.get("memory")
    if isinstance(mem, dict) and mem.get("split") == "longmemeval":
        payload.setdefault("memory_longmemeval", mem)
    return payload


def _fill_key(dst: dict[str, Any], src: dict[str, Any], key: str) -> None:
    if dst.get(key):
        return
    val = src.get(key)
    if not val:
        return
    if isinstance(val, dict) and val.get("skipped") and set(val) <= {"skipped"}:
        return
    dst[key] = deepcopy(val)


def merge_stamps(primary: dict[str, Any], extras: list[dict[str, Any]]) -> dict[str, Any]:
    out = deepcopy(primary)
    sources = [primary.get("_stamp")]
    for extra in extras:
        sources.append(extra.get("_stamp"))
        if not _memory_split(out, "locomo") and _memory_split(extra, "locomo"):
            loc = _memory_split(extra, "locomo")
            out["memory_locomo"] = loc
            if not (isinstance(out.get("memory"), dict) and out["memory"].get("split") == "locomo"):
                out["memory"] = loc
        if not _memory_split(out, "longmemeval") and _memory_split(extra, "longmemeval"):
            out["memory_longmemeval"] = _memory_split(extra, "longmemeval")
        for key in ("graph", "compare", "swe", "contradict", "latency", "index", "temporal"):
            _fill_key(out, extra, key)
        extra_modes = extra.get("mode_duration_sec")
        if isinstance(extra_modes, dict):
            dst_modes = out.get("mode_duration_sec")
            if not isinstance(dst_modes, dict):
                dst_modes = {}
                out["mode_duration_sec"] = dst_modes
            for name, sec in extra_modes.items():
                if name not in dst_modes:
                    dst_modes[name] = sec
    modes = out.get("mode_duration_sec")
    if isinstance(modes, dict) and modes:
        try:
            out["duration_sec"] = round(sum(float(v) for v in modes.values()), 3)
        except (TypeError, ValueError):
            pass
    out["_sources"] = [s for s in sources if s]
    return out


def load_last_summary() -> dict[str, Any]:
    if not LAST_SUMMARY.is_file():
        return {}
    try:
        data = json.loads(LAST_SUMMARY.read_text())
    except json.JSONDecodeError:
        return {}
    return data if isinstance(data, dict) else {}


def save_last_summary(payload: dict[str, Any]) -> Path:
    slim = deepcopy(payload)
    slim.pop("_stamp", None)
    LAST_SUMMARY.write_text(json.dumps(slim, indent=2) + "\n")
    return LAST_SUMMARY


def merge_previous_suites(
    payload: dict[str, Any],
    *,
    fill_from: list[Path] | None = None,
) -> dict[str, Any]:
    extras: list[dict[str, Any]] = []
    prev = load_last_summary()
    if prev:
        extras.append(prev)
    for path in fill_from or []:
        extras.append(load_stamp(path))
    if not extras:
        return payload
    return merge_stamps(payload, extras)


def _fmt_recall(block: dict[str, Any] | None, adapter: str = "superopen", k: int = 10) -> str:
    if not block:
        return PENDING
    row = (block.get("adapters") or {}).get(adapter) or {}
    key = f"recall_at_{k}"
    r = row.get(key)
    hits = row.get(f"hits_at_{k}")
    if hits is None and k == 10:
        hits = row.get("hits")
    total = row.get("total")
    if r is None:
        return PENDING
    if hits is not None and total:
        return f"{float(r):.1%} ({int(hits)}/{int(total)})"
    return f"{float(r):.1%}"


def _fmt_usd(value: Any) -> str | None:
    try:
        return f"${float(value):.2f}"
    except (TypeError, ValueError):
        return None


def _pct_delta(new: Any, old: Any) -> str | None:
    try:
        n, o = float(new), float(old)
    except (TypeError, ValueError):
        return None
    if o == 0:
        return None
    return f"{(o - n) / o * 100:.0f}%"


def _adapter_row(block: dict[str, Any], name: str, k: int = 10) -> str:
    return _fmt_recall(block, name, k) if _has_recall(block) else PENDING


def _internal_recall(block: dict[str, Any] | None, k: int = 10) -> str:
    if not block:
        return "BM25 / bow / RRF"
    parts = [
        f"BM25 {_adapter_row(block, 'bm25', k)}",
        f"bow {_adapter_row(block, 'bow', k) if _has_recall(block) and (block.get('adapters') or {}).get('bow') else _adapter_row(block, 'dense', k)}",
        f"RRF {_adapter_row(block, 'rrf', k)}",
    ]
    if all("pending" in p for p in parts):
        return PENDING
    return "; ".join(parts)


def _qa_llm(block: dict[str, Any]) -> dict[str, Any]:
    llm = block.get("qa_llm")
    if isinstance(llm, dict):
        so = llm.get("superopen") or llm
        if isinstance(so, dict) and (
            so.get("accuracy_judge") is not None or so.get("qa_accuracy") is not None
        ):
            return so
    return {}


def _fmt_qa(block: dict[str, Any] | None) -> str:
    if not block:
        return PENDING
    so = _qa_llm(block)
    if not so:
        return PENDING
    qa_n = block.get("qa_n")
    skipped = so.get("skipped_no_gold")
    host = so.get("host") or ""
    if so.get("qa_accuracy") is not None:
        hits, total = so.get("hits_strict"), so.get("total")
        parts = [f"{float(so['qa_accuracy']):.1%} (key-fact coverage)"]
    elif so.get("accuracy_judge") is not None:
        hits, total = so.get("hits_judge"), so.get("total")
        parts = [f"{float(so['accuracy_judge']):.1%} (judge)"]
    else:
        return PENDING
    if so.get("accuracy_judge") is not None and so.get("qa_accuracy") is not None:
        parts.append(f"judge {float(so['accuracy_judge']):.0%}")
    if hits is not None and total:
        parts.append(f"{int(hits)}/{int(total)} scored")
    if qa_n:
        parts.append(f"qa_n={qa_n}")
    if skipped:
        parts.append(f"{skipped} empty-gold skipped")
    if host:
        parts.append(host)
    return "; ".join(parts)


def _fmt_gold_store(block: dict[str, Any] | None) -> str:
    if not block:
        return ""
    gold = block.get("gold_in_store") or {}
    if not gold:
        ingest = block.get("ingest") or {}
        if not ingest:
            return ""
        collapsed = ingest.get("collapsed_near_duplicate")
        skipped = ingest.get("skipped")
        if collapsed is None and skipped is None:
            return ""
        return (
            f"Capture-side: collapsed={collapsed or 0}, skipped={skipped or 0}. "
            "BM25/bow/RRF search raw docs; Superopen searches the post-capture store."
        )
    return (
        f"Gold-in-store: hard_miss={gold.get('hard_miss', 0)}, "
        f"rank_miss={gold.get('rank_miss', 0)} of scored={gold.get('scored', 0)} "
        f"(stored_docs={gold.get('stored_docs', 0)}). "
        f"{gold.get('note') or ''}"
    ).strip()


def _graph_score(payload: dict[str, Any]) -> str:
    graph = payload.get("graph") or {}
    inner = graph.get("graph") if isinstance(graph.get("graph"), dict) else graph
    hits = inner.get("hits")
    total = inner.get("total") or inner.get("max")
    score = inner.get("score")
    pct = inner.get("pct")
    if hits is not None and total:
        return f"{int(hits)}/{int(float(total))}"
    if score is not None and total:
        return f"{int(float(score))}/{int(float(total))}"
    if pct is not None:
        return f"{float(pct):.0f}%"
    return PENDING


def _graph_p95(payload: dict[str, Any]) -> str:
    graph = payload.get("graph") or {}
    inner = graph.get("graph") if isinstance(graph.get("graph"), dict) else graph
    p95 = inner.get("p95_ms")
    p50 = inner.get("p50_ms")
    if p95 is None:
        return PENDING
    if p50 is None:
        return f"p95={float(p95):.0f} ms"
    return f"p50={float(p50):.0f} ms, p95={float(p95):.0f} ms"


def _index_block(payload: dict[str, Any]) -> dict[str, Any]:
    idx = payload.get("index")
    if isinstance(idx, dict) and (idx.get("nodes") is not None or idx.get("ok") is not None):
        return idx
    graph = payload.get("graph") or {}
    nested = graph.get("index") if isinstance(graph, dict) else None
    return nested if isinstance(nested, dict) else {}


def _compare_summary(payload: dict[str, Any]) -> dict[str, Any]:
    cmp_ = payload.get("compare") or {}
    if isinstance(cmp_, dict) and isinstance(cmp_.get("summary"), dict):
        return cmp_
    return {}


def _compare_cheaper(rows: list[dict[str, Any]]) -> tuple[int, int]:
    native = {r.get("id"): r for r in rows if r.get("arm") == "native"}
    so_rows = {r.get("id"): r for r in rows if r.get("arm") == "superopen"}
    wins = 0
    n = 0
    for qid, nr in native.items():
        sr = so_rows.get(qid)
        if not sr:
            continue
        n += 1
        try:
            if float(sr.get("cost_usd") or 0) < float(nr.get("cost_usd") or 0):
                wins += 1
        except (TypeError, ValueError):
            continue
    return wins, n


def _fmt_compare_coverage(cmp_: dict[str, Any]) -> str:
    summary = cmp_.get("summary") or {}
    cov_s = summary.get("superopen_coverage_avg")
    cov_n = summary.get("native_coverage_avg")
    if cov_s is None:
        return PENDING
    extra = f" (native {float(cov_n):.2f})" if cov_n is not None else ""
    return f"{float(cov_s):.2f}{extra}"


def _fmt_compare_usd(cmp_: dict[str, Any]) -> str:
    summary = cmp_.get("summary") or {}
    so_usd = summary.get("superopen_cost_usd")
    n_usd = summary.get("native_cost_usd")
    if so_usd is None:
        return PENDING
    so_s = _fmt_usd(so_usd) or PENDING
    n_s = _fmt_usd(n_usd)
    delta = _pct_delta(so_usd, n_usd)
    cheaper, n = _compare_cheaper(cmp_.get("rows") or [])
    parts = [f"{so_s} vs {n_s} native"] if n_s else [so_s]
    if delta:
        parts.append(f"{delta} cheaper")
    if n:
        parts.append(f"cheaper on {cheaper}/{n} questions")
    return "; ".join(parts)


def _fmt_compare_cache(cmp_: dict[str, Any]) -> str:
    summary = cmp_.get("summary") or {}
    s_cr = summary.get("superopen_cache_read_tokens")
    n_cr = summary.get("native_cache_read_tokens")
    if s_cr is None:
        return PENDING
    delta = _pct_delta(s_cr, n_cr)
    line = f"{int(s_cr):,} vs {int(n_cr):,} native" if n_cr is not None else f"{int(s_cr):,}"
    if delta:
        line += f" ({delta} less cache_read)"
    return line


def _swe_summary(payload: dict[str, Any]) -> dict[str, Any]:
    swe = payload.get("swe") or {}
    if isinstance(swe, dict) and isinstance(swe.get("summary"), dict):
        return swe
    return {}


def _fmt_swe_correctness(swe: dict[str, Any]) -> str:
    summary = swe.get("summary") or {}
    if not summary.get("graded"):
        n = summary.get("n")
        return f"pending official grader (n={n})" if n else PENDING
    n = int(summary.get("n") or 0)
    so_n = summary.get("superopen_resolved")
    nat_n = summary.get("native_resolved")
    if so_n is None:
        return PENDING
    so_pct = 100.0 * float(so_n) / n if n else 0.0
    nat_pct = 100.0 * float(nat_n) / n if n and nat_n is not None else 0.0
    return f"{int(so_n)} / {n} ({so_pct:.0f}%) vs native {int(nat_n)} / {n} ({nat_pct:.0f}%)"


def _fmt_swe_metric(swe: dict[str, Any], native_key: str, so_key: str, savings_key: str, *, money: bool = False, seconds: bool = False) -> str:
    summary = swe.get("summary") or {}
    native = summary.get(native_key)
    so_val = summary.get(so_key)
    if so_val is None:
        return PENDING
    savings = summary.get(savings_key) or "—"
    if money:
        return f"{_fmt_usd(so_val) or PENDING} vs {_fmt_usd(native) or PENDING} native ({savings})"
    if seconds:
        return f"{float(so_val):.0f}s vs {float(native or 0):.0f}s native ({savings})"
    return f"{int(so_val):,} vs {int(native or 0):,} native ({savings})"


def _fmt_count_pct(hits: Any, n: int) -> str:
    if hits is None or not n:
        return PENDING
    pct = 100.0 * float(hits) / n
    return f"{int(hits)} / {n} ({pct:.0f}%)"


def _fmt_compact_tokens(n: Any) -> str:
    if n is None:
        return PENDING
    val = float(n)
    if val >= 1_000_000:
        return f"{val / 1_000_000:.1f}M"
    if val >= 10_000:
        return f"{val / 1_000:.0f}K"
    return f"{int(val):,}"


def _fmt_pts(so_n: Any, native_n: Any, n: int) -> str:
    if so_n is None or native_n is None or not n:
        return PENDING
    delta = 100.0 * (float(so_n) - float(native_n)) / n
    sign = "+" if delta >= 0 else ""
    return f"{sign}{delta:.0f} pts"


def _row_tokens(row: dict[str, Any]) -> float:
    return (
        float(row.get("input_tokens") or 0)
        + float(row.get("output_tokens") or 0)
        + float(row.get("cache_read_tokens") or 0)
        + float(row.get("cache_creation_tokens") or 0)
    )


def _resolved_label(ok: Any, *, bold_pass: bool = False) -> str:
    if ok is True:
        return "**Passed**" if bold_pass else "Passed"
    if ok is False:
        return "Failed"
    return "pending"


def _instance_line(qid: str, nr: dict[str, Any], sr: dict[str, Any]) -> str:
    n_tok = _row_tokens(nr)
    s_tok = _row_tokens(sr)
    n_tools = float(nr.get("tool_calls") or 0)
    s_tools = float(sr.get("tool_calls") or 0)
    tok_pct = f"{(s_tok / n_tok * 100):.0f}%" if n_tok else "—"
    tool_pct = f"{(s_tools / n_tools * 100):.0f}%" if n_tools else "—"
    return (
        f"| `{qid}` | {_resolved_label(nr.get('resolved'))} | "
        f"{_resolved_label(sr.get('resolved'), bold_pass=True)} | {tok_pct} | {tool_pct} |"
    )


def _swe_headline_table(swe: dict[str, Any]) -> str:
    """Cold vs Superopen comparison table."""
    summary = swe.get("summary") or {}
    n = int(summary.get("n") or 0)
    graded = bool(summary.get("graded"))
    if graded:
        nat_c = _fmt_count_pct(summary.get("native_resolved"), n)
        so_c = f"**{_fmt_count_pct(summary.get('superopen_resolved'), n)}**"
        pts = _fmt_pts(summary.get("superopen_resolved"), summary.get("native_resolved"), n)
        corr_imp = f"**{pts}**" if pts != PENDING else PENDING
    else:
        nat_c = so_c = corr_imp = PENDING
    nat_tok = _fmt_compact_tokens(summary.get("native_tokens"))
    so_tok = _fmt_compact_tokens(summary.get("superopen_tokens"))
    tok_imp = summary.get("token_savings") or "—"
    nat_usd = _fmt_usd(summary.get("native_cost_usd")) or PENDING
    so_usd = _fmt_usd(summary.get("superopen_cost_usd")) or PENDING
    usd_imp = summary.get("cost_savings") or "—"
    nat_tools = int(summary.get("native_tool_calls") or 0)
    so_tools = int(summary.get("superopen_tool_calls") or 0)
    tool_imp = summary.get("tool_savings") or "—"
    nat_req = int(summary.get("native_api_requests") or 0)
    so_req = int(summary.get("superopen_api_requests") or 0)
    req_imp = summary.get("request_savings") or "—"
    nat_wall = float(summary.get("native_wall_sec") or 0)
    so_wall = float(summary.get("superopen_wall_sec") or 0)
    wall_imp = summary.get("wall_savings") or "—"
    if summary.get("superopen_tokens") is None:
        return "pending"
    return "\n".join(
        [
            "| Correctness & efficiency | Cold Claude Code | Claude Code with Superopen | Improvement |",
            "|---|---|---|---|",
            f"| Correctness | {nat_c} | {so_c} | {corr_imp} |",
            f"| Tokens | {nat_tok} | **{so_tok}** | **{tok_imp}** |",
            f"| Cost | {nat_usd} | **{so_usd}** | **{usd_imp}** |",
            f"| Tool calls | {nat_tools:,} | **{so_tools:,}** | **{tool_imp}** |",
            f"| API requests | {nat_req:,} | **{so_req:,}** | **{req_imp}** |",
            f"| Wall-clock | {nat_wall:,.0f}s | **{so_wall:,.0f}s** | **{wall_imp}** |",
        ]
    )


def _swe_efficiency_note(swe: dict[str, Any]) -> str:
    summary = swe.get("summary") or {}
    over = summary.get("efficiency_over") or "all_completed"
    if over == "both_resolved":
        return (
            "Correctness is scored on every instance. Tokens, cost, tool calls, API requests "
            "and wall-clock are over instances both arms resolved."
        )
    if not summary.get("graded"):
        return (
            "Correctness is scored on every instance (pending official grader until `--swe-grade` "
            "succeeds). Efficiency rows are over all completed sessions until both arms have "
            "resolved instances."
        )
    return (
        "Correctness is scored on every instance. Efficiency rows are over all completed sessions "
        "(no instance both arms resolved)."
    )


def _swe_instance_tables(swe: dict[str, Any]) -> str:
    rows = swe.get("rows") or []
    native = {r.get("id"): r for r in rows if r.get("arm") == "native"}
    so_rows = {r.get("id"): r for r in rows if r.get("arm") == "superopen"}
    header = [
        "| SWE-bench instance | Cold Claude Code | Claude Code with Superopen | Token usage vs. cold | Tool usage vs. cold |",
        "|---|---|---|---:|---:|",
    ]
    all_rows: list[str] = list(header)
    seen_any = False
    for qid, nr in native.items():
        sr = so_rows.get(qid)
        if not sr:
            continue
        all_rows.append(_instance_line(str(qid), nr, sr))
        seen_any = True
    parts = [_swe_headline_table(swe)]
    if seen_any:
        parts.append(
            "\n".join(
                [
                    "<details>",
                    "<summary>Correctness over all instances</summary>",
                    "",
                    _swe_efficiency_note(swe),
                    "",
                    "\n".join(all_rows),
                    "",
                    "</details>",
                ]
            )
        )
    return "\n\n".join(parts) if parts else "pending instance rows"


def _fmt_contradict_metric(payload: dict[str, Any], key: str) -> str:
    c = payload.get("contradict") or {}
    if not c:
        return PENDING
    val = c.get(key)
    if val is None:
        return PENDING
    try:
        return f"{float(val):.3f}"
    except (TypeError, ValueError):
        return str(val)


def _bow_row(block: dict[str, Any], k: int = 10) -> str:
    adapters = block.get("adapters") or {}
    if "bow" in adapters:
        return _adapter_row(block, "bow", k)
    return _adapter_row(block, "dense", k)


def _fmt_ingest(block: dict[str, Any] | None) -> str:
    if not block:
        return PENDING
    ingest = block.get("ingest") or {}
    if not ingest:
        return PENDING
    usd = ingest.get("llm_usd")
    if usd is None:
        usd = 0.0
    parts = [f"${float(usd):.2f}"]
    eid = ingest.get("embedder_id") or block.get("embedder_id")
    if eid:
        parts.append(str(eid))
    pending = ingest.get("embedding_pending")
    if pending is not None:
        parts.append(f"pending={pending}")
    collapsed = ingest.get("collapsed_near_duplicate")
    skipped = ingest.get("skipped")
    if collapsed is not None or skipped is not None:
        parts.append(f"collapsed={collapsed or 0} skipped={skipped or 0}")
    worker = (block.get("embed_worker") or {}).get("model")
    if worker:
        parts.append(f"worker={worker}")
    return "; ".join(parts)


def _fmt_latency(payload: dict[str, Any]) -> str:
    lat = (payload.get("latency") or {}).get("memory_search_ms") or {}
    p50, p95 = lat.get("p50"), lat.get("p95")
    if p50 is None:
        return PENDING
    return f"p50={float(p50):.0f} ms, p95={float(p95):.0f} ms"


def _fmt_duration(sec: Any) -> str:
    try:
        s = float(sec)
    except (TypeError, ValueError):
        return PENDING
    if s < 0:
        return PENDING
    if s >= 3600:
        h = int(s // 3600)
        m = int((s % 3600) // 60)
        return f"{h}h {m}m"
    if s >= 60:
        m = int(s // 60)
        r = int(s % 60)
        return f"{m}m {r}s"
    if s >= 10:
        return f"{s:.0f}s"
    return f"{s:.1f}s"


def _fmt_run_duration(payload: dict[str, Any]) -> str:
    modes = payload.get("mode_duration_sec") or {}
    if isinstance(modes, dict) and modes:
        try:
            return _fmt_duration(sum(float(v) for v in modes.values()))
        except (TypeError, ValueError):
            pass
    sec = payload.get("duration_sec")
    if sec is None:
        return PENDING
    return _fmt_duration(sec)


def _fmt_run_duration_detail(payload: dict[str, Any]) -> str:
    modes = payload.get("mode_duration_sec") or {}
    if isinstance(modes, dict) and modes:
        return "; ".join(f"{k} {_fmt_duration(v)}" for k, v in modes.items())
    return "wall clock"


def _run_duration_sec(payload: dict[str, Any]) -> float | None:
    modes = payload.get("mode_duration_sec") or {}
    if isinstance(modes, dict) and modes:
        try:
            return sum(float(v) for v in modes.values())
        except (TypeError, ValueError):
            pass
    sec = payload.get("duration_sec")
    try:
        return float(sec) if sec is not None else None
    except (TypeError, ValueError):
        return None


def _arm_wall_sec(block: dict[str, Any] | None, arm: str) -> float | None:
    if not block:
        return None
    rows = [r for r in (block.get("rows") or []) if isinstance(r, dict) and r.get("arm") == arm]
    row_walls = [float(r["wall_sec"]) for r in rows if r.get("wall_sec") is not None]
    raw = (block.get("summary") or {}).get(f"{arm}_wall_sec")
    if raw is not None:
        try:
            val = float(raw)
        except (TypeError, ValueError):
            val = None
        else:
            if row_walls or val > 0 or rows:
                return val
            return None
    if row_walls:
        return sum(row_walls)
    return None


def _duration_breakdown(
    payload: dict[str, Any], cmp_: dict[str, Any], swe: dict[str, Any]
) -> dict[str, float | None]:
    combined_sec = _run_duration_sec(payload)
    native = 0.0
    superopen = 0.0
    native_known = False
    so_known = False
    for block in (cmp_, swe):
        n_wall = _arm_wall_sec(block, "native")
        s_wall = _arm_wall_sec(block, "superopen")
        if n_wall is not None:
            native += n_wall
            native_known = True
        if s_wall is not None:
            superopen += s_wall
            so_known = True
    extras: float | None = None
    if combined_sec is not None and (native_known or so_known):
        extras = max(0.0, combined_sec - native - superopen)
    return {
        "native": native if native_known else None,
        "superopen": superopen if so_known else None,
        "extras": extras,
    }


def _fmt_total_cost(payload: dict[str, Any]) -> str:
    usd = payload.get("total_cost_usd")
    if usd is None:
        cmp_ = _compare_summary(payload)
        summary = cmp_.get("summary") or {}
        n = summary.get("native_cost_usd")
        s = summary.get("superopen_cost_usd")
        try:
            usd = float(n or 0) + float(s or 0)
        except (TypeError, ValueError):
            usd = None
        if usd == 0:
            usd = None
    if usd is None:
        return PENDING
    return f"${float(usd):.2f}"


def _fmt_index_time(idx: dict[str, Any]) -> str:
    elapsed = idx.get("elapsed_sec")
    if elapsed is None:
        return PENDING
    return _fmt_duration(elapsed)


def _scale_note(payload: dict[str, Any], locomo: dict[str, Any]) -> str:
    scale = payload.get("scale") or locomo.get("scale") or "small"
    return (
        f"This run is `--scale {scale}` (LOCOMO n=100 stratified, "
        "LME 50, compare 6, graph 12). Locomo is capped at 100."
    )


def _today() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%d")


def collect_machine() -> dict[str, Any]:
    import os
    import platform
    import subprocess

    info: dict[str, Any] = {
        "hostname": platform.node() or "unknown",
        "system": platform.system(),
        "release": platform.release(),
        "machine": platform.machine(),
        "cpu_count": os.cpu_count() or 0,
        "cpu": platform.processor() or platform.machine(),
        "mem_gb": None,
        "claude_code": "2.1.241",
    }
    try:
        from docker import CLAUDE_CODE_PIN

        info["claude_code"] = CLAUDE_CODE_PIN
    except Exception:
        pass
    if platform.system() == "Darwin":
        mac = platform.mac_ver()[0]
        if mac:
            info["release"] = mac
        try:
            info["cpu"] = subprocess.check_output(
                ["sysctl", "-n", "machdep.cpu.brand_string"], text=True, timeout=2
            ).strip()
        except Exception:
            pass
        try:
            mem = subprocess.check_output(["sysctl", "-n", "hw.memsize"], text=True, timeout=2).strip()
            info["mem_gb"] = round(int(mem) / (1024**3), 1)
        except Exception:
            pass
    else:
        try:
            for line in Path("/proc/meminfo").read_text().splitlines():
                if line.startswith("MemTotal:"):
                    info["mem_gb"] = round(int(line.split()[1]) / (1024**2), 1)
                    break
        except Exception:
            pass
    return info


def _as_usd(value: Any) -> float:
    try:
        return float(value or 0)
    except (TypeError, ValueError):
        return 0.0


def _cost_breakdown(
    payload: dict[str, Any],
    locomo: dict[str, Any],
    lme: dict[str, Any],
    cmp_: dict[str, Any],
    swe: dict[str, Any],
) -> dict[str, Any]:
    native = 0.0
    superopen = 0.0
    extras = 0.0
    parts: list[str] = []

    if cmp_:
        native += _as_usd((cmp_.get("summary") or {}).get("native_cost_usd"))
        superopen += _as_usd((cmp_.get("summary") or {}).get("superopen_cost_usd"))
    if swe:
        native += _as_usd((swe.get("summary") or {}).get("native_cost_usd"))
        superopen += _as_usd((swe.get("summary") or {}).get("superopen_cost_usd"))
        if (swe.get("summary") or {}).get("graded"):
            parts.append("SWE eval $0.00")

    def _qa_costs(block: dict[str, Any], label: str) -> None:
        nonlocal superopen, extras
        qa = _qa_llm(block)
        agent = sum(_as_usd(r.get("cost_usd")) for r in (qa.get("rows") or []) if isinstance(r, dict))
        superopen += agent
        judge = _as_usd(qa.get("judge_usd"))
        if not judge:
            judge = sum(_as_usd(r.get("judge_usd")) for r in (qa.get("rows") or []) if isinstance(r, dict))
        ingest = _as_usd((block.get("ingest") or {}).get("llm_usd"))
        extras += judge + ingest
        if judge:
            parts.append(f"{label} judge {_fmt_usd(judge)}")
        if ingest:
            parts.append(f"{label} ingest {_fmt_usd(ingest)}")

    if locomo:
        _qa_costs(locomo, "LOCOMO")
    if lme:
        _qa_costs(lme, "LME")

    combined = native + superopen + extras
    ledger = payload.get("total_cost_usd")
    if ledger is not None:
        led = _as_usd(ledger)
        if led > combined + 0.0001:
            extras += led - combined
            combined = led
            parts.append("other ledger")
        elif combined == 0 and led:
            combined = led
    known = bool(native or superopen or extras or ledger is not None)
    return {
        "native": native,
        "superopen": superopen,
        "extras": extras,
        "combined": combined,
        "parts": parts,
        "known": known,
    }


def _recall_sort_key(block: dict[str, Any], name: str, k: int = 10) -> float:
    row = (block.get("adapters") or {}).get(name) or {}
    try:
        return float(row.get(f"recall_at_{k}"))
    except (TypeError, ValueError):
        return float("-inf")


def _adapter_recall_cells(block: dict[str, Any], name: str) -> tuple[str, str]:
    if name == "bow":
        return _bow_row(block, 5), _bow_row(block, 10)
    return _adapter_row(block, name, 5), _adapter_row(block, name, 10)


def _adapter_table(block: dict[str, Any] | None) -> str:
    block = block or {}
    ingest = _fmt_ingest(block)
    has_ingest = ingest != PENDING
    systems = [
        ("superopen", "**Superopen** (`so memory recall`)"),
        ("bm25", "BM25"),
        ("bow", "bow (bag-of-words)"),
        ("rrf", "RRF"),
    ]
    rest = [row for row in systems if row[0] != "superopen"]
    rest.sort(key=lambda row: _recall_sort_key(block, row[0]), reverse=True)
    ordered = [systems[0], *rest]
    if has_ingest:
        lines = ["| System | recall@5 | recall@10 | Ingest cost |", "|---|---|---|---|"]
    else:
        lines = ["| System | recall@5 | recall@10 |", "|---|---|---|"]
    for name, label in ordered:
        r5, r10 = _adapter_recall_cells(block, name)
        if name == "superopen" and r5 != PENDING:
            r5 = f"**{r5}**"
        if name == "superopen" and r10 != PENDING:
            r10 = f"**{r10}**"
        if has_ingest:
            cost = ingest if name == "superopen" else "$0 (shared index)"
            if name != "superopen" and r10 == PENDING:
                cost = PENDING
            lines.append(f"| {label} | {r5} | {r10} | {cost} |")
        else:
            lines.append(f"| {label} | {r5} | {r10} |")
    return "\n".join(lines)


def _memory_compared_with(block: dict[str, Any] | None) -> str:
    block = block or {}
    parts: list[str] = []
    for name, label in (("bm25", "BM25"), ("rrf", "RRF"), ("bow", "bow")):
        val = _bow_row(block, 10) if name == "bow" else _adapter_row(block, name, 10)
        if val != PENDING:
            parts.append(f"{label} {val}")
    return ", ".join(parts) if parts else PENDING


def _glance_swe_cells(swe: dict[str, Any]) -> tuple[str, str, str, str, str]:
    summary = swe.get("summary") or {}
    n = summary.get("n")
    dataset = f"Verified (n={n})" if n else "Verified"
    if not swe or summary.get("superopen_tokens") is None:
        return dataset, PENDING, f"native {PENDING}", PENDING, f"native {PENDING}"
    if summary.get("graded"):
        so_c = _fmt_count_pct(summary.get("superopen_resolved"), int(n or 0))
        nat_c = _fmt_count_pct(summary.get("native_resolved"), int(n or 0))
    else:
        so_c = nat_c = PENDING
    so_usd = _fmt_usd(summary.get("superopen_cost_usd")) or PENDING
    nat_usd = _fmt_usd(summary.get("native_cost_usd")) or PENDING
    return dataset, so_c, f"native {nat_c}", so_usd, f"native {nat_usd}"


def _glance_table(
    locomo: dict[str, Any],
    lme: dict[str, Any],
    cmp_: dict[str, Any],
    swe: dict[str, Any],
    graph_s: str,
    django_tag: str,
    locomo_n: Any,
) -> str:
    swe_ds, so_c, nat_c, so_usd, nat_usd = _glance_swe_cells(swe)
    locomo_ds = f"LOCOMO (n={locomo_n})"
    cov_s = (cmp_.get("summary") or {}).get("superopen_coverage_avg")
    cov_n = (cmp_.get("summary") or {}).get("native_coverage_avg")
    if cov_s is None:
        sess_so, sess_nat = PENDING, f"native {PENDING}"
    else:
        sess_so = f"{float(cov_s):.2f}"
        sess_nat = f"native {float(cov_n):.2f}" if cov_n is not None else "—"
    so_sess_usd = _fmt_usd((cmp_.get("summary") or {}).get("superopen_cost_usd"))
    nat_sess_usd = _fmt_usd((cmp_.get("summary") or {}).get("native_cost_usd"))
    if so_sess_usd:
        sess_usd = so_sess_usd
        sess_usd_nat = f"native {nat_sess_usd}" if nat_sess_usd else "—"
    else:
        sess_usd, sess_usd_nat = PENDING, f"native {PENDING}"
    return "\n".join(
        [
            "| Suite | Dataset (n) | Metric | Superopen | Compared with |",
            "|---|---|---|---|---|",
            f"| SWE-bench | {swe_ds} | Correctness | {so_c} | {nat_c} |",
            f"| SWE-bench | {swe_ds} | Cost | {so_usd} | {nat_usd} |",
            f"| Memory | {locomo_ds} | recall@10 | {_fmt_recall(locomo, k=10)} | {_memory_compared_with(locomo)} |",
            f"| Memory | {locomo_ds} | QA accuracy | {_fmt_qa_short(locomo)} | — |",
            f"| Memory | LongMemEval-S (50) | recall@10 | {_fmt_recall(lme, k=10)} | {_memory_compared_with(lme)} |",
            f"| Memory | LongMemEval-S (50) | QA accuracy | {_fmt_qa_short(lme)} | — |",
            f"| Graph | Django {django_tag} | 12-probe | {graph_s} | — |",
            f"| Sessions | Django (6) | Key-fact coverage | {sess_so} | {sess_nat} |",
            f"| Sessions | Django (6) | Cost | {sess_usd} | {sess_usd_nat} |",
        ]
    )


def _system_table(payload: dict[str, Any]) -> str:
    machine = payload.get("machine") if isinstance(payload.get("machine"), dict) else {}
    if not machine:
        machine = collect_machine()
    host = payload.get("host") or "claude-code"
    model = payload.get("model") or "claude-sonnet-5"
    scale = payload.get("scale") or "small"
    cpu = machine.get("cpu") or machine.get("machine") or "unknown"
    cores = machine.get("cpu_count") or "?"
    arch = machine.get("machine") or ""
    mem = machine.get("mem_gb")
    mem_s = f"{mem:g} GB" if mem is not None else "unknown RAM"
    hostname = machine.get("hostname") or "unknown"
    os_name = machine.get("system") or ""
    release = machine.get("release") or ""
    os_line = f"{os_name} {release}".strip()
    if os_name == "Darwin":
        os_line = f"macOS {release} (darwin)"
    pin = machine.get("claude_code") or "2.1.241"
    return f"""| | |
|---|---|
| Machine | {hostname} · {cpu} · {cores}-core {arch} · {mem_s} |
| OS | {os_line} |
| Agent | Claude Code {pin} · `{model}` ({host}) |
| Scale | {scale} (LOCOMO n=100, LME n=50, compare 6, graph 12, SWE-bench 5/50 issues) |"""


def _run_table(payload: dict[str, Any], costs: dict[str, Any]) -> str:
    duration = _fmt_run_duration(payload)
    detail = _fmt_run_duration_detail(payload)
    if duration != PENDING and detail not in {PENDING, "wall clock"}:
        duration = f"{duration} ({detail})"
    walls = _duration_breakdown(payload, _compare_summary(payload), _swe_summary(payload))
    native_dur = _fmt_duration(walls["native"]) if walls["native"] is not None else PENDING
    so_dur = _fmt_duration(walls["superopen"]) if walls["superopen"] is not None else PENDING
    extras_dur = _fmt_duration(walls["extras"]) if walls["extras"] is not None else PENDING
    if costs.get("known"):
        native = _fmt_usd(costs["native"]) or "$0.00"
        so_usd = _fmt_usd(costs["superopen"]) or "$0.00"
        combined = _fmt_usd(costs["combined"]) or "$0.00"
        extras = _fmt_usd(costs["extras"]) or "$0.00"
        if costs.get("parts"):
            extras = f"{extras} ({'; '.join(costs['parts'])})"
    else:
        native = so_usd = extras = combined = PENDING
    return f"""| Metric | Combined | Native | Superopen | Extras |
|---|---|---|---|---|
| Duration | {duration} | {native_dur} | {so_dur} | {extras_dur} |
| Cost | {combined} | {native} | {so_usd} | {extras} |"""


def render(payload: dict[str, Any]) -> str:
    locomo = _memory_split(payload, "locomo")
    lme = _memory_split(payload, "longmemeval")
    cmp_ = _compare_summary(payload)
    swe = _swe_summary(payload)
    idx = _index_block(payload)
    locomo_n = locomo.get("n") or payload.get("n") or 100
    locomo_scored = ((locomo.get("adapters") or {}).get("superopen") or {}).get("total")
    locomo_label = f"LOCOMO (n={locomo_n})"
    if locomo_scored:
        locomo_label = f"LOCOMO (n={locomo_n} asked, {locomo_scored} scored)"

    locomo_qa = _fmt_qa(locomo)
    lme_qa = _fmt_qa(lme)
    rescue = _fmt_contradict_metric(payload, "rescue_at_10")
    verbatim = _fmt_contradict_metric(payload, "historical_verbatim")
    graph_s = _graph_score(payload)
    graph_lat = _graph_p95(payload)
    locomo_store = _fmt_gold_store(locomo)
    lme_store = _fmt_gold_store(lme)
    cov = _fmt_compare_coverage(cmp_)
    usd = _fmt_compare_usd(cmp_)
    cache = _fmt_compare_cache(cmp_)
    swe_n = (swe.get("summary") or {}).get("n") or (swe.get("n") if swe else None)
    swe_heading = f"### SWE-bench (n={swe_n})" if swe_n else "### SWE-bench"
    swe_tables = _swe_instance_tables(swe) if swe else "pending"
    latency = _fmt_latency(payload)
    index_time = _fmt_index_time(idx)
    costs = _cost_breakdown(payload, locomo, lme, cmp_, swe)

    index_line = PENDING
    if idx:
        bits = []
        if idx.get("nodes") is not None:
            bits.append(f"{int(idx['nodes']):,} nodes")
        if idx.get("edges") is not None:
            bits.append(f"{int(idx['edges']):,} edges")
        if idx.get("files") is not None:
            bits.append(f"{int(idx['files']):,} files")
        if bits:
            index_line = ", ".join(bits)

    def _int_cell(val: Any) -> str:
        if val is None:
            return "—"
        try:
            return f"{int(val):,}"
        except (TypeError, ValueError):
            return str(val)

    temporal = payload.get("temporal") or {}
    checkpoints = temporal.get("checkpoints") if isinstance(temporal, dict) else None
    if checkpoints:
        rows = ["| Checkpoint | Nodes | Edges | Files | `so init` |", "|---|---:|---:|---:|---:|"]
        for row in checkpoints:
            elapsed = row.get("elapsed_sec")
            elapsed_s = f"{float(elapsed):.0f}s" if elapsed is not None else "—"
            rows.append(
                f"| {row.get('tag') or '?'} | {_int_cell(row.get('nodes'))} | "
                f"{_int_cell(row.get('edges'))} | {_int_cell(row.get('files'))} | {elapsed_s} |"
            )
        temporal_table = "\n".join(rows)
    else:
        temporal_table = "pending"

    django_tag = idx.get("tag") or (payload.get("graph") or {}).get("tag") or "5.2.4"
    compare_dur = _fmt_duration((cmp_ or {}).get("duration_sec")) if cmp_ else PENDING
    locomo_note = f"\n\n{locomo_store}" if locomo_store else ""
    lme_note = f"\n\n{lme_store}" if lme_store else ""
    glance = _glance_table(locomo, lme, cmp_, swe, graph_s, django_tag, locomo_n)

    md = f"""# Superopen Benchmarks

Last updated: {_today()}. How to run: [benchmarks/README.md](benchmarks/README.md).

## System

{_system_table(payload)}

## Run

{_run_table(payload, costs)}

## Results at a glance

{glance}

## Results

{swe_heading}

{swe_tables}

### Memory

#### {locomo_label}

{_adapter_table(locomo)}

QA accuracy: {locomo_qa}.{locomo_note}

#### LongMemEval-S (50)

{_adapter_table(lme)}

QA accuracy: {lme_qa}.{lme_note}

#### Contradiction

| Metric | Superopen |
|---|---|
| Rescue@10 | {rescue} |
| historical-verbatim | {verbatim} |

### Graph (Django {django_tag})

| Metric | Score |
|---|---|
| Index time | {index_time} |
| Index size | {index_line} |
| 12-probe score | {graph_s} |
| Probe latency | {graph_lat} |

### Sessions (Django, 6)

| Metric | Score |
|---|---|
| Duration | {compare_dur} |
| Key-fact coverage | {cov} |
| USD | {usd} |
| cache_read tokens | {cache} |

### Temporal (Django LTS)

{temporal_table}

### Latency

| Metric | Score |
|---|---|
| `so memory search` | {latency} |
"""
    return md.rstrip() + "\n"


def write_benchmarks_md(
    source: Path | dict[str, Any],
    *,
    dest: Path | None = None,
    fill_from: list[Path] | None = None,
) -> Path:
    if isinstance(source, dict):
        payload = deepcopy(source)
    else:
        payload = load_stamp(source)
        if fill_from:
            payload = merge_stamps(payload, [load_stamp(p) for p in fill_from])
    text = render(payload)
    out_path = dest or (REPO / "BENCHMARKS.md")
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(text)
    return out_path


def parse_fill_from(raw: str) -> list[Path]:
    out: list[Path] = []
    for part in raw.split(","):
        part = part.strip()
        if part:
            out.append(Path(part))
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate BENCHMARKS.md")
    parser.add_argument("--out", default=None, help=argparse.SUPPRESS)
    parser.add_argument(
        "--fill-from",
        default="",
        dest="fill_from",
        help=argparse.SUPPRESS,
    )
    parser.add_argument("--dest", default=None, help="Markdown path (default: repo-root BENCHMARKS.md)")
    args = parser.parse_args()
    if not args.out:
        dest = write_benchmarks_md({}, dest=Path(args.dest) if args.dest else None)
    else:
        dest = write_benchmarks_md(
            Path(args.out),
            dest=Path(args.dest) if args.dest else None,
            fill_from=parse_fill_from(args.fill_from),
        )
    print(dest)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
