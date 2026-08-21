"""Copy Claude + Superopen session transcripts and count graph vs grep tools."""

from __future__ import annotations

import json
import re
import shutil
from collections import Counter
from pathlib import Path
from typing import Any

HOST_HOOK_MARKERS = (
    "/.claude/hooks/cbm-code-discovery-gate",
    "/.claude/hooks/cbm-session-reminder",
    "/.claude/hooks/cbm-subagent-reminder",
)

GRAPH_TOOLS = (
    "graph_query",
    "graph_search",
    "graph_snippet",
    "graph_trace",
    "graph_architecture",
    "graph_impact",
    "graph_schema",
    "code_search",
)

BASH_GREP_RE = re.compile(r"\b(grep|ripgrep|rg|find|fd|ack)\b", re.I)


def _walk(obj: Any, fn: Any) -> None:
    if isinstance(obj, dict):
        fn(obj)
        for v in obj.values():
            _walk(v, fn)
    elif isinstance(obj, list):
        for v in obj:
            _walk(v, fn)


def count_jsonl_tools(path: Path) -> dict[str, Any]:
    counts: Counter[str] = Counter()
    bash_grep = 0
    truncated = 0
    max_query_bytes = 0
    host_hooks = 0
    graph_cli = 0
    memory_cli = 0
    hook_binary_missing = 0
    if not path.is_file():
        return {
            "tools": dict(counts),
            "bash_grep": 0,
            "truncated": 0,
            "max_query_bytes": 0,
            "host_hooks": 0,
            "hook_binary_missing": 0,
            "jsonl": str(path),
            "missing": True,
        }
    text = path.read_text(errors="replace")
    for marker in HOST_HOOK_MARKERS:
        host_hooks += text.count(marker)
    for line in text.splitlines():
        try:
            obj = json.loads(line)
        except json.JSONDecodeError:
            continue

        def visit(o: dict[str, Any]) -> None:
            nonlocal bash_grep, truncated, max_query_bytes, graph_cli, memory_cli, hook_binary_missing
            if o.get("type") == "hook_non_blocking_error":
                code = o.get("exitCode")
                err = str(o.get("stderr") or "")
                if code == 127 or "not found" in err.lower():
                    hook_binary_missing += 1
            if o.get("type") != "tool_use":
                return
            name = str(o.get("name") or "")
            counts[name] += 1
            inp = o.get("input") if isinstance(o.get("input"), dict) else {}
            cmd = str(inp.get("command") or "")
            if name == "Bash" and BASH_GREP_RE.search(cmd):
                bash_grep += 1
            graph_cmd = (
                "so graph" in cmd
                or "graphify query" in cmd
                or "graphify path" in cmd
                or "graphify explain" in cmd
            )
            mem_cmd = "so memory" in cmd or "memory search" in cmd or "memory_search" in name or "memory_recall" in name
            if "graph_query" in name or "graph query" in cmd or graph_cmd:
                blob = json.dumps(o)
                max_query_bytes = max(max_query_bytes, len(blob))
            if graph_cmd:
                graph_cli += 1
            if mem_cmd:
                memory_cli += 1

        _walk(obj, visit)
        if "TRUNCATED" in line:
            truncated += 1
    graph = graph_cli
    for name, n in counts.items():
        if any(g in name for g in GRAPH_TOOLS) or name.endswith(tuple(GRAPH_TOOLS)):
            graph += n
    return {
        "tools": dict(counts),
        "graph_tools": graph,
        "memory_tools": memory_cli,
        "grep": counts.get("Grep", 0),
        "read": counts.get("Read", 0),
        "bash": counts.get("Bash", 0),
        "bash_grep": bash_grep,
        "truncated": truncated,
        "max_query_bytes": max_query_bytes,
        "host_hooks": host_hooks,
        "hook_binary_missing": hook_binary_missing,
        "jsonl": str(path),
        "missing": False,
    }


def find_claude_jsonl(claude_dir: Path, session_id: str) -> Path | None:
    if not session_id or not claude_dir.is_dir():
        return None
    matches = list(claude_dir.glob(f"projects/*/{session_id}.jsonl"))
    if matches:
        return matches[0]
    return None


def copy_session_transcripts(
    dest: Path,
    session_id: str,
    claude_dir: Path,
    worktree: Path,
) -> dict[str, str]:
    dest.mkdir(parents=True, exist_ok=True)
    copied: dict[str, str] = {"session_id": session_id}
    jsonl = find_claude_jsonl(claude_dir, session_id)
    if jsonl and jsonl.is_file():
        target = dest / "claude.jsonl"
        shutil.copy2(jsonl, target)
        copied["claude_jsonl"] = str(target)
        tr = jsonl.parent / session_id / "tool-results"
        if tr.is_dir():
            shutil.copytree(tr, dest / "tool-results", dirs_exist_ok=True)
            copied["tool_results"] = str(dest / "tool-results")
    so_sess = worktree / ".so" / "sessions" / session_id
    if so_sess.is_dir():
        shutil.copytree(so_sess, dest / "so-session", dirs_exist_ok=True)
        copied["so_session"] = str(dest / "so-session")
    return copied


def attach_transcript_metrics(metrics: dict[str, Any], jsonl: Path | None) -> dict[str, Any]:
    if jsonl is None:
        metrics["transcript"] = {"missing": True}
        return metrics
    metrics["transcript"] = count_jsonl_tools(jsonl)
    return metrics
