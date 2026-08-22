#!/usr/bin/env python3
"""Isolated agent-graph eval: vanilla / Superopen / Graphify / CBM / iai.

Default corpus is torvalds/linux (CBM's published kernel index). Each arm gets
its own git worktree, HOME, CLAUDE_CONFIG_DIR, and XDG dirs. MCP is passed only
for the arm that owns that server. This process never writes the developer's
~/.claude.json. Superopen never passes `--mcp-config`. CBM may still use MCP.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import sqlite3
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parent
REPO_ROOT = ROOT.parent.parent
sys.path.insert(0, str(ROOT))

from grade import filter_questions, grade_answer, grade_worktree, load_questions, parse_usage, wrap_prompt  # noqa: E402
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
    mcp_child_env,
    mcp_launch,
    rewrite_docker_agent_paths,
    parse_nodes_edges,
    parse_rss_bytes,
    prepend_path,
    run,
    run_streaming,
    write_mcp_config,
)
import docker as eval_docker  # noqa: E402
import transcripts  # noqa: E402

INDEX_TIMEOUT = 3600
AGENT_TIMEOUT = 900
CLONE_TIMEOUT = 1800
INIT_PROMPT = """Run `so init` in this repository (the same command as /so init).
If the graph already exists, `so init` prints "Already initialized" — that is success. Do not pass --force and do not rebuild.
Then run `so graph status` so node and edge counts are visible, and stop.
Do not answer architecture questions."""
USE_DOCKER = False
LINUX_SO_BIN: Path | None = None

ARM_BIN_FLAGS = {
    "superopen": ("so", "--so-bin"),
    "graphify": ("graphify", "--graphify-bin"),
    "cbm": ("cbm", "--cbm-bin"),
    "iai": ("iai", "--iai-mcp"),
}


def claude_json(
    prompt: str,
    cwd: Path,
    out_path: Path,
    mcp_config: Path | None,
    model: str,
    env: dict[str, str],
    timeout: int,
    docker_spec: dict[str, Any] | None = None,
) -> dict[str, Any]:
    mcp_arg = str(mcp_config) if mcp_config is not None else None
    if docker_spec is not None and mcp_config is not None:
        mcp_arg = "/eval/mcp.json"
    cmd = ["claude", "-p", "--model", model, "--dangerously-skip-permissions", "--output-format", "json", prompt]
    if mcp_arg is not None:
        cmd = [
            "claude",
            "--mcp-config",
            mcp_arg,
            "-p",
            "--model",
            model,
            "--dangerously-skip-permissions",
            "--output-format",
            "json",
            prompt,
        ]
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
        "mcp_config": None,
        "harness": "product",
        "label": ARM_LABELS["vanilla"],
    }


SKIP_INDEX = False


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
        "mcp_config": None,
        "docker_mcp_config": None,
        "so_in_container": True,
        "so_install_ok": install_proc.returncode == 0,
        "harness": "product",
        "label": ARM_LABELS["superopen"],
        "returncode": returncode,
        "error": err,
        "skip_index": SKIP_INDEX,
    }


def prepare_graphify(paths: dict[str, Path], env: dict[str, str], bins: dict[str, Path | None]) -> dict[str, Any]:
    binary = bins["graphify"]
    if binary is None:
        return {"ok": False, "error": "--graphify-bin is required", "label": ARM_LABELS["graphify"]}
    prepend_path(env, binary.parent)
    graph_json = paths["worktree"] / "graphify-out" / "graph.json"
    cache = os.environ.get("GRAPHIFY_OUT_CACHE", "").strip()
    if cache and not graph_json.is_file():
        src = Path(cache) / "graph.json"
        if src.is_file():
            shutil.copytree(cache, paths["worktree"] / "graphify-out", dirs_exist_ok=True)
    if graph_json.is_file():
        nodes = edges = None
        try:
            if graph_json.stat().st_size < 80_000_000:
                doc = json.loads(graph_json.read_text())
                nodes = len(doc.get("nodes") or [])
                edges = len(doc.get("edges") or [])
        except Exception:  # noqa: BLE001
            pass
        index_s = 0.0
        rss = None
        proc_ok = True
        returncode = 0
        err = None
        proc = None
    else:
        proc, index_s, err, rss = _timed_run(
            [str(binary), "extract", ".", "--code-only", "--force"],
            paths["worktree"],
            env,
            INDEX_TIMEOUT,
            paths["root"] / "graphify-extract.out",
        )
        if err or proc is None:
            return {"ok": False, "error": err or "extract_failed", "index_s": index_s, "ingest_usd": 0.0, "peak_rss_mb": _rss_mb(rss), "label": ARM_LABELS["graphify"]}
        nodes, edges = parse_nodes_edges((proc.stdout or "") + "\n" + (proc.stderr or ""))
        if graph_json.is_file() and (nodes is None or edges is None):
            try:
                doc = json.loads(graph_json.read_text())
                nodes = nodes if nodes is not None else len(doc.get("nodes") or [])
                edges = edges if edges is not None else len(doc.get("edges") or [])
            except Exception:  # noqa: BLE001
                pass
        proc_ok = proc.returncode == 0
        returncode = proc.returncode
        err = None if proc.returncode == 0 else "graphify extract failed"
    install_proc = run([str(binary), "install"], cwd=paths["worktree"], env=env, timeout=180)
    claude_install = run([str(binary), "claude", "install"], cwd=paths["worktree"], env=env, timeout=180)
    sync_isolated_claude(paths["home"], paths["claude"])
    if USE_DOCKER:
        rewrite_docker_agent_paths(paths, graphify_bin=binary)
    return {
        "ok": proc_ok,
        "nodes": nodes,
        "edges": edges,
        "index_s": index_s,
        "ingest_usd": 0.0,
        "peak_rss_mb": _rss_mb(rss),
        "mcp_config": None,
        "harness": "product",
        "label": ARM_LABELS["graphify"],
        "returncode": returncode,
        "error": err,
        "graphify_install_ok": install_proc.returncode == 0 and claude_install.returncode == 0,
        "skip_index": SKIP_INDEX,
    }


def _linux_cbm_bin() -> Path | None:
    for candidate in (
        Path("/tmp/cbm-linux/portable/codebase-memory-mcp"),
        Path("/tmp/cbm-linux/codebase-memory-mcp"),
    ):
        if candidate.is_file():
            return candidate
    return None


def prepare_cbm(paths: dict[str, Path], env: dict[str, str], bins: dict[str, Path | None]) -> dict[str, Any]:
    binary = bins["cbm"]
    if binary is None:
        return {"ok": False, "error": "--cbm-bin is required", "label": ARM_LABELS["cbm"]}
    prepend_path(env, binary.parent)
    cache = Path(env["CBM_CACHE_DIR"])
    cache.mkdir(parents=True, exist_ok=True)
    print(f"  CBM_CACHE_DIR={cache} (arm-isolated; not the host daemon cache)", flush=True)
    linux_cbm = _linux_cbm_bin()
    extra_bins = extra_volumes = docker_extra_env = None
    if USE_DOCKER:
        if linux_cbm is None:
            return {
                "ok": False,
                "error": "docker CBM requires /tmp/cbm-linux/portable/codebase-memory-mcp",
                "index_s": 0.0,
                "ingest_usd": 0.0,
                "label": ARM_LABELS["cbm"],
            }
        extra_bins = [(str(linux_cbm), "/usr/local/bin/codebase-memory-mcp")]
        extra_volumes = [(str(cache), "/eval/cbm-cache")]
        docker_extra_env = {"CBM_CACHE_DIR": "/eval/cbm-cache"}
        argv = eval_docker.run_argv(
            ["/usr/local/bin/codebase-memory-mcp", "cli", "--json", "index_repository", "--repo-path", "/work"],
            worktree=paths["worktree"],
            claude_dir=paths["claude"],
            home=paths["home"],
            extra_bins=[(linux_cbm, "/usr/local/bin/codebase-memory-mcp")],
            extra_volumes=[(cache, "/eval/cbm-cache")],
            extra_env={"CBM_CACHE_DIR": "/eval/cbm-cache"},
        )
        eval_docker.assert_isolated(argv)
        proc, index_s, err, rss = _timed_run(
            argv, Path("/"), os.environ.copy(), INDEX_TIMEOUT, paths["root"] / "cbm-index.out"
        )
    else:
        proc, index_s, err, rss = _timed_run(
            [str(binary), "cli", "--json", "index_repository", "--repo-path", str(paths["worktree"])],
            paths["worktree"],
            env,
            INDEX_TIMEOUT,
            paths["root"] / "cbm-index.out",
        )
    if err or proc is None:
        return {"ok": False, "error": err or "index_failed", "index_s": index_s, "ingest_usd": 0.0, "peak_rss_mb": _rss_mb(rss), "label": ARM_LABELS["cbm"]}
    nodes = edges = project = None
    try:
        outer = json.loads(proc.stdout or "{}")
        inner = json.loads(outer["content"][0]["text"]) if isinstance(outer, dict) and "content" in outer else outer
        nodes, edges, project = inner.get("nodes"), inner.get("edges"), inner.get("project")
    except Exception:  # noqa: BLE001
        nodes, edges = parse_nodes_edges((proc.stdout or "") + "\n" + (proc.stderr or ""))
    command, args = mcp_launch(binary)
    mcp = write_mcp_config(
        paths["mcp_config"],
        "codebase-memory",
        command,
        args,
        mcp_child_env(env, {"CBM_CACHE_DIR": env["CBM_CACHE_DIR"]}),
    )
    docker_mcp = write_mcp_config(
        paths["root"] / "mcp.docker.json",
        "codebase-memory",
        "/usr/local/bin/codebase-memory-mcp",
        [],
        {
            "HOME": "/eval/home",
            "CLAUDE_CONFIG_DIR": "/eval/claude",
            "CBM_CACHE_DIR": "/eval/cbm-cache",
        },
    )
    cbm_install = run(
        [str(binary), "install", "--yes", "--skip-binary", "--clients=claude"],
        cwd=paths["worktree"],
        env=env,
        timeout=180,
    )
    sync_isolated_claude(paths["home"], paths["claude"])
    if USE_DOCKER:
        rewrite_docker_agent_paths(paths, cbm_bin=binary)
    return {
        "ok": proc.returncode == 0,
        "nodes": nodes,
        "edges": edges,
        "project": project,
        "index_s": index_s,
        "ingest_usd": 0.0,
        "peak_rss_mb": _rss_mb(rss),
        "mcp_config": str(mcp),
        "docker_mcp_config": str(docker_mcp) if USE_DOCKER else None,
        "extra_bins": extra_bins,
        "extra_volumes": extra_volumes,
        "docker_extra_env": docker_extra_env,
        "harness": "product",
        "cbm_install_ok": cbm_install.returncode == 0,
        "label": ARM_LABELS["cbm"],
        "returncode": proc.returncode,
        "error": None if proc.returncode == 0 else "cbm index_repository failed",
    }


def iai_src_root(wrapper: Path) -> Path | None:
    cur = wrapper.resolve()
    for _ in range(8):
        if (cur / "src" / "iai_mcp").is_dir():
            return cur
        if cur.parent == cur:
            break
        cur = cur.parent
    env_root = os.environ.get("IAI_ROOT", "").strip()
    if env_root:
        candidate = Path(env_root)
        if (candidate / "src" / "iai_mcp").is_dir():
            return candidate
    return None


def prepare_iai(paths: dict[str, Path], env: dict[str, str], bins: dict[str, Path | None]) -> dict[str, Any]:
    wrapper = bins["iai"]
    if wrapper is None:
        return {"ok": False, "error": "--iai-mcp is required", "label": ARM_LABELS["iai"]}
    src_root = iai_src_root(wrapper)
    pythonpath = str((src_root / "src").resolve()) if src_root is not None else ""
    if pythonpath:
        env["PYTHONPATH"] = pythonpath + os.pathsep + env.get("PYTHONPATH", "")
    command, args = mcp_launch(wrapper)
    host_cmd, host_args = command, args
    if src_root is not None:
        host_cmd, host_args = "python3", ["-m", "iai_mcp.cli"]
    mcp = write_mcp_config(
        paths["mcp_config"],
        "iai-pme",
        host_cmd,
        host_args,
        mcp_child_env(env, {"IAI_MCP_STORE": env["IAI_MCP_STORE"], "PYTHONPATH": pythonpath}),
    )
    docker_mcp = extra_volumes = docker_extra_env = None
    if USE_DOCKER and src_root is not None:
        docker_mcp = write_mcp_config(
            paths["root"] / "mcp.docker.json",
            "iai-pme",
            "python3",
            ["-m", "iai_mcp.cli"],
            {
                "HOME": "/eval/home",
                "CLAUDE_CONFIG_DIR": "/eval/claude",
                "IAI_MCP_STORE": "/eval/home/.iai-mcp",
                "PYTHONPATH": "/eval/iai/src",
            },
        )
        extra_volumes = [(str(src_root), "/eval/iai")]
        docker_extra_env = {"IAI_MCP_STORE": "/eval/home/.iai-mcp", "PYTHONPATH": "/eval/iai/src"}
    return {
        "ok": True,
        "nodes": 0,
        "edges": 0,
        "index_s": 0.0,
        "ingest_usd": 0.0,
        "peak_rss_mb": None,
        "mcp_config": str(mcp),
        "docker_mcp_config": str(docker_mcp) if docker_mcp else None,
        "extra_volumes": extra_volumes,
        "docker_extra_env": docker_extra_env,
        "iai_src": str(src_root) if src_root else None,
        "harness": "product",
        "label": ARM_LABELS["iai"],
        "warning": None if src_root else "iai checkout src/iai_mcp not found; MCP may fail to start",
    }


PREPARE = {
    "vanilla": prepare_vanilla,
    "superopen": prepare_superopen,
    "graphify": prepare_graphify,
    "cbm": prepare_cbm,
    "iai": prepare_iai,
}


def docker_spec_for(paths: dict[str, Path], info: dict[str, Any]) -> dict[str, Any] | None:
    if not USE_DOCKER:
        return None
    so_bin = LINUX_SO_BIN if info.get("so_in_container") else None
    mcp = None
    if info.get("docker_mcp_config"):
        mcp = Path(info["docker_mcp_config"])
    elif info.get("mcp_config"):
        mcp = Path(info["mcp_config"])
    extra_bins = [(Path(src), dest) for src, dest in (info.get("extra_bins") or [])]
    extra_volumes = [(Path(src), dest) for src, dest in (info.get("extra_volumes") or [])]
    return {
        "worktree": paths["worktree"],
        "so_bin": so_bin,
        "claude_dir": paths["claude"],
        "home": paths["home"],
        "mcp_config": mcp,
        "extra_bins": extra_bins or None,
        "extra_volumes": extra_volumes or None,
        "extra_env": info.get("docker_extra_env"),
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
    """Transcript graph use — not whether the final answer mentioned the graph."""
    t = metrics.get("transcript") or {}
    if int(t.get("graph_tools") or 0) > 0:
        return True
    if int(t.get("max_query_bytes") or 0) > 0:
        return True
    return bool((metrics.get("purity") or {}).get("mentions_graph"))


def _used_memory(metrics: dict[str, Any]) -> bool:
    t = metrics.get("transcript") or {}
    if int(t.get("memory_tools") or 0) > 0:
        return True
    return bool(metrics.get("memory_used"))


def normalize_arm(name: str) -> str:
    aliases = {"so": "superopen", "peer_cli": "graphify", "peer_mcp": "cbm"}
    return aliases.get(name, name)


def aggregate_arm(questions: list[dict[str, Any]], per_q: dict[str, dict[str, Any]]) -> dict[str, Any]:
    costs, sides, turns, covs, durs = [], [], [], [], []
    errors = 0
    graph_y = 0
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
        "CBM publishes a Linux kernel full index of ~3 minutes (28M LOC, 75k files). That figure is index wall time, not agent Q&A.",
        "Superopen prepare is `so init` (same argv as `/so init` on a fresh tree), timed separately from agent USD.",
        "iai is a personal-memory engine (not a code graph). On a fresh clone it is expected to track vanilla plus empty memory tool calls.",
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
            "| Arm | Cost USD | Input-side tokens | Mean turns | Mean duration ms | Mean coverage | Graph-used questions | Errors |",
        "|-----|---------:|------------------:|-----------:|-----------------:|--------------:|---------------------:|-------:|",
    ]
    for arm in arms:
        a = aggregate_arm(questions, results.get(arm) or {})
        lines.append(
            f"| {arm} | {a['cost_usd']:.6f} | {a['input_side_total']} | {a['mean_turns']} | {a['mean_duration_ms']} | {a['mean_coverage']} | {a['graph_used']}/{a['questions']} | {a['errors']} |"
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
    """Load KEY=VALUE from path without overriding existing environment."""
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
    for arm in arms:
        spec = ARM_BIN_FLAGS.get(arm)
        if not spec:
            continue
        key, flag = spec
        path = bins.get(key)
        if path is None or not path.exists():
            return f"{arm} arm requires {flag} (got {path})"
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
    parser.add_argument("--graphify-bin", default="", help="graphify CLI (code-only extract arm)")
    parser.add_argument("--cbm-bin", default="", help="codebase-memory-mcp binary")
    parser.add_argument("--iai-mcp", default="", help="iai MCP wrapper (node .js or executable)")
    parser.add_argument("--arms", default="vanilla,superopen,graphify,cbm,iai")
    parser.add_argument("--index-timeout", type=int, default=3600)
    parser.add_argument("--agent-timeout", type=int, default=900)
    parser.add_argument("--index-only", action="store_true", help="Time graph prepare only; skip Claude questions")
    parser.add_argument("--skip-index", action="store_true", help="Reuse an existing .so graph in --work; skip harness so init")
    parser.add_argument("--docker", action="store_true", help="Run each arm's Claude process in its own container")
    parser.add_argument("--sha", default="", help="Pin the corpus worktree to this git SHA")
    parser.add_argument("--agent-init", action="store_true", default=True, help="Superopen: first Claude session runs so init (user-like), then Q&A sessions")
    parser.add_argument("--no-agent-init", action="store_false", dest="agent_init")
    args = parser.parse_args()

    INDEX_TIMEOUT = args.index_timeout
    AGENT_TIMEOUT = args.agent_timeout
    SKIP_INDEX = args.skip_index
    USE_DOCKER = args.docker

    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    work = (Path(args.work) if args.work else ROOT / "work" / stamp).resolve()
    out_dir = (Path(args.out) if args.out else ROOT / "results" / stamp).resolve()
    work.mkdir(parents=True, exist_ok=True)
    out_dir.mkdir(parents=True, exist_ok=True)

    questions = load_questions(Path(args.questions) if args.questions else default_questions(args.repo))
    questions = filter_questions(questions, args.question_ids)
    arms = [normalize_arm(a.strip()) for a in args.arms.split(",") if a.strip()]
    unknown = [a for a in arms if a not in PREPARE]
    if unknown:
        print(f"unknown arms: {unknown}", file=sys.stderr)
        return 2

    bins = {
        "so": Path(args.so_bin) if args.so_bin else None,
        "graphify": Path(args.graphify_bin) if args.graphify_bin else None,
        "cbm": Path(args.cbm_bin) if args.cbm_bin else None,
        "iai": Path(args.iai_mcp) if args.iai_mcp else None,
    }
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
        print(f"prepare {arm} ({ARM_LABELS.get(arm)})")
        paths = arm_paths(work, arm)
        ensure_dirs(paths)
        env = arm_env(paths)
        if not args.index_only:
            copy_claude_auth(paths["claude"])
        add_worktree(mirror, paths["worktree"], timeout=1800, sha=args.sha)
        info = PREPARE[arm](paths, env, bins)
        prep[arm] = info
        (paths["root"] / "prepare.json").write_text(json.dumps({k: v for k, v in info.items() if k != "error"}, indent=2) + "\n")
        print("  ", {k: info.get(k) for k in ("ok", "index_s", "nodes", "edges", "error")})
        if args.index_only:
            results[arm] = {}
            continue
        if not info.get("ok"):
            results[arm] = {q["id"]: {"error": info.get("error") or "prepare_failed"} for q in questions}
            continue

        mcp = Path(info["mcp_config"]) if info.get("mcp_config") else None
        spec = docker_spec_for(paths, info)
        if arm == "superopen" and args.agent_init and not args.index_only:
            print(f"  run {arm}/init-session")
            t0 = time.time()
            init_out = out_dir / f"{arm}.init.json"
            init_metrics = claude_json(
                INIT_PROMPT,
                paths["worktree"],
                init_out,
                mcp,
                args.model,
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
            print("   ", {k: slim_init.get(k) for k in ("turns", "cost_usd", "input_side_total", "error")})
        per_q: dict[str, dict[str, Any]] = {}
        for q in questions:
            print(f"  run {arm}/{q['id']}")
            t0 = time.time()
            out = out_dir / f"{arm}.{q['id']}.json"
            metrics = claude_json(
                wrap_prompt(q["prompt"]),
                paths["worktree"],
                out,
                mcp,
                args.model,
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
            print("   ", {k: slim.get(k) for k in ("turns", "cost_usd", "input_side_total", "error")})
        results[arm] = per_q
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
