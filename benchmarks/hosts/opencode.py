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
        "stderr": proc.stderr,
    }


VENDOR = "opencode"
