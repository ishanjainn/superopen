"""Cursor CLI headless host adapter.

Auth is CURSOR_API_KEY in the environment (Dashboard API key, `crsr_...`).
The key is never placed on the command line. Print mode emits stream-json;
token and cost fields are recorded when the CLI includes them.
"""

from __future__ import annotations

import json
import shutil
from pathlib import Path
from typing import Any

import isolate
from hosts.claude_code import _command_invokes_so, _usage_from_obj, empty_usage


def available() -> bool:
    if isolate.mode() == isolate.ISOLATE_DOCKER:
        return True
    return shutil.which("agent") is not None or shutil.which("cursor-agent") is not None


def _binary() -> str:
    return shutil.which("agent") or shutil.which("cursor-agent") or "agent"


def _cost_from_obj(obj: Any) -> float | None:
    if isinstance(obj, dict):
        for key in ("cost_usd", "total_cost_usd"):
            val = obj.get(key)
            if isinstance(val, (int, float)):
                return float(val)
        for key in ("chargedCents", "charged_cents", "costCents", "cost_cents", "totalCostCents", "total_cost_cents"):
            val = obj.get(key)
            if isinstance(val, (int, float)):
                return float(val) / 100.0
        usage = obj.get("usage")
        if isinstance(usage, dict):
            found = _cost_from_obj(usage)
            if found is not None:
                return found
            val = usage.get("cost")
            if isinstance(val, (int, float)):
                return float(val)
        for val in obj.values():
            found = _cost_from_obj(val)
            if found is not None:
                return found
    elif isinstance(obj, list):
        for item in obj:
            found = _cost_from_obj(item)
            if found is not None:
                return found
    return None


def _usage_int(usage: dict[str, Any], *keys: str) -> int:
    for key in keys:
        val = usage.get(key)
        if isinstance(val, (int, float)):
            return int(val)
    return 0


def apply_closing_usage(usage: dict[str, Any], acc: dict[str, Any]) -> None:
    """Session-close usage replaces partial sums. Cursor sends this on the result event."""
    acc["input_tokens"] = _usage_int(usage, "input_tokens", "inputTokens")
    acc["output_tokens"] = _usage_int(usage, "output_tokens", "outputTokens")
    acc["cache_read_tokens"] = _usage_int(usage, "cache_read_input_tokens", "cacheReadTokens", "cache_read_tokens")
    acc["cache_creation_tokens"] = _usage_int(
        usage, "cache_creation_input_tokens", "cacheWriteTokens", "cache_write_tokens", "cache_creation_tokens"
    )


def _note_tool(blob: dict[str, Any], acc: dict[str, Any], saw_graph: list[bool]) -> None:
    entries: list[tuple[str, dict[str, Any]]] = []
    for key, val in blob.items():
        if isinstance(val, dict):
            entries.append((key, val))
    fn = blob.get("function")
    if isinstance(fn, dict):
        entries.append((str(fn.get("name") or "function"), fn))
    for name, val in entries:
        args = val.get("args") or val.get("arguments") or {}
        if isinstance(args, str):
            try:
                args = json.loads(args)
            except json.JSONDecodeError:
                args = {"command": args}
        if not isinstance(args, dict):
            args = {}
        lower = name.lower()
        cmd = str(args.get("command") or args.get("cmd") or "")
        path = str(args.get("path") or args.get("file_path") or args.get("target_file") or "")
        if "shell" in lower or "bash" in lower or "terminal" in lower:
            acc["shell_calls"] = int(acc.get("shell_calls") or 0) + 1
            if _command_invokes_so(cmd):
                acc["so_invoked"] = True
                if " graph " in f" {cmd.lower()} ":
                    acc["graph_calls"] += 1
                    saw_graph[0] = True
            elif _command_is_grep(cmd):
                acc["grep_calls"] = int(acc.get("grep_calls") or 0) + 1
                acc["source_reads"] += 1
        elif "grep" in lower or "ripgrep" in lower:
            acc["grep_calls"] = int(acc.get("grep_calls") or 0) + 1
            acc["source_reads"] += 1
        elif "glob" in lower:
            acc["glob_calls"] = int(acc.get("glob_calls") or 0) + 1
            acc["source_reads"] += 1
        elif "read" in lower:
            acc["read_calls"] = int(acc.get("read_calls") or 0) + 1
            acc["source_reads"] += 1
            if path:
                acc["read_paths"].append(path)
            if saw_graph[0]:
                acc["read_after_bodies"] = True


def _command_is_grep(command: str) -> bool:
    text = command.strip().lower()
    if not text:
        return False
    head = text.split()[0].rstrip(";")
    if head in {"grep", "rg", "ag", "ack"}:
        return True
    return any(mark in text for mark in ("| grep ", "| rg ", "; grep ", "; rg ", "&& grep ", "&& rg "))


def metrics_from_stream(stdout: str, *, returncode: int) -> dict[str, Any]:
    acc = empty_usage()
    acc["read_calls"] = 0
    acc["grep_calls"] = 0
    acc["glob_calls"] = 0
    acc["shell_calls"] = 0
    saw_graph = [False]
    result = ""
    turns = 0
    is_error = False
    saw_result = False
    cost: float | None = None
    closing_usage: dict[str, Any] | None = None
    for line in (stdout or "").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(event, dict):
            continue
        typ = str(event.get("type") or "")
        if typ == "result":
            found = _cost_from_obj(event)
            if found is not None:
                cost = found
        usage = event.get("usage") if isinstance(event.get("usage"), dict) else None
        if typ == "result" and usage:
            closing_usage = usage
        else:
            _usage_from_obj(event, acc)
        if typ == "assistant":
            turns += 1
        elif typ == "tool_call" and event.get("subtype") in (None, "", "started"):
            acc["tool_calls"] += 1
            blob = event.get("tool_call")
            if isinstance(blob, dict):
                _note_tool(blob, acc, saw_graph)
        elif typ == "result":
            saw_result = True
            is_error = bool(event.get("is_error")) or str(event.get("subtype") or "") not in {"", "success"}
            result = str(event.get("result") or "")
    if closing_usage is not None:
        apply_closing_usage(closing_usage, acc)
    ok = returncode == 0 and saw_result and not is_error
    return {
        "ok": ok,
        "result": result,
        "input_tokens": int(acc["input_tokens"]),
        "cache_creation_tokens": int(acc["cache_creation_tokens"]),
        "cache_read_tokens": int(acc["cache_read_tokens"]),
        "output_tokens": int(acc["output_tokens"]),
        "cost_usd": cost,
        "turns": turns,
        "so_invoked": bool(acc["so_invoked"]),
        "tool_calls": int(acc["tool_calls"]),
        "graph_calls": int(acc["graph_calls"]),
        "read_calls": int(acc.get("read_calls") or 0),
        "grep_calls": int(acc.get("grep_calls") or 0),
        "glob_calls": int(acc.get("glob_calls") or 0),
        "shell_calls": int(acc.get("shell_calls") or 0),
        "source_reads": int(acc["source_reads"]),
        "read_after_bodies": bool(acc["read_after_bodies"]),
        "usage_at_close": closing_usage or {},
    }


def run_prompt(
    prompt: str,
    worktree: Path,
    env: dict[str, str],
    model: str,
    timeout: int,
) -> dict[str, Any]:
    cmd = [
        _binary(),
        "-p",
        "--force",
        "--trust",
        "--output-format",
        "stream-json",
        "--workspace",
        str(worktree),
    ]
    chosen = (model or "").strip()
    if chosen and chosen != "default":
        cmd.extend(["--model", chosen])
    cmd.append(prompt)
    proc = isolate.run(cmd, cwd=worktree, env=env, timeout=timeout)
    metrics = metrics_from_stream(proc.stdout or "", returncode=proc.returncode)
    metrics["stderr"] = proc.stderr
    metrics["transcript"] = proc.stdout or ""
    return metrics


VENDOR = "cursor"
