#!/usr/bin/env python3
"""Isolated agent-graph eval: vanilla vs Superopen.

Each arm gets its own git worktree, HOME, CLAUDE_CONFIG_DIR, and XDG dirs.
This process never writes the developer's ~/.claude.json.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent
REPO_ROOT = ROOT.parent.parent
sys.path.insert(0, str(ROOT))

from grade import filter_questions, grade_answer, grade_worktree, load_questions, memory_used, parse_usage, wrap_prompt  # noqa: E402
from isolate import (  # noqa: E402
    ARM_LABELS,
    KNOWN_REPOS,
    add_worktree,
    arm_env,
    arm_paths,
    copy_claude_auth,
    ensure_dirs,
    ensure_git_mirror,
    sync_isolated_claude,
    rewrite_docker_agent_paths,
    parse_nodes_edges,
    parse_rss_bytes,
    prepend_path,
    run,
    run_streaming,
)
import docker as eval_docker  # noqa: E402
import transcripts  # noqa: E402

INDEX_TIMEOUT = 3600
AGENT_TIMEOUT = 900
CLONE_TIMEOUT = 1800
MEMORY_ARMS = frozenset({"superopen"})
INDEX_HEAVY_ARMS = frozenset({"superopen"})
SIDE_WHILE_INDEX_ARMS = frozenset({"vanilla"})
_PRINT_LOCK = threading.Lock()
INIT_PROMPT = """Run `so init` in this repository (the same command as /so init).
If the graph already exists, `so init` prints "Already initialized" — that is success. Do not pass --force and do not rebuild.
Then run `so graph status` so node and edge counts are visible, and stop.
Do not answer architecture questions."""
USE_DOCKER = False
LINUX_SO_BIN: Path | None = None
SKIP_INDEX = False


def claude_json(
    prompt: str,
    cwd: Path,
    out_path: Path,
    model: str,
    env: dict[str, str],
    timeout: int,
    docker_spec: dict[str, Any] | None = None,
) -> dict[str, Any]:
    cmd = ["claude", "-p", "--model", model, "--dangerously-skip-permissions", "--output-format", "json", prompt]
    run_env = env
    if docker_spec is not None:
        cmd = eval_docker.run_argv(cmd, **docker_spec)
        eval_docker.assert_isolated(cmd)
        run_env = os.environ.copy()
        cwd = Path("/")
    try:
        proc = run(cmd, cwd=cwd, env=run_env, timeout=timeout)
    except subprocess.TimeoutExpired as exc:
        out_path.write_text(exc.stdout or "")
        out_path.with_suffix(".err").write_text((exc.stderr or "") + f"\ntimeout after {timeout}s\n")
        return {"error": f"timeout after {timeout}s"}
    out_path.write_text(proc.stdout or "")
    out_path.with_suffix(".err").write_text(proc.stderr or "")
    if not (proc.stdout or "").strip():
        return {"error": "empty_stdout", "returncode": proc.returncode, "stderr": (proc.stderr or "")[-2000:]}
    try:
        metrics = parse_usage(out_path)
        metrics["returncode"] = proc.returncode
        return metrics
    except Exception as exc:  # noqa: BLE001
        return {"error": str(exc), "returncode": proc.returncode, "stdout_head": (proc.stdout or "")[:500]}


def _count_so_graph(repo: Path) -> tuple[int | None, int | None]:
    db = repo / ".so" / "db" / "so.db"
    if not db.exists():
        return None, None
    try:
        con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
        try:
            nodes = con.execute("SELECT COUNT(*) FROM nodes").fetchone()[0]
            edges = con.execute("SELECT COUNT(*) FROM edges").fetchone()[0]
            return int(nodes), int(edges)
        finally:
            con.close()
    except sqlite3.Error:
        return None, None


def _rss_mb(rss: int | None) -> float | None:
    if rss is None:
        return None
    return round(rss / (1024 * 1024), 1)


def _timed_run(
    cmd: list[str],
    cwd: Path,
    env: dict[str, str],
    timeout: int,
    log: Path,
) -> tuple[subprocess.CompletedProcess[str] | None, float, str | None, int | None]:
    t0 = time.time()
    timed_cmd = cmd
    if sys.platform == "darwin":
        timed_cmd = ["/usr/bin/time", "-l", *cmd]
    print(f"  $ {' '.join(cmd)}", flush=True)
    try:
        proc = run_streaming(timed_cmd, cwd=cwd, env=env, timeout=timeout, log=log)
    except subprocess.TimeoutExpired:
        return None, round(time.time() - t0, 1), f"timeout after {timeout}s", None
    rss = parse_rss_bytes((proc.stdout or "") + "\n" + (proc.stderr or ""))
    if rss is None and log.is_file():
        rss = parse_rss_bytes(log.read_text())
    return proc, round(time.time() - t0, 1), None, rss


def prepare_vanilla(_paths: dict[str, Path], _env: dict[str, str], _bins: dict[str, Path | None]) -> dict[str, Any]:
    return {
        "ok": True,
        "nodes": None,
        "edges": None,
        "index_s": 0.0,
        "ingest_usd": 0.0,
        "peak_rss_mb": None,
        "harness": "product",
        "label": ARM_LABELS["vanilla"],
    }


def prepare_superopen(paths: dict[str, Path], env: dict[str, str], bins: dict[str, Path | None]) -> dict[str, Any]:
    so_bin = bins["so"]
    if so_bin is None:
        return {"ok": False, "error": "--so-bin is required", "label": ARM_LABELS["superopen"]}
    env["SUPEROPEN_ROOT"] = str(paths["worktree"])
    env["SUPEROPEN_SO_BIN"] = str(so_bin)
    prepend_path(env, so_bin.parent)
    db = paths["worktree"] / ".so" / "db" / "so.db"
    cache = os.environ.get("SO_GRAPH_CACHE", "").strip()
    if cache and not db.is_file():
        src = Path(cache)
        if src.is_file():
            dest_dir = db.parent
            dest_dir.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src, db)
            for extra in (src.parent / "so.db-wal", src.parent / "so.db-shm"):
                if extra.is_file():
                    shutil.copy2(extra, dest_dir / extra.name)
        elif (src / "so.db").is_file():
            shutil.copytree(src, paths["worktree"] / ".so" / "db", dirs_exist_ok=True)
    if SKIP_INDEX:
        if not db.is_file():
            return {
                "ok": False,
                "error": f"--skip-index requires existing graph at {db}",
                "index_s": 0.0,
                "ingest_usd": 0.0,
                "label": ARM_LABELS["superopen"],
            }
        nodes, edges = _count_so_graph(paths["worktree"])
        index_s = 0.0
        rss = None
        proc_ok = True
        returncode = 0
        err = None
    else:
        proc, index_s, err, rss = _timed_run(
            [str(so_bin), "init"],
            paths["worktree"],
            env,
            INDEX_TIMEOUT,
            paths["root"] / "so-init.out",
        )
        if err or proc is None:
            return {"ok": False, "error": err or "init_failed", "index_s": index_s, "ingest_usd": 0.0, "peak_rss_mb": _rss_mb(rss), "label": ARM_LABELS["superopen"]}
        nodes, edges = parse_nodes_edges((proc.stdout or "") + "\n" + (proc.stderr or ""))
        if nodes is None or edges is None:
            db_nodes, db_edges = _count_so_graph(paths["worktree"])
            nodes = nodes if nodes is not None else db_nodes
            edges = edges if edges is not None else db_edges
        proc_ok = proc.returncode == 0
        returncode = proc.returncode
        err = None if proc.returncode == 0 else "so init failed"
    install_proc = run([str(so_bin), "install", "--vendor=claude-code"], cwd=paths["worktree"], env=env, timeout=180)
    sync_isolated_claude(paths["home"], paths["claude"])
    if USE_DOCKER:
        rewrite_docker_agent_paths(paths, so_bin=so_bin)
    return {
        "ok": proc_ok,
        "nodes": nodes,
        "edges": edges,
        "index_s": index_s,
        "ingest_usd": 0.0,
        "peak_rss_mb": _rss_mb(rss),
        "so_in_container": True,
        "so_install_ok": install_proc.returncode == 0,
        "harness": "product",
        "label": ARM_LABELS["superopen"],
        "returncode": returncode,
        "error": err,
        "skip_index": SKIP_INDEX,
    }


PREPARE = {
    "vanilla": prepare_vanilla,
    "superopen": prepare_superopen,
}


def docker_spec_for(paths: dict[str, Path], info: dict[str, Any]) -> dict[str, Any] | None:
    if not USE_DOCKER:
        return None
    so_bin = LINUX_SO_BIN if info.get("so_in_container") else None
    return {
        "worktree": paths["worktree"],
        "so_bin": so_bin,
        "claude_dir": paths["claude"],
        "home": paths["home"],
    }


def record_session(metrics: dict[str, Any], paths: dict[str, Path], out_dir: Path, arm: str, tag: str) -> dict[str, Any]:
    session_id = str(metrics.get("session_id") or "")
    dest = out_dir / "transcripts" / arm / tag
    copied = transcripts.copy_session_transcripts(dest, session_id, paths["claude"], paths["worktree"])
    jsonl = Path(copied["claude_jsonl"]) if copied.get("claude_jsonl") else transcripts.find_claude_jsonl(paths["claude"], session_id)
    transcripts.attach_transcript_metrics(metrics, jsonl)
    metrics["transcript_dir"] = str(dest)
    if (metrics.get("transcript") or {}).get("host_hooks"):
        metrics["error"] = "host_global_hook_leak"
    if (metrics.get("transcript") or {}).get("hook_binary_missing"):
        metrics["error"] = "hook_binary_missing"
    return metrics


def _used_graph(metrics: dict[str, Any]) -> bool:
    t = metrics.get("transcript") or {}
    if int(t.get("graph_tools") or 0) > 0:
        return True
    if int(t.get("max_query_bytes") or 0) > 0:
        return True
    return bool((metrics.get("purity") or {}).get("mentions_graph"))


def _used_memory(metrics: dict[str, Any]) -> bool:
    t = metrics.get("transcript") or {}
    if memory_used(str(metrics.get("result") or ""), t):
        return True
    return bool(metrics.get("memory_used"))


def normalize_arm(name: str) -> str:
    return {"so": "superopen"}.get(name, name)


def flush_memory_after_question(
    arm: str,
    paths: dict[str, Path],
    env: dict[str, str],
    bins: dict[str, Path | None],
    metrics: dict[str, Any],
) -> None:
    if arm not in MEMORY_ARMS:
        return
    session_id = str(metrics.get("session_id") or "").strip()
    if arm == "superopen" and session_id:
        so_bin = bins.get("so")
        if so_bin is not None and so_bin.exists():
            proc = run(
                [str(so_bin), "sessions", "finalize", session_id],
                cwd=paths["worktree"],
                env=env,
                timeout=300,
            )
            if proc.returncode != 0:
                print(f"   warn: so sessions finalize {session_id} rc={proc.returncode}", flush=True)


def default_index_timeout(repo_arg: str, questions_path: Path | None) -> int:
    if questions_path is not None and questions_path.name == "grafana-memory.json":
        return 10800
    spec = KNOWN_REPOS.get(repo_arg)
    if spec is not None and spec[2] == "grafana":
        return 7200
    return 3600


def _log(msg: str) -> None:
    with _PRINT_LOCK:
        print(msg, flush=True)


def should_parallel_index(arms: list[str]) -> bool:
    return "superopen" in arms and any(a in SIDE_WHILE_INDEX_ARMS for a in arms)


def load_completed_prep(work: Path, arm: str) -> dict[str, Any] | None:
    path = work / "arms" / arm / "prepare.json"
    if not path.is_file():
        return None
    try:
        return json.loads(path.read_text())
    except json.JSONDecodeError:
        return None


def load_completed_arm(out_dir: Path, arm: str, questions: list[dict[str, Any]]) -> dict[str, dict[str, Any]] | None:
    totals = out_dir / f"{arm}.totals.json"
    if not totals.is_file():
        return None
    per_q: dict[str, dict[str, Any]] = {}
    for q in questions:
        metrics_path = out_dir / f"{arm}.{q['id']}.metrics.json"
        if not metrics_path.is_file():
            return None
        per_q[q["id"]] = json.loads(metrics_path.read_text())
    return per_q


def arm_setup(
    arm: str,
    work: Path,
    mirror: Path,
    sha: str,
    *,
    index_only: bool,
) -> tuple[dict[str, Path], dict[str, str]]:
    paths = arm_paths(work, arm)
    ensure_dirs(paths)
    env = arm_env(paths)
    if not index_only:
        copy_claude_auth(paths["claude"])
    add_worktree(mirror, paths["worktree"], timeout=1800, sha=sha)
    return paths, env


def arm_prepare(arm: str, paths: dict[str, Path], env: dict[str, str], bins: dict[str, Path | None]) -> dict[str, Any]:
    _log(f"prepare {arm} ({ARM_LABELS.get(arm)})")
    info = PREPARE[arm](paths, env, bins)
    slim = {k: v for k, v in info.items() if k != "error"}
    (paths["root"] / "prepare.json").write_text(json.dumps(slim, indent=2) + "\n")
    _log(f"  {arm} { {k: info.get(k) for k in ('ok', 'index_s', 'nodes', 'edges', 'error')} }")
    return info


def arm_run_questions(
    arm: str,
    paths: dict[str, Path],
    env: dict[str, str],
    info: dict[str, Any],
    questions: list[dict[str, Any]],
    *,
    model: str,
    out_dir: Path,
    bins: dict[str, Path | None],
    agent_init: bool,
    prep: dict[str, dict[str, Any]],
) -> dict[str, dict[str, Any]]:
    if not info.get("ok"):
        return {q["id"]: {"error": info.get("error") or "prepare_failed"} for q in questions}

    spec = docker_spec_for(paths, info)
    if arm == "superopen" and agent_init:
        _log(f"  run {arm}/init-session")
        t0 = time.time()
        init_out = out_dir / f"{arm}.init.json"
        init_metrics = claude_json(
            INIT_PROMPT,
            paths["worktree"],
            init_out,
            model,
            env,
            AGENT_TIMEOUT,
            docker_spec=spec,
        )
        init_metrics["wall_s"] = round(time.time() - t0, 1)
        record_session(init_metrics, paths, out_dir, arm, "init")
        slim_init = {k: v for k, v in init_metrics.items() if k != "result"}
        slim_init["result_excerpt"] = (init_metrics.get("result") or init_metrics.get("error") or "")[:1200]
        (out_dir / f"{arm}.init.metrics.json").write_text(json.dumps(slim_init, indent=2) + "\n")
        prep[arm]["agent_init_usd"] = init_metrics.get("cost_usd")
        prep[arm]["agent_init_wall_s"] = init_metrics.get("wall_s")
        _log(f"   {arm}/init { {k: slim_init.get(k) for k in ('turns', 'cost_usd', 'input_side_total', 'error')} }")

    per_q: dict[str, dict[str, Any]] = {}
    for q in questions:
        _log(f"  run {arm}/{q['id']}")
        t0 = time.time()
        out = out_dir / f"{arm}.{q['id']}.json"
        metrics = claude_json(
            wrap_prompt(q["prompt"]),
            paths["worktree"],
            out,
            model,
            env,
            AGENT_TIMEOUT,
            docker_spec=spec,
        )
        metrics["wall_s"] = round(time.time() - t0, 1)
        record_session(metrics, paths, out_dir, arm, q["id"])
        if not metrics.get("error"):
            metrics["coverage"] = grade_answer(metrics.get("result") or "", q.get("key_facts") or [])
            if q.get("expect_path_contains") or q.get("expect_diff_files"):
                metrics["task"] = grade_worktree(paths["worktree"], q)
            metrics["memory_used"] = _used_memory(metrics)
            if q.get("expect_memory"):
                metrics["memory_expected"] = True
        per_q[q["id"]] = metrics
        slim = {k: v for k, v in metrics.items() if k != "result"}
        slim["result_excerpt"] = (metrics.get("result") or metrics.get("error") or "")[:1200]
        (out_dir / f"{arm}.{q['id']}.metrics.json").write_text(json.dumps(slim, indent=2) + "\n")
        _log(f"   {arm}/{q['id']} { {k: slim.get(k) for k in ('turns', 'cost_usd', 'input_side_total', 'error')} }")
        if not metrics.get("error"):
            flush_memory_after_question(arm, paths, env, bins, metrics)
    return per_q


def run_arm_full(
    arm: str,
    paths: dict[str, Path],
    env: dict[str, str],
    questions: list[dict[str, Any]],
    *,
    model: str,
    out_dir: Path,
    bins: dict[str, Path | None],
    agent_init: bool,
    prep: dict[str, dict[str, Any]],
    index_only: bool,
) -> tuple[dict[str, Any], dict[str, dict[str, Any]]]:
    info = arm_prepare(arm, paths, env, bins)
    if index_only:
        return info, {}
    per_q = arm_run_questions(
        arm,
        paths,
        env,
        info,
        questions,
        model=model,
        out_dir=out_dir,
        bins=bins,
        agent_init=agent_init,
        prep=prep,
    )
    return info, per_q


def run_arms_parallel_index(
    arms_to_run: list[str],
    contexts: dict[str, tuple[dict[str, Path], dict[str, str]]],
    questions: list[dict[str, Any]],
    *,
    model: str,
    out_dir: Path,
    bins: dict[str, Path | None],
    agent_init: bool,
    prep: dict[str, dict[str, Any]],
    results: dict[str, dict[str, dict[str, Any]]],
    index_only: bool,
) -> None:
    index_arm = next((a for a in arms_to_run if a in INDEX_HEAVY_ARMS), None)
    if index_arm is None:
        raise RuntimeError("parallel index mode requires an index-heavy arm")
    side_arms = [a for a in arms_to_run if a != index_arm]
    _log(f"parallel: {index_arm} index + side arms {side_arms}")

    def run_side(arm: str) -> tuple[str, dict[str, Any], dict[str, dict[str, Any]]]:
        paths, env = contexts[arm]
        info, per_q = run_arm_full(
            arm,
            paths,
            env,
            questions,
            model=model,
            out_dir=out_dir,
            bins=bins,
            agent_init=agent_init,
            prep=prep,
            index_only=index_only,
        )
        return arm, info, per_q

    max_workers = max(len(side_arms) + 1, 2)
    with ThreadPoolExecutor(max_workers=max_workers) as ex:
        index_paths, index_env = contexts[index_arm]
        index_future = ex.submit(arm_prepare, index_arm, index_paths, index_env, bins)
        side_futures = {ex.submit(run_side, arm): arm for arm in side_arms}
        prep[index_arm] = index_future.result()
        if not index_only:
            if prep[index_arm].get("ok"):
                results[index_arm] = arm_run_questions(
                    index_arm,
                    index_paths,
                    index_env,
                    prep[index_arm],
                    questions,
                    model=model,
                    out_dir=out_dir,
                    bins=bins,
                    agent_init=agent_init,
                    prep=prep,
                )
            else:
                results[index_arm] = {
                    q["id"]: {"error": prep[index_arm].get("error") or "prepare_failed"} for q in questions
                }
        for fut in as_completed(side_futures):
            arm, info, per_q = fut.result()
            prep[arm] = info
            results[arm] = per_q


def aggregate_arm(questions: list[dict[str, Any]], per_q: dict[str, dict[str, Any]]) -> dict[str, Any]:
    costs, sides, turns, covs, durs = [], [], [], [], []
    errors = 0
    graph_y = 0
    mem_y = 0
    for q in questions:
        r = per_q.get(q["id"]) or {}
        if r.get("error"):
            errors += 1
            continue
        costs.append(float(r.get("cost_usd") or 0))
        sides.append(int(r.get("input_side_total") or 0))
        if r.get("turns") is not None:
            turns.append(int(r["turns"]))
        if r.get("duration_ms") is not None:
            durs.append(int(r["duration_ms"]))
        covs.append(float((r.get("coverage") or {}).get("coverage") or 0))
        if _used_graph(r):
            graph_y += 1
        if _used_memory(r):
            mem_y += 1
    n = max(len(questions) - errors, 0)
    return {
        "questions": len(questions),
        "errors": errors,
        "cost_usd": round(sum(costs), 6),
        "input_side_total": sum(sides),
        "mean_turns": round(sum(turns) / len(turns), 2) if turns else None,
        "mean_duration_ms": round(sum(durs) / len(durs), 1) if durs else None,
        "mean_coverage": round(sum(covs) / len(covs), 4) if covs else None,
        "graph_used": graph_y,
        "memory_used": mem_y,
        "n_ok": n,
    }


def write_summary(
    out_dir: Path,
    corpus: str,
    sha: str,
    questions: list[dict[str, Any]],
    prep: dict[str, dict[str, Any]],
    results: dict[str, dict[str, dict[str, Any]]],
    arms: list[str],
    model: str,
) -> None:
    lines = [
        "# Agent graph eval summary",
        "",
        f"Generated: {datetime.now(timezone.utc).isoformat()}",
        f"Corpus: {corpus} `{sha}`",
        f"Model: {model}",
        "Harness: product (each arm's real install command)",
        "",
        "Superopen prepare is `so init` (same argv as `/so init` on a fresh tree), timed separately from agent USD.",
        "",
        "## Index",
        "",
        "| Arm | Label | OK | Seconds | Peak RSS MB | Nodes | Edges | Ingest USD |",
        "|-----|-------|:--:|--------:|------------:|------:|------:|-----------:|",
    ]
    for arm in arms:
        g = prep.get(arm) or {}
        lines.append(
            f"| {arm} | {g.get('label')} | {g.get('ok')} | {g.get('index_s')} | {g.get('peak_rss_mb')} | {g.get('nodes')} | {g.get('edges')} | {g.get('ingest_usd')} |"
        )
    lines += [
        "",
        "## Agent totals (sum over questions)",
        "",
        "| Arm | Cost USD | Input-side tokens | Mean turns | Mean duration ms | Mean coverage | Graph-used questions | Memory-used questions | Errors |",
        "|-----|---------:|------------------:|-----------:|-----------------:|--------------:|---------------------:|----------------------:|-------:|",
    ]
    for arm in arms:
        a = aggregate_arm(questions, results.get(arm) or {})
        lines.append(
            f"| {arm} | {a['cost_usd']:.6f} | {a['input_side_total']} | {a['mean_turns']} | {a['mean_duration_ms']} | {a['mean_coverage']} | {a['graph_used']}/{a['questions']} | {a['memory_used']}/{a['questions']} | {a['errors']} |"
        )
    lines += ["", "## Per question", ""]
    for q in questions:
        lines += [
            f"### {q['id']}",
            "",
            q["prompt"],
            "",
            "| Arm | Turns | Cost USD | Input-side | Duration ms | Coverage | Verdict | Graph? | Task? | Memory? |",
            "|-----|------:|---------:|-----------:|------------:|---------:|---------|:------:|:-----:|:-------:|",
        ]
        for arm in arms:
            r = (results.get(arm) or {}).get(q["id"]) or {}
            if r.get("error"):
                lines.append(f"| {arm} | ERR | | | | | | |")
                continue
            cov = (r.get("coverage") or {}).get("coverage")
            verdict = (r.get("coverage") or {}).get("verdict") or ""
            cov_s = f"{cov:.2f}" if cov is not None else ""
            g = "Y" if _used_graph(r) else "N"
            task_ok = (r.get("task") or {}).get("ok")
            task_s = "Y" if task_ok else ("N" if task_ok is False else "")
            mem_s = "Y" if _used_memory(r) else "N"
            lines.append(
                f"| {arm} | {r.get('turns')} | {float(r.get('cost_usd') or 0):.6f} | {r.get('input_side_total')} | {r.get('duration_ms')} | {cov_s} | {verdict} | {g} | {task_s} | {mem_s} |"
            )
        lines.append("")
    lines += ["", "## Shared instruction (identical for all arms)", "", "```", wrap_prompt("<question>").strip(), "```", ""]
    out_dir.joinpath("summary.md").write_text("\n".join(lines) + "\n")


def resolve_mirror(repo_arg: str, cache_root: Path) -> tuple[Path, str, str]:
    spec = KNOWN_REPOS.get(repo_arg)
    if spec is not None:
        url, label, slug = spec
        cache = cache_root / slug
        mirror, sha = ensure_git_mirror(cache, url, timeout=CLONE_TIMEOUT)
        return mirror, sha, label
    path = Path(repo_arg).expanduser().resolve()
    if not path.is_dir():
        raise SystemExit(f"missing --repo {path}")
    sha = run(["git", "-C", str(path), "rev-parse", "HEAD"], timeout=60)
    if sha.returncode != 0:
        raise SystemExit(f"--repo is not a git checkout: {path}")
    return path, sha.stdout.strip(), str(path)


def default_questions(repo_arg: str) -> Path:
    spec = KNOWN_REPOS.get(repo_arg)
    slug = spec[2] if spec else "linux"
    named = ROOT / "questions" / f"{slug}.json"
    if named.is_file():
        return named
    return ROOT / "questions" / "linux.json"


def load_dotenv(path: Path) -> None:
    if not path.is_file():
        return
    for line in path.read_text().splitlines():
        s = line.strip()
        if not s or s.startswith("#") or "=" not in s:
            continue
        key, _, val = s.partition("=")
        key = key.strip()
        val = val.strip().strip("'").strip('"')
        if key and key not in os.environ:
            os.environ[key] = val


def require_bins(arms: list[str], bins: dict[str, Path | None]) -> str | None:
    if "superopen" in arms:
        path = bins.get("so")
        if path is None or not path.exists():
            return f"superopen arm requires --so-bin (got {path})"
    return None


def main() -> int:
    global INDEX_TIMEOUT, AGENT_TIMEOUT, SKIP_INDEX, USE_DOCKER, LINUX_SO_BIN
    load_dotenv(REPO_ROOT / ".env")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", default="linux", help="linux (clone torvalds/linux), grafana, or a local git checkout")
    parser.add_argument("--questions", default="")
    parser.add_argument("--question-ids", default="", help="Comma-separated question ids to keep")
    parser.add_argument("--work", default="")
    parser.add_argument("--out", default="")
    parser.add_argument("--model", default="haiku")
    parser.add_argument("--so-bin", default="/tmp/so")
    parser.add_argument("--arms", default="vanilla,superopen")
    parser.add_argument("--skip-arms", default="", help="Comma-separated arms to skip (reuse results from --out)")
    parser.add_argument("--parallel-index", action="store_true", help="Run vanilla while superopen indexes")
    parser.add_argument("--no-parallel-index", action="store_true", help="Force sequential arm execution")
    parser.add_argument("--index-timeout", type=int, default=0, help="Override index timeout (default: repo/question aware)")
    parser.add_argument("--agent-timeout", type=int, default=900)
    parser.add_argument("--index-only", action="store_true", help="Time graph prepare only; skip Claude questions")
    parser.add_argument("--skip-index", action="store_true", help="Reuse an existing .so graph in --work; skip harness so init")
    parser.add_argument("--docker", action="store_true", help="Run each arm's Claude process in its own container")
    parser.add_argument("--sha", default="", help="Pin the corpus worktree to this git SHA")
    parser.add_argument("--agent-init", action="store_true", default=True, help="Superopen: first Claude session runs so init (user-like), then Q&A sessions")
    parser.add_argument("--no-agent-init", action="store_false", dest="agent_init")
    args = parser.parse_args()

    AGENT_TIMEOUT = args.agent_timeout
    SKIP_INDEX = args.skip_index
    USE_DOCKER = args.docker

    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    work = (Path(args.work) if args.work else ROOT / "work" / stamp).resolve()
    out_dir = (Path(args.out) if args.out else ROOT / "results" / stamp).resolve()
    work.mkdir(parents=True, exist_ok=True)
    out_dir.mkdir(parents=True, exist_ok=True)

    questions_path = Path(args.questions) if args.questions else default_questions(args.repo)
    questions = load_questions(questions_path)
    questions = filter_questions(questions, args.question_ids)
    arms = [normalize_arm(a.strip()) for a in args.arms.split(",") if a.strip()]
    skip_arms = {normalize_arm(a.strip()) for a in args.skip_arms.split(",") if a.strip()}
    global INDEX_TIMEOUT
    INDEX_TIMEOUT = args.index_timeout or default_index_timeout(args.repo, questions_path)
    print(f"index timeout {INDEX_TIMEOUT}s", flush=True)
    unknown = [a for a in arms if a not in PREPARE]
    if unknown:
        print(f"unknown arms: {unknown}", file=sys.stderr)
        return 2

    bins = {"so": Path(args.so_bin) if args.so_bin else None}
    missing = require_bins(arms, bins)
    if missing:
        print(missing, file=sys.stderr)
        return 2
    if USE_DOCKER:
        if not eval_docker.docker_available():
            print("docker not on PATH", file=sys.stderr)
            return 2
        print("docker build", eval_docker.IMAGE)
        proc = run(eval_docker.build_image_cmd(), cwd=ROOT, timeout=1200)
        if proc.returncode != 0:
            print(proc.stderr or proc.stdout, file=sys.stderr)
            return 2
        linux_so = work / "so-linux"
        print(f"linux so -> {linux_so}")
        eval_docker.build_linux_so(REPO_ROOT, linux_so)
        LINUX_SO_BIN = linux_so
    elif not args.index_only and shutil.which("claude") is None:
        print("claude CLI not on PATH", file=sys.stderr)
        return 2

    print(f"mirror {args.repo}")
    mirror, sha, corpus = resolve_mirror(args.repo, ROOT / "cache")
    if args.sha:
        sha = args.sha
    print(f"  corpus {corpus}")
    print(f"  sha {sha}")
    (out_dir / "corpus.json").write_text(
        json.dumps({"repo": args.repo, "corpus": corpus, "sha": sha, "mirror": str(mirror)}, indent=2) + "\n"
    )

    prep: dict[str, dict[str, Any]] = {}
    results: dict[str, dict[str, dict[str, Any]]] = {}

    for arm in arms:
        if arm not in skip_arms:
            continue
        loaded = load_completed_arm(out_dir, arm, questions)
        if loaded is None:
            print(f"--skip-arms {arm}: no complete results in {out_dir}", file=sys.stderr)
            return 2
        results[arm] = loaded
        prep[arm] = load_completed_prep(work, arm) or {}
        print(f"skip {arm} (loaded {len(loaded)} questions from {out_dir})", flush=True)

    arms_to_run = [a for a in arms if a not in skip_arms]
    parallel = (args.parallel_index or should_parallel_index(arms_to_run)) and not args.no_parallel_index
    if parallel and not should_parallel_index(arms_to_run):
        parallel = False
    if parallel:
        print("parallel index mode enabled", flush=True)

    contexts: dict[str, tuple[dict[str, Path], dict[str, str]]] = {}
    for arm in arms_to_run:
        contexts[arm] = arm_setup(arm, work, mirror, sha, index_only=args.index_only)

    if parallel and should_parallel_index(arms_to_run):
        run_arms_parallel_index(
            arms_to_run,
            contexts,
            questions,
            model=args.model,
            out_dir=out_dir,
            bins=bins,
            agent_init=args.agent_init,
            prep=prep,
            results=results,
            index_only=args.index_only,
        )
    else:
        for arm in arms_to_run:
            paths, env = contexts[arm]
            info, per_q = run_arm_full(
                arm,
                paths,
                env,
                questions,
                model=args.model,
                out_dir=out_dir,
                bins=bins,
                agent_init=args.agent_init,
                prep=prep,
                index_only=args.index_only,
            )
            prep[arm] = info
            if args.index_only:
                results[arm] = {}
            else:
                results[arm] = per_q

    for arm in arms:
        if arm in skip_arms or args.index_only:
            continue
        per_q = results.get(arm) or {}
        agg = aggregate_arm(questions, per_q)
        (out_dir / f"{arm}.totals.json").write_text(json.dumps(agg, indent=2) + "\n")

    graphs = {
        arm: {k: (prep.get(arm) or {}).get(k) for k in ("ok", "nodes", "edges", "index_s", "ingest_usd", "label")}
        for arm in arms
    }
    write_summary(out_dir, corpus, sha, questions, prep, results, arms, args.model)
    (out_dir / "prepare.json").write_text(json.dumps(prep, indent=2) + "\n")
    (out_dir / "graphs.json").write_text(json.dumps(graphs, indent=2) + "\n")
    print(f"wrote {out_dir / 'summary.md'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
