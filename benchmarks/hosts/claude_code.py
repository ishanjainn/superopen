"""Claude Code headless host adapter."""

from __future__ import annotations

import json
import shutil
from pathlib import Path
from typing import Any

import isolate


def available() -> bool:

    if isolate.mode() == isolate.ISOLATE_DOCKER:
        return True
    return shutil.which("claude") is not None


def _usage_from_obj(obj: Any, acc: dict[str, Any]) -> None:
    if isinstance(obj, dict):
        for key, val in obj.items():
            lower = str(key).lower()
            if lower in {"cachereadinputtokens", "cache_read_input_tokens"}:
                acc["cache_read_tokens"] += int(val or 0)
            elif lower in {"outputtokens", "output_tokens"}:
                acc["output_tokens"] += int(val or 0)
            elif lower in {"inputtokens", "input_tokens"}:
                acc["input_tokens"] += int(val or 0)
            elif lower in {"cachecreationinputtokens", "cache_creation_input_tokens"}:
                acc["cache_creation_tokens"] += int(val or 0)
            else:
                _usage_from_obj(val, acc)
    elif isinstance(obj, list):
        for item in obj:
            _usage_from_obj(item, acc)


def _command_invokes_so(command: str) -> bool:
    lower = f" {command.lower()} "
    if " so " not in lower and "/so " not in lower and not command.lower().startswith("so "):
        return False
    return " memory " in lower or " graph " in lower or "memory" in lower or "graph" in lower


def _tool_name(obj: dict[str, Any]) -> str:
    return str(obj.get("name") or obj.get("tool_name") or "").lower()


def _tool_command(obj: dict[str, Any]) -> str:
    inp = obj.get("input") or obj.get("tool_input") or {}
    if isinstance(inp, dict):
        return str(inp.get("command") or inp.get("cmd") or "")
    if isinstance(inp, str):
        return inp
    return str(obj.get("command") or "")


def _tool_path(obj: dict[str, Any]) -> str:
    inp = obj.get("input") or obj.get("tool_input") or {}
    if isinstance(inp, dict):
        return str(inp.get("file_path") or inp.get("path") or inp.get("target_file") or "")
    return ""


def empty_usage() -> dict[str, Any]:
    return {
        "input_tokens": 0,
        "output_tokens": 0,
        "cache_read_tokens": 0,
        "cache_creation_tokens": 0,
        "so_invoked": False,
        "tool_calls": 0,
        "graph_calls": 0,
        "source_reads": 0,
        "read_paths": [],
        "read_after_bodies": False,
    }


def _accumulate_tools(obj: Any, acc: dict[str, Any], *, saw_graph_body: list[bool]) -> None:
    if isinstance(obj, dict):
        typ = str(obj.get("type") or "").lower()
        name = _tool_name(obj)
        is_tool = typ in {"tool_use", "tool_call", "toolcall"} or (
            name in {"bash", "shell", "read", "glob", "grep", "edit", "write"}
            and ("input" in obj or "tool_input" in obj)
        )
        if is_tool:
            acc["tool_calls"] += 1
            cmd = _tool_command(obj)
            path = _tool_path(obj)
            if name in {"bash", "shell"} and _command_invokes_so(cmd):
                acc["so_invoked"] = True
                if " graph " in f" {cmd.lower()} ":
                    acc["graph_calls"] += 1
                    saw_graph_body[0] = True
            if name in {"read", "glob"}:
                acc["source_reads"] += 1
                if path:
                    acc["read_paths"].append(path)
                if saw_graph_body[0]:
                    acc["read_after_bodies"] = True
        if _command_invokes_so(str(obj.get("command") or "")):
            acc["so_invoked"] = True
        for val in obj.values():
            _accumulate_tools(val, acc, saw_graph_body=saw_graph_body)
    elif isinstance(obj, list):
        for item in obj:
            _accumulate_tools(item, acc, saw_graph_body=saw_graph_body)


def _so_from_obj(obj: Any) -> bool:
    acc = empty_usage()
    _accumulate_tools(obj, acc, saw_graph_body=[False])
    return bool(acc["so_invoked"])


def jsonl_sizes(claude_dir: Path) -> dict[str, int]:
    sizes: dict[str, int] = {}
    if not claude_dir.is_dir():
        return sizes
    for path in claude_dir.rglob("*.jsonl"):
        try:
            sizes[str(path)] = path.stat().st_size
        except OSError:
            continue
    return sizes


def usage_from_new_jsonl(claude_dir: Path, before: dict[str, int]) -> dict[str, Any]:
    acc = empty_usage()
    saw_graph_body = [False]
    if not claude_dir.is_dir():
        return acc
    for path in claude_dir.rglob("*.jsonl"):
        try:
            data = path.read_bytes()
        except OSError:
            continue
        prev = before.get(str(path), 0)
        if len(data) <= prev:
            continue
        chunk = data[prev:].decode("utf-8", errors="replace")
        for line in chunk.splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                row = json.loads(line)
            except json.JSONDecodeError:
                continue
            _usage_from_obj(row, acc)
            _accumulate_tools(row, acc, saw_graph_body=saw_graph_body)
    return acc


def run_prompt(
    prompt: str,
    worktree: Path,
    env: dict[str, str],
    model: str,
    timeout: int,
) -> dict[str, Any]:
    claude_dir = Path(env.get("CLAUDE_CONFIG_DIR") or "")
    before = jsonl_sizes(claude_dir)
    cmd = [
        "claude",
        "-p",
        "--model",
        model,
        "--dangerously-skip-permissions",
        "--output-format",
        "json",
        prompt,
    ]
    proc = isolate.run(cmd, cwd=worktree, env=env, timeout=timeout)
    data: dict[str, Any] = {}
    if proc.stdout:
        try:
            data = json.loads(proc.stdout)
        except json.JSONDecodeError:
            data = {"result": proc.stdout}
    usage = data.get("usage") or {}
    model_usage = data.get("modelUsage") or {}
    model_row = next(iter(model_usage.values()), {}) if model_usage else {}
    inp = int(model_row.get("inputTokens", usage.get("input_tokens", 0)) or 0)
    out = int(model_row.get("outputTokens", usage.get("output_tokens", 0)) or 0)
    cc = int(model_row.get("cacheCreationInputTokens", usage.get("cache_creation_input_tokens", 0)) or 0)
    cr = int(model_row.get("cacheReadInputTokens", usage.get("cache_read_input_tokens", 0)) or 0)
    jsonl = usage_from_new_jsonl(claude_dir, before)
    return {
        "ok": proc.returncode == 0 and not data.get("is_error"),
        "result": str(data.get("result") or ""),
        "input_tokens": max(inp, jsonl["input_tokens"]),
        "cache_creation_tokens": max(cc, jsonl["cache_creation_tokens"]),
        "cache_read_tokens": max(cr, jsonl["cache_read_tokens"]),
        "output_tokens": max(out, jsonl["output_tokens"]),
        "cost_usd": data.get("total_cost_usd"),
        "turns": data.get("num_turns"),
        "so_invoked": bool(jsonl["so_invoked"]),
        "tool_calls": int(jsonl.get("tool_calls") or 0),
        "graph_calls": int(jsonl.get("graph_calls") or 0),
        "source_reads": int(jsonl.get("source_reads") or 0),
        "read_after_bodies": bool(jsonl.get("read_after_bodies")),
        "stderr": proc.stderr,
    }


VENDOR = "claude-code"
