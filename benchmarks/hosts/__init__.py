"""Coding-agent host adapters for session compare, SWE-bench, and memory QA."""

from __future__ import annotations

from types import ModuleType


def load(name: str) -> ModuleType:
    if name == "claude-code":
        from hosts import claude_code

        return claude_code
    if name == "cursor":
        from hosts import cursor

        return cursor
    if name == "opencode":
        from hosts import opencode

        return opencode
    raise RuntimeError(f"unknown coding-agent host: {name}")
