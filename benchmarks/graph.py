"""Django graph index, 12-probe suite, and LTS growth."""

from __future__ import annotations

import json
import re
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable

import isolate
from grade import graph_probe_grade


@dataclass
class Probe:
    id: str
    category: str
    run: Callable[[Path, dict[str, str], str], tuple[bool, bool, str]]


def _so(args: list[str], root: Path, env: dict[str, str], so_bin: str, timeout: int = 120) -> subprocess.CompletedProcess[str]:
    root = root.resolve()
    return isolate.run([so_bin, *args], cwd=root, env=env, timeout=timeout)


def _parse_status(stdout: str) -> dict[str, Any]:
    try:
        payload = json.loads(stdout)
    except json.JSONDecodeError:
        return {}
    if isinstance(payload.get("data"), dict):
        return payload["data"]
    return payload if isinstance(payload, dict) else {}


def _first_qualified_name(stdout: str) -> str | None:
    for line in (stdout or "").splitlines():
        line = line.strip()
        if not line or line.startswith("total:") or line.startswith("search_mode:"):
            continue
        if line.startswith("results:") or line.startswith("truncated:") or line.startswith("has_more:"):
            continue
        if line.startswith("next_cursor:"):
            continue
        parts = line.split()
        if parts and "." in parts[0]:
            return parts[0]
    return None


def _probe_q1(root: Path, env: dict[str, str], so_bin: str) -> tuple[bool, bool, str]:
    root = root.resolve()
    proc = _so(["--json", "graph", "status"], root, env, so_bin)
    if proc.returncode != 0:
        return False, False, proc.stderr or proc.stdout
    data = _parse_status(proc.stdout or "")
    ok = int(data.get("node_count") or 0) > 0 and int(data.get("edge_count") or 0) > 0
    return ok, False, json.dumps(data)[:500]


def _probe_search(query: str, label_hint: str | None = None):
    def fn(root: Path, env: dict[str, str], so_bin: str) -> tuple[bool, bool, str]:
        proc = _so(["graph", "search", query, "--limit", "10"], root, env, so_bin)
        out = proc.stdout or ""
        ok = proc.returncode == 0 and len(out.strip()) > 0
        if ok and label_hint:
            ok = label_hint in out
        return ok, False, out[:500]

    return fn


def _probe_snippet(root: Path, env: dict[str, str], so_bin: str) -> tuple[bool, bool, str]:
    proc = _so(["graph", "search", "get_response", "--limit", "5"], root, env, so_bin)
    qn = _first_qualified_name(proc.stdout or "")
    if not qn:
        return False, False, proc.stdout or proc.stderr or ""
    proc2 = _so(["graph", "snippet", qn], root, env, so_bin)
    out = proc2.stdout or ""
    ok = proc2.returncode == 0 and ("src=" in out or re.search(r"L\d+", out))
    partial = proc2.returncode == 0 and not ok
    return ok, partial, out[:500]


def _probe_trace(direction: str):
    def fn(root: Path, env: dict[str, str], so_bin: str) -> tuple[bool, bool, str]:
        proc = _so(
            ["graph", "trace", "django.core.handlers.base.BaseHandler.get_response", "--direction", direction],
            root,
            env,
            so_bin,
        )
        ok = proc.returncode == 0 and len((proc.stdout or "").strip()) > 0
        partial = proc.returncode == 0 and not ok
        return ok, partial, (proc.stdout or "")[:500]

    return fn


def _probe_query(root: Path, env: dict[str, str], so_bin: str) -> tuple[bool, bool, str]:
    proc = _so(
        ["graph", "query", "How does Django middleware call get_response?"],
        root,
        env,
        so_bin,
    )
    ok = proc.returncode == 0 and "NODE" in (proc.stdout or "")
    return ok, False, (proc.stdout or "")[:500]


def _probe_files(root: Path, env: dict[str, str], so_bin: str) -> tuple[bool, bool, str]:
    proc = _so(["--json", "graph", "status"], root, env, so_bin)
    data = _parse_status(proc.stdout or "")
    ok = int(data.get("file_count") or 0) > 0
    return ok, False, str(data.get("file_count"))


PROBES: list[Probe] = [
    Probe("Q1", "index stats", _probe_q1),
    Probe("Q2", "find functions", _probe_search("get_response", "Method")),
    Probe("Q3", "find classes", _probe_search("MiddlewareMixin", "Class")),
    Probe("Q4", "name pattern", _probe_search("migrate", "Method")),
    Probe("Q5", "code snippet", _probe_snippet),
    Probe("Q6", "text search", _probe_search("QuerySet", "Method")),
    Probe("Q7", "outbound trace", _probe_trace("outbound")),
    Probe("Q8", "inbound trace", _probe_trace("inbound")),
    Probe("Q9", "NL query", _probe_query),
    Probe("Q10", "properties", _probe_snippet),
    Probe("Q11", "inheritance", _probe_trace("outbound")),
    Probe("Q12", "list files", _probe_files),
]


def run_graph_probes(root: Path, env: dict[str, str], so_bin: str) -> dict[str, Any]:
    rows: list[dict[str, Any]] = []
    total = 0.0
    for probe in PROBES:
        ok, partial, notes = probe.run(root, env, so_bin)
        score = graph_probe_grade(ok, partial)
        total += score
        rows.append(
            {
                "id": probe.id,
                "category": probe.category,
                "score": score,
                "ok": ok,
                "partial": partial,
                "notes": notes,
            }
        )
    denom = len(PROBES)
    return {
        "score": total,
        "max": float(denom),
        "pct": (total / denom * 100.0) if denom else 0.0,
        "probes": rows,
    }


def run_index(so_bin: str, root: Path, env: dict[str, str], timeout: int) -> dict[str, Any]:
    start = time.time()
    proc = isolate.run(
        [so_bin, "init", "--root", str(root), "--force"],
        cwd=root,
        env=env,
        timeout=timeout,
    )
    elapsed = time.time() - start
    status_proc = _so(["--json", "graph", "status"], root.resolve(), env, so_bin)
    data = _parse_status(status_proc.stdout or "") if status_proc.returncode == 0 else {}
    return {
        "ok": proc.returncode == 0,
        "elapsed_sec": elapsed,
        "nodes": data.get("node_count"),
        "edges": data.get("edge_count"),
        "files": data.get("file_count"),
        "tag": isolate.DJANGO_TAG,
        "log": (proc.stdout or "")[-2000:],
    }


def run_graph_mode(args: Any, out: Path, so_bin: str) -> dict[str, Any]:
    cache = Path("benchmarks/cache")
    mirror, sha = isolate.ensure_django_mirror(cache)
    work = out / "graph"
    paths = isolate.arm_paths(work, "superopen")
    isolate.ensure_dirs(paths, "opencode")
    env = isolate.arm_env(paths, {"SUPEROPEN_SO_BIN": so_bin})
    isolate.add_worktree(mirror, paths["worktree"])
    isolate.ensure_container(paths, so_bin)
    index = run_index(so_bin, paths["worktree"], env, args.index_timeout)
    probes = run_graph_probes(paths["worktree"], env, so_bin)
    probes["hits"] = int(probes.get("score") or 0)
    probes["total"] = int(probes.get("max") or 0)
    payload = {"corpus": "django/django", "sha": sha, "tag": isolate.DJANGO_TAG, "index": index, "graph": probes}
    (out / "graph.json").write_text(json.dumps(payload, indent=2) + "\n")
    return payload


LTS_TAGS = ("1.11.29", "2.2.28", "3.2.25", "4.2.20", "5.2.4")


def run_temporal_mode(args: Any, out: Path, so_bin: str) -> dict[str, Any]:
    cache = Path("benchmarks/cache") / "temporal"
    cache.mkdir(parents=True, exist_ok=True)
    rows: list[dict[str, Any]] = []
    for tag in LTS_TAGS:
        mirror = cache / tag.replace(".", "_")
        if not (mirror / ".git").is_dir():
            isolate.host_run(
                ["git", "clone", "--depth", "1", "--branch", tag, isolate.DJANGO_URL, str(mirror)],
                timeout=1800,
            )
        worktree = out / "temporal" / tag / "repo"
        isolate.add_worktree(mirror, worktree)
        paths = isolate.arm_paths(out / "temporal" / tag, "superopen")
        paths["worktree"] = worktree
        isolate.ensure_dirs(paths, "opencode")
        env = isolate.arm_env(paths, {"SUPEROPEN_SO_BIN": so_bin})
        isolate.ensure_container(paths, so_bin)
        start = time.time()
        isolate.run(
            [so_bin, "init", "--root", str(worktree), "--force"],
            cwd=worktree,
            env=env,
            timeout=args.index_timeout,
        )
        elapsed = time.time() - start
        ok, _, body = _probe_q1(worktree, env, so_bin)
        try:
            data = json.loads(body) if body.startswith("{") else {}
            if isinstance(data.get("data"), dict):
                data = data["data"]
        except json.JSONDecodeError:
            data = {}
        rows.append(
            {
                "tag": tag,
                "elapsed_sec": elapsed,
                "nodes": data.get("node_count"),
                "edges": data.get("edge_count"),
                "files": data.get("file_count"),
                "ok": ok,
            }
        )
    payload = {"checkpoints": rows}
    (out / "temporal.json").write_text(json.dumps(payload, indent=2) + "\n")
    return payload
