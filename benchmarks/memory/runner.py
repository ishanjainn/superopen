"""Memory benchmark runner: ingest → index → search → answer → grade."""

from __future__ import annotations

import json
import os
import shutil
import sqlite3
import statistics
import subprocess
import time
from pathlib import Path
from typing import Any

from grade import grade_answer, recall_any_at_k, wrap_prompt
from memory.adapters.bm25 import BM25Index, dense_search, rrf_merge
from memory.adapters import superopen as so_adapter
from memory.fetch_datasets import dataset_path, ensure_readme
from spend import SpendLedger


def _observation_text(obs: Any) -> str:
    if isinstance(obs, str):
        return obs.strip()
    if isinstance(obs, dict):
        parts: list[str] = []
        for speaker, rows in obs.items():
            if not isinstance(rows, list):
                continue
            for row in rows:
                if isinstance(row, list) and row:
                    parts.append(f"{speaker}: {row[0]}")
                elif isinstance(row, str):
                    parts.append(f"{speaker}: {row}")
        return "\n".join(parts)
    return ""


def _session_text(session: Any) -> str:
    if isinstance(session, str):
        return session
    if isinstance(session, dict):
        if session.get("content"):
            return str(session.get("content"))
        turns = session.get("turns") or session.get("messages") or session.get("haystack_session")
        if turns:
            return _session_text(turns)
        return json.dumps(session)
    if isinstance(session, list):
        lines: list[str] = []
        for turn in session:
            if isinstance(turn, dict):
                role = turn.get("role") or turn.get("speaker") or ""
                content = turn.get("content") or turn.get("text") or ""
                if isinstance(content, list):
                    content = " ".join(str(x) for x in content)
                lines.append(f"{role}: {content}".strip())
            else:
                lines.append(str(turn))
        return "\n".join(lines)
    return str(session or "")


def _evidence_to_doc_id(sample_id: str, evidence: str) -> str:
    session = evidence.split(":")[0].lstrip("D")
    return f"{sample_id}:session_{session}"


def _locomo_session_date(conv: dict[str, Any], n: str) -> str:
    for key in (f"session_{n}_date_time", f"session_{n}_date", f"session_{n}_datetime"):
        val = conv.get(key)
        if val:
            return str(val)
    return ""


def _flatten_locomo(data: list[dict[str, Any]], n: int | None = None) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    qa_items: list[dict[str, Any]] = []
    docs: list[dict[str, Any]] = []
    seen_docs: set[str] = set()
    for sample in data:
        sid = str(sample.get("sample_id", len(docs)))
        conv = sample.get("conversation") or {}
        if isinstance(conv, dict):
            for key, val in conv.items():
                if not isinstance(key, str) or not key.startswith("session_"):
                    continue
                rest = key[len("session_") :]
                if not rest.isdigit():
                    continue
                text = _session_text(val)
                if not text.strip():
                    continue
                date = _locomo_session_date(conv, rest)
                if date:
                    text = f"DATE: {date}\n{text}"
                doc_id = f"{sid}:session_{rest}"
                if doc_id not in seen_docs:
                    seen_docs.add(doc_id)
                    docs.append({"id": doc_id, "text": text, "gold": [doc_id], "session": sid})
        for qa in sample.get("qa") or []:
            if not isinstance(qa, dict):
                continue
            evidence = qa.get("evidence") or []
            gold = [_evidence_to_doc_id(sid, e) for e in evidence if isinstance(e, str)]
            qa_items.append(
                {
                    "id": f"{sid}:{len(qa_items)}",
                    "question": qa.get("question", ""),
                    "answer": qa.get("answer", ""),
                    "gold": gold,
                    "category": qa.get("category"),
                }
            )
    if n is None or n >= len(qa_items):
        return qa_items, docs
    return _qa_sample(qa_items, n, "locomo"), docs


def _flatten_longmemeval(data: Any, n: int) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    rows = data if isinstance(data, list) else data.get("questions") or data.get("samples") or []
    qa_items: list[dict[str, Any]] = []
    docs: list[dict[str, Any]] = []
    seen: set[str] = set()
    for i, row in enumerate(rows):
        if not isinstance(row, dict):
            continue
        qid = str(row.get("question_id") or row.get("id") or i)
        sessions = row.get("haystack_sessions") or []
        sids = row.get("haystack_session_ids") or []
        for j, sess in enumerate(sessions):
            sid = str(sids[j] if j < len(sids) else f"{qid}:s{j}")
            if sid in seen:
                continue
            seen.add(sid)
            docs.append({"id": sid, "text": _session_text(sess), "gold": [sid], "session": qid})
        gold = row.get("answer_session_ids") or row.get("haystack_session_ids") or []
        if isinstance(gold, str):
            gold = [gold]
        qa_items.append(
            {
                "id": qid,
                "question": str(row.get("question") or ""),
                "answer": str(row.get("answer") or row.get("gold_answer") or ""),
                "gold": [str(g) for g in gold[:8]],
            }
        )
        if len(qa_items) >= n:
            break
    return qa_items[:n], docs


def _synthetic_corpus(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
    docs: list[dict[str, Any]] = []
    for i, item in enumerate(items):
        text = item.get("text") or item.get("context") or item.get("question") or json.dumps(item)
        docs.append({"id": str(item.get("id", i)), "text": str(text), "gold": item.get("gold") or []})
    return docs


def _load_split(split: str, n: int) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    path = dataset_path(split)
    if not path.is_file():
        ensure_readme()
        raise FileNotFoundError(f"dataset missing: {path}. See benchmarks/datasets/README.md")
    data = json.loads(path.read_text())
    if split == "locomo" and isinstance(data, list) and data and "qa" in data[0]:
        return _flatten_locomo(data, n)
    if split == "longmemeval":
        return _flatten_longmemeval(data, n)
    items = data if isinstance(data, list) else data.get("questions") or data.get("samples") or []
    items = items[:n]
    return items, _synthetic_corpus(items)


def _adapter_rank(
    name: str,
    item: dict[str, Any],
    so_bin: str,
    store: Path,
    bm25: BM25Index,
    docs: list[dict[str, Any]],
    id_to_docs: dict[int, str] | dict[int, list[str]] | None = None,
    env: dict[str, str] | None = None,
) -> list[str]:
    query = str(item.get("question") or item.get("query") or item.get("text") or "")
    if name == "superopen":
        return so_adapter.rank_doc_ids(so_bin, store, query, id_to_docs or {}, limit=10, env=env)
    if name == "bm25":
        return bm25.search(query, k=10)
    if name == "dense":
        return dense_search(docs, query, k=10)
    if name == "rrf":
        return rrf_merge([bm25.search(query, k=10), dense_search(docs, query, k=10)], k=10)
    raise ValueError(f"unknown adapter: {name}")


def _run_qa(
    items: list[dict[str, Any]],
    so_bin: str,
    store: Path,
    bm25: BM25Index,
    docs: list[dict[str, Any]],
    adapters: list[str],
    id_to_docs: dict[int, str] | dict[int, list[str]],
    env: dict[str, str] | None = None,
) -> dict[str, Any]:
    qa: dict[str, Any] = {}
    for name in adapters:
        covered = 0
        total = 0
        for item in items:
            query = str(item.get("question") or "")
            answer = str(item.get("answer") or "")
            if not answer:
                continue
            total += 1
            if name == "superopen":
                blob = so_adapter.recall_pack_text(so_bin, store, query, limit=10, env=env)
            else:
                ranked = _adapter_rank(name, item, so_bin, store, bm25, docs, id_to_docs, env=env)
                by_id = {d["id"]: d.get("text") or "" for d in docs}
                blob = "\n".join(by_id.get(r, "") for r in ranked)
            aliases = [answer]
            if len(answer) > 24:
                aliases.append(answer[:24])
            g = grade_answer(blob, [{"id": "gold", "aliases": aliases}])
            if g["coverage"] > 0:
                covered += 1
        qa[name] = {
            "accuracy": (covered / total) if total else None,
            "hits": covered,
            "total": total,
            "method": "extractive_from_retrieve",
        }
    return qa


def _qa_sample(items: list[dict[str, Any]], n: int, split: str) -> list[dict[str, Any]]:
    """Category-stratified locomo sample.

    A locomo `--qa-n 20` draw is 16 scored rows plus 4 empty-gold category-5
    items, and the scored rows in current dumps are all conv-26. That is a
    sampling artifact, not a spend stop (`skipped_no_gold` reports the empty
    golds). Do not expand the sample in this pass.
    """
    if n <= 0 or n >= len(items):
        return items
    if split != "locomo":
        return items[:n]
    buckets: dict[str, list[dict[str, Any]]] = {}
    order: list[str] = []
    for item in items:
        cat = str(item.get("category") if item.get("category") is not None else "na")
        if cat not in buckets:
            buckets[cat] = []
            order.append(cat)
        buckets[cat].append(item)
    out: list[dict[str, Any]] = []
    i = 0
    while len(out) < n:
        progressed = False
        for cat in order:
            rows = buckets[cat]
            if i < len(rows):
                out.append(rows[i])
                progressed = True
                if len(out) >= n:
                    break
        if not progressed:
            break
        i += 1
    return out


def _freeze_so(store: Path) -> Path:
    """Copy `.so/` after ingest so each QA session starts from the diary corpus."""
    src = store / ".so"
    dest = store.parent / f".{store.name}.qa-frozen.so"
    if dest.exists():
        shutil.rmtree(dest)
    if not src.is_dir():
        raise RuntimeError(f"memory QA needs a frozen .so/ store at {store}")
    shutil.copytree(src, dest, symlinks=True)
    return dest


def _restore_so(store: Path, frozen: Path) -> None:
    live = store / ".so"
    last_err: OSError | None = None
    for _ in range(8):
        try:
            if live.exists():
                shutil.rmtree(live)
            shutil.copytree(frozen, live, symlinks=True)
            return
        except OSError as exc:
            last_err = exc
            time.sleep(0.25)
    raise RuntimeError(f"could not restore frozen memory store: {last_err}") from last_err


def _qa_llm_payload(
    *,
    covered_strict: int,
    covered_judge: int,
    coverage_sum: float,
    judged: int,
    total: int,
    skipped_no_gold: int,
    timed_out: int,
    empty_pack: int,
    tools_likely: int,
    so_invoked_n: int,
    host_name: str,
    model: str,
    rows: list[dict[str, Any]],
    stopped: str | None = None,
) -> dict[str, Any]:
    out: dict[str, Any] = {
        "accuracy": (covered_strict / total) if total else None,
        "accuracy_strict": (covered_strict / total) if total else None,
        "accuracy_judge": (covered_judge / judged) if judged else None,
        "qa_accuracy": (coverage_sum / total) if total else None,
        "hits": covered_strict,
        "hits_strict": covered_strict,
        "hits_judge": covered_judge,
        "total": total,
        "skipped_no_gold": skipped_no_gold,
        "timed_out": timed_out,
        "empty_pack": empty_pack,
        "tools_likely": tools_likely,
        "verbose_output": tools_likely,
        "so_invoked": so_invoked_n,
        "method": "coding_agent_session",
        "host": host_name,
        "model": model,
        "note": "locomo qa_n=20 is 16 scored (often all conv-26) + 4 empty-gold category-5; skipped_no_gold is not a spend stop",
        "store_frozen_per_question": True,
        "rows": rows,
    }
    if stopped:
        out["stopped_on_spend"] = stopped
    return out


def _run_qa_llm(
    items: list[dict[str, Any]],
    so_bin: str,
    store: Path,
    args: Any,
    ledger: SpendLedger,
    out: Path,
) -> dict[str, Any]:
    import isolate
    from compare import _prepare_superopen
    from judge import judge as llm_judge

    host_name = getattr(args, "host", "claude-code")
    if host_name == "claude-code":
        from hosts import claude_code as llm_host
    elif host_name == "opencode":
        from hosts import opencode as llm_host
    else:
        return {"skipped": "coding-agent host required (claude-code or opencode)"}
    if not llm_host.available():
        return {"skipped": f"{host_name} not on PATH"}

    paths = getattr(args, "_memory_paths", None)
    env = getattr(args, "_memory_env", None)
    if paths is None or env is None:
        work = out / "memory_qa" / str(getattr(args, "split", "locomo"))
        paths = isolate.arm_paths(work, "superopen")
        paths["worktree"] = store
        isolate.ensure_dirs(paths, host_name)
        if host_name == "opencode":
            isolate.copy_auth(paths["opencode"])
        else:
            isolate.copy_auth(paths["claude"])
        env = isolate.arm_env(paths, {"SUPEROPEN_SO_BIN": so_bin})
        isolate.ensure_container(paths, so_bin)
    _prepare_superopen(so_bin, paths, env, host_name)

    covered_strict = 0
    covered_judge = 0
    coverage_sum = 0.0
    judged = 0
    total = 0
    skipped_no_gold = 0
    timed_out = 0
    empty_pack = 0
    tools_likely = 0
    so_invoked_n = 0
    rows: list[dict[str, Any]] = []
    stopped: str | None = None
    judge_cache = out / "judge_cache"
    frozen = _freeze_so(store)
    try:
        for item in items:
            query = str(item.get("question") or "")
            answer = str(item.get("answer") or "")
            if not answer:
                skipped_no_gold += 1
                continue
            _restore_so(store, frozen)
            result = llm_host.run_prompt(
                wrap_prompt(query),
                store,
                env,
                args.model,
                timeout=max(300, int(getattr(args, "agent_timeout", 0) or 0)),
            )
            usd = result.get("cost_usd")
            ledger.record("memory_qa_llm", usd, {"id": item.get("id")})
            total += 1
            verbose_output = int(result.get("output_tokens") or 0) >= 50
            if verbose_output:
                tools_likely += 1
            so_invoked = bool(result.get("so_invoked"))
            if so_invoked:
                so_invoked_n += 1
            hit_strict = False
            hit_judge: bool | None = None
            timed = False
            text = str(result.get("result") or "")
            if not result.get("ok") and "timeout" in str(result.get("stderr") or "").lower():
                timed_out += 1
                timed = True
            else:
                aliases = [answer]
                if len(answer) > 24:
                    aliases.append(answer[:24])
                g = grade_answer(text, [{"id": "gold", "aliases": aliases}])
                coverage_sum += float(g.get("coverage") or 0)
                if g["coverage"] > 0:
                    covered_strict += 1
                    hit_strict = True
                verdict = llm_judge(
                    query,
                    answer,
                    text,
                    str(item.get("id") or ""),
                    ledger,
                    judge_cache,
                )
                if verdict.get("hit") is not None:
                    judged += 1
                    hit_judge = bool(verdict["hit"])
                    if hit_judge:
                        covered_judge += 1
            rows.append(
                {
                    "id": item.get("id"),
                    "ok": result.get("ok"),
                    "hit": hit_strict,
                    "hit_strict": hit_strict,
                    "hit_judge": hit_judge,
                    "timeout": timed,
                    "tools_likely": verbose_output,
                    "verbose_output": verbose_output,
                    "so_invoked": so_invoked,
                    "input_tokens": result.get("input_tokens"),
                    "cache_read_tokens": result.get("cache_read_tokens"),
                    "output_tokens": result.get("output_tokens"),
                    "cost_usd": usd,
                    "answer_gold": answer[:80],
                    "result_head": text[:240],
                    "stderr_head": str(result.get("stderr") or "")[:400],
                }
            )
            try:
                ledger.check()
            except RuntimeError as exc:
                stopped = str(exc)
                break
    finally:
        try:
            _restore_so(store, frozen)
        except RuntimeError:
            pass
    return _qa_llm_payload(
        covered_strict=covered_strict,
        covered_judge=covered_judge,
        coverage_sum=coverage_sum,
        judged=judged,
        total=total,
        skipped_no_gold=skipped_no_gold,
        timed_out=timed_out,
        empty_pack=empty_pack,
        tools_likely=tools_likely,
        so_invoked_n=so_invoked_n,
        host_name=host_name,
        model=str(getattr(args, "model", "") or ""),
        rows=rows,
        stopped=stopped,
    )


def _id_map_from_store(store: Path) -> dict[int, list[str]]:
    db = store / ".so" / "db" / "so.db"
    if not db.is_file():
        return {}
    conn = sqlite3.connect(db)
    out: dict[int, list[str]] = {}
    for eid, title in conn.execute("SELECT id, title FROM memory_episodes"):
        if title:
            out.setdefault(int(eid), []).append(str(title))
    conn.close()
    return out


def _memory_arm(out: Path, split: str, so_bin: str, host_name: str, store: Path) -> tuple[dict[str, Path], dict[str, str]]:
    import isolate

    paths = isolate.arm_paths(out / "memory_iso" / split, "superopen")
    paths["worktree"] = store
    isolate.ensure_dirs(paths, host_name)
    if host_name == "opencode":
        isolate.copy_auth(paths["opencode"])
    else:
        isolate.copy_auth(paths["claude"])
    env = isolate.arm_env(paths, {"SUPEROPEN_SO_BIN": so_bin})
    isolate.ensure_container(paths, so_bin)
    return paths, env


def run_memory_mode(args: Any, out: Path, so_bin: str, ledger: SpendLedger) -> dict[str, Any]:
    items, docs = _load_split(args.split, args.n)
    if not docs:
        docs = _synthetic_corpus(items)
    bm25 = BM25Index(docs)
    adapters = [a.strip() for a in args.adapters.split(",") if a.strip()]
    results: dict[str, Any] = {
        "split": args.split,
        "n": len(items),
        "phase": args.phase,
        "scale": getattr(args, "scale", "small"),
        "adapters": {},
    }

    store = out / "memory" / args.split
    store.mkdir(parents=True, exist_ok=True)
    host_name = getattr(args, "host", "claude-code")
    paths, env = _memory_arm(out, str(args.split), so_bin, host_name, store)
    args._memory_paths = paths
    args._memory_env = env
    ingest = args.phase in (1, 3) or ("superopen" in adapters and args.phase >= 2)
    skip_ingest = os.environ.get("SO_BENCH_SKIP_INGEST") == "1"
    id_to_docs: dict[int, list[str]] = {}
    if ingest and not skip_ingest:
        import isolate

        isolate.run([so_bin, "init", "--root", str(store), "--force"], cwd=store, env=env)
        captures: list[dict[str, Any]] = []
        collapsed = 0
        skipped = 0
        unique_titles: set[str] = set()
        for doc in docs:
            rec = so_adapter.capture_episode(so_bin, store, doc["id"], doc["text"], env=env)
            captures.append({"requested": doc["id"], "stored_as": rec.get("title"), "id": rec.get("id"), "collapsed": rec.get("collapsed"), "ok": rec.get("ok")})
            if rec.get("id") is not None:
                id_to_docs.setdefault(int(rec["id"]), []).append(doc["id"])
            if rec.get("collapsed"):
                collapsed += 1
            if rec.get("skipped") or not rec.get("ok"):
                skipped += 1
            if rec.get("title"):
                unique_titles.add(rec["title"])
        eid = so_adapter.embedder_id(so_bin, store, env=env)
        results["embedder_id"] = eid
        results["ingest"] = {
            "store": str(store),
            "episodes_attempted": len(docs),
            "captures_ok": sum(1 for c in captures if c.get("ok")),
            "collapsed_near_duplicate": collapsed,
            "skipped": skipped,
            "unique_titles_returned": len(unique_titles),
            "stored_episode_ids": len(id_to_docs),
            "embedder_id": eid,
            "llm_usd": 0.0,
        }
        results["ingest_sample"] = captures[:12]
    elif skip_ingest and "superopen" in adapters:
        id_to_docs = _id_map_from_store(store)
        eid = so_adapter.embedder_id(so_bin, store, env=env)
        results["embedder_id"] = eid
        results["ingest"] = {
            "store": str(store),
            "skipped_reingest": True,
            "stored_episode_ids": len(id_to_docs),
            "embedder_id": eid,
            "llm_usd": 0.0,
        }

    if args.phase == 1:
        (out / f"memory_{args.split}.json").write_text(json.dumps(results, indent=2) + "\n")
        return results

    for name in adapters:
        hits5 = 0
        hits10 = 0
        total = 0
        for item in items:
            gold = item.get("gold_session_ids") or item.get("gold") or []
            ranked = _adapter_rank(name, item, so_bin, store, bm25, docs, id_to_docs, env=env)
            if gold:
                total += 1
                if recall_any_at_k(ranked, gold, 5):
                    hits5 += 1
                if recall_any_at_k(ranked, gold, 10):
                    hits10 += 1
        results["adapters"][name] = {
            "recall_at_5": (hits5 / total) if total else None,
            "recall_at_10": (hits10 / total) if total else None,
            "hits_at_5": hits5,
            "hits_at_10": hits10,
            "hits": hits10,
            "total": total,
        }

    if args.phase >= 3:
        qa_items = _qa_sample(items, int(getattr(args, "qa_n", 0) or 0), args.split)
        results["qa_n"] = len(qa_items)
        results["qa"] = _run_qa(qa_items, so_bin, store, bm25, docs, adapters, id_to_docs, env=env)
        results["qa"]["extractive_note"] = "debug metric; headline QA is qa_llm when --max-spend > 0"
        if ledger.allow_llm() and "superopen" in adapters:
            results["qa_llm"] = {"superopen": _run_qa_llm(qa_items, so_bin, store, args, ledger, out)}
        else:
            results["qa_llm"] = "skipped (pass --max-spend > 0 for a coding-agent session over the memory store)"

    (out / f"memory_{args.split}.json").write_text(json.dumps(results, indent=2) + "\n")
    return results


def run_latency_mode(out: Path, so_bin: str) -> dict[str, Any]:
    if not shutil.which(so_bin) and not Path(so_bin).is_file():
        payload = {
            "skipped": f"so binary not found: {so_bin}",
            "note": "Build with `make build-native` or pass --so-bin",
        }
        (out / "latency.json").write_text(json.dumps(payload, indent=2) + "\n")
        return payload
    fixture = Path("benchmarks/fixtures/tiny")
    fixture.mkdir(parents=True, exist_ok=True)
    if not (fixture / ".so").exists():
        subprocess.run([so_bin, "init", "--root", str(fixture)], check=False)
    samples: list[float] = []
    for q in ("graph query", "memory search test", "middleware"):
        start = time.perf_counter()
        subprocess.run(
            [so_bin, "memory", "search", q],
            cwd=str(fixture),
            capture_output=True,
            text=True,
        )
        samples.append((time.perf_counter() - start) * 1000.0)
    payload = {
        "memory_search_ms": {
            "p50": statistics.median(samples) if samples else None,
            "p95": sorted(samples)[int(len(samples) * 0.95)] if samples else None,
            "samples": samples,
        },
        "note": "Run on your hardware; numbers are not comparable across machines.",
    }
    (out / "latency.json").write_text(json.dumps(payload, indent=2) + "\n")
    return payload


def run_contradiction_mode(out: Path, seeds: list[int]) -> dict[str, Any]:
    cmd = [
        "go",
        "test",
        "./internal/memory/",
        "-run",
        "TestRescueAt10SemanticTarget|TestHistoricalVerbatimRequired",
        "-count=1",
    ]
    proc = subprocess.run(cmd, capture_output=True, text=True, cwd=str(Path.cwd()))
    payload = {
        "seeds": seeds,
        "ok": proc.returncode == 0,
        "rescue_at_10": "pending",
        "historical_verbatim": "pending",
        "note": "Go unit tests gate contradiction ranking.",
        "log_tail": (proc.stdout or "")[-1500:],
    }
    if proc.returncode == 0:
        payload["rescue_at_10"] = 1.0
        payload["historical_verbatim"] = 1.0
    (out / "contradiction.json").write_text(json.dumps(payload, indent=2) + "\n")
    return payload
