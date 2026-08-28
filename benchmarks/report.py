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
    if src.get(key):
        dst[key] = deepcopy(src[key])


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
        for key in ("graph", "compare", "contradict", "latency", "index", "temporal"):
            _fill_key(out, extra, key)
    out["_sources"] = [s for s in sources if s]
    return out


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
        return "BM25 / dense / RRF"
    parts = [
        f"BM25 {_adapter_row(block, 'bm25', k)}",
        f"dense {_adapter_row(block, 'dense', k)}",
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


def _fmt_ingest(block: dict[str, Any] | None) -> str:
    if not block:
        return PENDING
    ingest = block.get("ingest") or {}
    if not ingest:
        return PENDING
    usd = ingest.get("llm_usd")
    if usd is None:
        usd = 0.0
    return f"${float(usd):.2f}"


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
    sec = payload.get("duration_sec")
    if sec is None:
        return PENDING
    return _fmt_duration(sec)


def _fmt_run_duration_detail(payload: dict[str, Any]) -> str:
    modes = payload.get("mode_duration_sec") or {}
    if isinstance(modes, dict) and modes:
        return "; ".join(f"{k} {_fmt_duration(v)}" for k, v in modes.items())
    return "wall clock"


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


def _system_table(payload: dict[str, Any]) -> str:
    machine = payload.get("machine") if isinstance(payload.get("machine"), dict) else {}
    if not machine:
        machine = collect_machine()
    host = payload.get("host") or "claude-code"
    model = payload.get("model") or "claude-sonnet-5"
    isolate = payload.get("isolate") or "docker"
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
    isolate_line = (
        f"{isolate} (`so-bench` image; host HOME not mounted)"
        if isolate == "docker"
        else f"{isolate} (isolated HOME on this machine)"
    )
    return f"""| | |
|---|---|
| Machine | {hostname} · {cpu} · {cores}-core {arch} · {mem_s} |
| OS | {os_line} |
| Isolate | {isolate_line} |
| Superopen | locally built CLI bind-mounted at `/usr/local/bin/so` |
| Agent | Claude Code {pin} · `{model}` ({host}) |
| Scale | {scale} (LOCOMO n=100, LME n=50, compare 6, graph 12) |"""


def render(payload: dict[str, Any]) -> str:
    locomo = _memory_split(payload, "locomo")
    lme = _memory_split(payload, "longmemeval")
    cmp_ = _compare_summary(payload)
    idx = _index_block(payload)
    scale = payload.get("scale") or locomo.get("scale") or lme.get("scale") or "small"
    host = payload.get("host") or (cmp_.get("host") if cmp_ else None) or "claude-code"
    model = payload.get("model") or (cmp_.get("model") if cmp_ else None) or "claude-sonnet-5"
    isolate = payload.get("isolate") or "docker"
    locomo_n = locomo.get("n") or payload.get("n") or 100
    locomo_scored = ((locomo.get("adapters") or {}).get("superopen") or {}).get("total")
    locomo_label = f"LOCOMO (n={locomo_n})"
    if locomo_scored:
        locomo_label = f"LOCOMO (n={locomo_n} asked, {locomo_scored} scored)"

    locomo_r5 = _fmt_recall(locomo, k=5)
    locomo_r10 = _fmt_recall(locomo, k=10)
    locomo_bm5 = _adapter_row(locomo, "bm25", 5)
    locomo_bm10 = _adapter_row(locomo, "bm25", 10)
    locomo_dense5 = _adapter_row(locomo, "dense", 5)
    locomo_dense10 = _adapter_row(locomo, "dense", 10)
    locomo_rrf5 = _adapter_row(locomo, "rrf", 5)
    locomo_rrf10 = _adapter_row(locomo, "rrf", 10)
    locomo_qa = _fmt_qa(locomo)
    lme_r5 = _fmt_recall(lme, k=5)
    lme_r10 = _fmt_recall(lme, k=10)
    lme_bm5 = _adapter_row(lme, "bm25", 5)
    lme_bm10 = _adapter_row(lme, "bm25", 10)
    lme_dense5 = _adapter_row(lme, "dense", 5)
    lme_dense10 = _adapter_row(lme, "dense", 10)
    lme_rrf5 = _adapter_row(lme, "rrf", 5)
    lme_rrf10 = _adapter_row(lme, "rrf", 10)
    lme_qa = _fmt_qa(lme)
    rescue = _fmt_contradict_metric(payload, "rescue_at_10")
    verbatim = _fmt_contradict_metric(payload, "historical_verbatim")
    ingest_usd = _fmt_ingest(locomo) if locomo else _fmt_ingest(lme)
    graph_s = _graph_score(payload)
    cov = _fmt_compare_coverage(cmp_)
    usd = _fmt_compare_usd(cmp_)
    cache = _fmt_compare_cache(cmp_)
    latency = _fmt_latency(payload)
    run_duration = _fmt_run_duration(payload)
    run_duration_detail = _fmt_run_duration_detail(payload)
    total_cost = _fmt_total_cost(payload)
    index_time = _fmt_index_time(idx)

    index_line = PENDING
    if idx:
        nodes, edges, files = idx.get("nodes"), idx.get("edges"), idx.get("files")
        elapsed = idx.get("elapsed_sec")
        bits = []
        if nodes is not None:
            bits.append(f"{int(nodes):,} nodes")
        if edges is not None:
            bits.append(f"{int(edges):,} edges")
        if files is not None:
            bits.append(f"{int(files):,} files")
        if bits:
            index_line = ", ".join(bits)

    temporal = payload.get("temporal") or {}
    checkpoints = temporal.get("checkpoints") if isinstance(temporal, dict) else None
    temporal_table = ""
    if checkpoints:
        rows = ["| Checkpoint | Nodes | Edges | Files | `so init` |", "|---|---:|---:|---:|---:|"]
        for row in checkpoints:
            def _cell(key: str) -> str:
                val = row.get(key)
                return "—" if val is None else str(val)

            elapsed = row.get("elapsed_sec")
            elapsed_s = f"{float(elapsed):.0f}s" if elapsed is not None else "—"
            rows.append(
                f"| {row.get('tag') or '?'} | {_cell('nodes')} | {_cell('edges')} | {_cell('files')} | {elapsed_s} |"
            )
        temporal_table = "\n".join(rows)
    else:
        temporal_table = "pending"

    django_tag = idx.get("tag") or (payload.get("graph") or {}).get("tag") or "5.2.4"
    django_ds = f"Django {django_tag}"
    locomo_repro_n = locomo.get("n") or 100
    compare_dur = _fmt_duration((cmp_ or {}).get("duration_sec")) if cmp_ else PENDING

    md = f"""# Superopen Benchmarks

Last updated: {_today()}. Generated by `python3 benchmarks/run.py`.

## System

{_system_table(payload)}

## Run

| Metric | Value | Detail |
|---|---|---|
| Duration | {run_duration} | {run_duration_detail} |
| Total cost | {total_cost} | spend ledger |
| Graph index | {index_time} | `so init`, $0 LLM |

## Results at a glance

| Suite | Dataset | Metric | Score |
|---|---|---|---|
| Graph | {django_ds} | index time | {index_time} |
| Graph | {django_ds} | index size | {index_line} |
| Graph | {django_ds} | 12-probe score | {graph_s} |
| Memory | {locomo_label} | QA accuracy | {locomo_qa} |
| Memory | {locomo_label} | recall@10 | {locomo_r10} |
| Memory | {locomo_label} | recall@5 | {locomo_r5} |
| Memory | LongMemEval-S (50) | QA accuracy | {lme_qa} |
| Memory | LongMemEval-S (50) | recall@10 | {lme_r10} |
| Memory | LongMemEval-S (50) | recall@5 | {lme_r5} |
| Memory | Contradiction | Rescue@10 | {rescue} |
| Memory | Contradiction | historical-verbatim | {verbatim} |
| Cost | memory ingest | USD | {ingest_usd} |
| Cost | graph build | LLM credits | $0 |
| Session | Django (6) | duration | {compare_dur} |
| Session | Django (6) | key-fact coverage | {cov} |
| Session | Django (6) | USD | {usd} |
| Session | Django (6) | cache_read tokens | {cache} |
| Latency | local fixture | `so memory search` | {latency} |

## Datasets

| Dataset | What we score | Notes |
|---|---|---|
| LOCOMO (`locomo10.json`) | 100 category-stratified QA over the full session corpus | File has 300 items; n cap is 100 |
| LongMemEval-S | 50 English questions | Haystack is not shrunk |
| Django | tag `{django_tag}` | Graph probes + native vs Superopen sessions |
| Contradiction | Rescue@10, historical-verbatim | Go tests in `internal/memory/` |

## Conversational memory

### LOCOMO

| Adapter | recall@5 | recall@10 |
|---|---|---|
| Superopen (`so memory recall`) | {locomo_r5} | {locomo_r10} |
| BM25 | {locomo_bm5} | {locomo_bm10} |
| dense | {locomo_dense5} | {locomo_dense10} |
| RRF | {locomo_rrf5} | {locomo_rrf10} |

QA accuracy: {locomo_qa}.

```bash
python3 benchmarks/run.py --mode memory --phase 2 --split locomo --scale small --so-bin ./bin/so
python3 benchmarks/run.py --mode memory --phase 3 --split locomo --scale small --qa-n 20 --host claude-code --model claude-sonnet-5 --max-spend 15 --so-bin ./bin/so
```

Asked n={locomo_repro_n}. Items without a mappable gold episode id are not scored.

### LongMemEval-S

| Adapter | recall@5 | recall@10 |
|---|---|---|
| Superopen (`so memory recall`) | {lme_r5} | {lme_r10} |
| BM25 | {lme_bm5} | {lme_bm10} |
| dense | {lme_dense5} | {lme_dense10} |
| RRF | {lme_rrf5} | {lme_rrf10} |

QA accuracy: {lme_qa}.

```bash
python3 benchmarks/run.py --mode memory --phase 2 --split longmemeval --scale small --so-bin ./bin/so
```

### Contradiction

| Metric | Superopen |
|---|---|
| Rescue@10 | {rescue} |
| historical-verbatim | {verbatim} |

```bash
python3 benchmarks/run.py --mode contradict --seeds 13,42,137
```

## Code intelligence

### Graph-tools (Django, 12 probes)

Score: **{graph_s}**. Index: {index_line}.

```bash
python3 benchmarks/run.py --mode graph --repo django --index-timeout 1800 --so-bin ./bin/so
```

### Native vs Superopen sessions (Django, 6)

key-fact coverage: **{cov}**.

USD: **{usd}**.

cache_read: **{cache}**.

```bash
python3 benchmarks/run.py --mode compare --scale small --host claude-code --model claude-sonnet-5 --max-turns 14 --max-spend 20 --so-bin ./bin/so
```

## Temporal (Django LTS)

{temporal_table}

```bash
python3 benchmarks/run.py --mode temporal --repo django --so-bin ./bin/so
```

## Latency

{latency}

```bash
python3 benchmarks/run.py --mode latency --so-bin ./bin/so
```
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
