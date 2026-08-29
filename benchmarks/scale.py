"""Published vs gate sizes. Small is a valid subsample, not a toy slice."""

from __future__ import annotations

import os
from typing import Any

# Locomo small n=100 (category-stratified). Full uses the 300-item file for
# retrieve; coding-agent QA still samples via --qa-n. LongMemEval-S English
# n=50 is already the Superopen split. Compare uses the whole 6-question Django bank.
SMALL = {"locomo": 100, "longmemeval": 50, "compare": 6}
FULL = {"locomo": 300, "longmemeval": 50, "compare": 6}

MIN_LOCOMO = SMALL["locomo"]
MIN_LME = SMALL["longmemeval"]
MIN_COMPARE = SMALL["compare"]

CODING_HOSTS = ("claude-code", "opencode")


def apply_scale(args: Any) -> None:
    scale = str(getattr(args, "scale", "small") or "small")
    table = FULL if scale == "full" else SMALL
    split = str(getattr(args, "split", "locomo") or "locomo")
    if getattr(args, "n", None) is None:
        args.n = table.get(split, table["locomo"])
    if not getattr(args, "qa_n", None):
        # Full retrieve uses n=300; coding-agent QA stays a 20-item sample unless set.
        if scale == "full" and split == "locomo":
            args.qa_n = 20
        else:
            args.qa_n = int(args.n)
    if getattr(args, "compare_n", None) is None:
        args.compare_n = table["compare"]
    args.scale = scale
    args.scale_sizes = dict(table)


def validate_sizes(args: Any) -> None:
    split = str(getattr(args, "split", "locomo") or "locomo")
    n = int(getattr(args, "n", 0) or 0)
    if split == "locomo" and n < MIN_LOCOMO:
        raise SystemExit(
            f"locomo n={n} is too small to publish or gate on (min {MIN_LOCOMO}, category-stratified). "
            "Pass --scale small|full, not a toy slice."
        )
    if split == "longmemeval" and n < MIN_LME:
        raise SystemExit(
            f"longmemeval n={n} is too small (min {MIN_LME}, the Superopen English split). "
            "Do not shrink the haystack to make R@10 cheaper."
        )
    compare_n = int(getattr(args, "compare_n", MIN_COMPARE) or MIN_COMPARE)
    compare_ids = str(getattr(args, "compare_ids", "") or "").strip()
    if not compare_ids and compare_n < MIN_COMPARE:
        raise SystemExit(
            f"compare n={compare_n} is too small (min {MIN_COMPARE}, the full Django bank). "
            "n=3 cached short answers are not a Superopen session."
        )


def require_coding_host(host: str) -> str:
    name = (host or "").strip()
    if name not in CODING_HOSTS:
        raise SystemExit(
            "benchmarks use a real coding-agent host only: --host claude-code or --host opencode. "
            "HTTP Messages APIs are not Superopen users."
        )
    return name


def default_model(host: str, model: str) -> str:
    model = (model or "").strip()
    if host == "claude-code" and (not model or model.startswith("opencode/")):
        return "claude-sonnet-5"
    if host == "opencode" and (not model or model.startswith("claude")):
        return "opencode/big-pickle"
    return model


def require_agent_credentials(*, modes: list[str], phase: int, max_spend: float) -> None:
    """Fail fast before Docker/agent work when compare or phase-3 QA will run."""
    need = "compare" in modes or ("memory" in modes and int(phase) >= 3)
    if not need:
        return
    key = (os.environ.get("ANTHROPIC_API_KEY") or "").strip()
    if not key:
        raise SystemExit(
            "ANTHROPIC_API_KEY is required for compare and memory phase 3 "
            "(Claude Code sessions and the QA judge). Export it or `source .env` before running."
        )
    if max_spend <= 0:
        raise SystemExit(
            "compare and memory phase 3 require --max-spend > 0 (USD cap for agent sessions)."
        )
