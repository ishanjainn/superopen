"""OpenCode headless host adapter."""

from __future__ import annotations

import json
import shutil
import subprocess
from pathlib import Path
from typing import Any

import isolate


def available() -> bool:

    if isolate.mode() == isolate.ISOLATE_DOCKER:
        return True
    return shutil.which("opencode") is not None


def run_prompt(
    prompt: str,
    worktree: Path,
    env: dict[str, str],
    model: str,
    timeout: int,
) -> dict[str, Any]:
    cmd = [
        "opencode",
        "run",
        "--dir",
        str(worktree),
        "--model",
        model,
        "--format",
        "json",
        "--auto",
        prompt,
    ]
    proc = isolate.run(cmd, cwd=worktree, env=env, timeout=timeout)
    tokens_in = 0
    tokens_out = 0
    cost_usd: float | None = None
    text_parts: list[str] = []
    so_invoked = False
    tool_calls = 0
    graph_calls = 0
    source_reads = 0
    read_after_bodies = False
    saw_graph = False
    for line in (proc.stdout or "").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            evt = json.loads(line)
        except json.JSONDecodeError:
            continue
        typ = evt.get("type")
        part = evt.get("part") or {}
        if typ == "text":
            text_parts.append(str(part.get("text") or ""))
        if typ == "step_finish":
            tok = part.get("tokens") or {}
            tokens_in += int(tok.get("input") or 0)
            tokens_out += int(tok.get("output") or 0)
            if "cost" in part:
                cost_usd = float(part.get("cost") or 0)
        if typ in {"tool_use", "tool_call"} or part.get("tool") or part.get("name"):
            tool_calls += 1
            name = str(part.get("name") or part.get("tool") or evt.get("name") or "").lower()
            cmd = str(part.get("command") or (part.get("input") or {}).get("command") or "")
            if "so graph" in cmd or "so graph" in json.dumps(evt):
                graph_calls += 1
                saw_graph = True
            if name in {"read", "glob"}:
                source_reads += 1
                if saw_graph:
                    read_after_bodies = True
        blob = json.dumps(evt)
        if "so memory" in blob or "so graph" in blob or "/so memory" in blob or "/so graph" in blob:
            so_invoked = True
    return {
        "ok": proc.returncode == 0,
        "result": "\n".join(text_parts).strip(),
        "input_tokens": tokens_in,
        "output_tokens": tokens_out,
        "cost_usd": cost_usd,
        "so_invoked": so_invoked,
        "tool_calls": tool_calls,
        "graph_calls": graph_calls,
        "source_reads": source_reads,
        "read_after_bodies": read_after_bodies,
        "stderr": proc.stderr,
    }


VENDOR = "opencode"
