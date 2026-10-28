"""Token accounting shared by SWE summaries and their per-instance report."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


def row_tokens(row: dict[str, Any]) -> int:
    """Cache and reasoning are subsets for inclusive hosts such as Codex."""
    total = int(row.get("input_tokens") or 0) + int(row.get("output_tokens") or 0)
    if row.get("token_accounting") != "inclusive":
        total += int(row.get("cache_read_tokens") or 0)
        total += int(row.get("cache_creation_tokens") or 0)
    return total


def codex_luna_standard_credits(usage: dict[str, Any]) -> float:
    """Estimate GPT-5.6 Luna standard credits; subscription billing may differ."""
    input_tokens = int(usage.get("input_tokens") or 0)
    cached = int(usage.get("cache_read_tokens") or usage.get("cached_input_tokens") or 0)
    output = int(usage.get("output_tokens") or 0)
    if cached > input_tokens:
        raise ValueError("cached input cannot exceed input tokens")
    return ((input_tokens - cached) * 5 + cached * 0.5 + output * 30) / 1_000_000


def codex_usage_from_events(path: Path) -> dict[str, Any]:
    """Read completed Codex turns; nested cache and reasoning counters are subsets."""
    keys = (
        "input_tokens", "cached_input_tokens", "cache_write_input_tokens",
        "output_tokens", "reasoning_output_tokens",
    )
    usage = {key: 0 for key in keys}
    turns = 0
    for line in path.read_text().splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get("type") != "turn.completed" or not isinstance(event.get("usage"), dict):
            continue
        turns += 1
        for key in keys:
            usage[key] += int(event["usage"].get(key) or 0)
    if not turns:
        raise ValueError(f"no completed Codex turn in {path}")
    if usage["cached_input_tokens"] > usage["input_tokens"] or usage["reasoning_output_tokens"] > usage["output_tokens"]:
        raise ValueError(f"inconsistent Codex token subsets in {path}")
    return {
        "input_tokens": usage["input_tokens"],
        "cache_read_tokens": usage["cached_input_tokens"],
        "cache_creation_tokens": usage["cache_write_input_tokens"],
        "output_tokens": usage["output_tokens"],
        "reasoning_output_tokens": usage["reasoning_output_tokens"],
        "uncached_input_tokens": usage["input_tokens"] - usage["cached_input_tokens"],
        "token_accounting": "inclusive",
        "turns": turns,
    }


def refresh_codex_swe_usage(payload: dict[str, Any], work: Path) -> None:
    """Repair row counters from retained raw events before any SWE summary."""
    if payload.get("host") != "codex":
        return
    baseline = Path(str(payload.get("baseline_swe_json") or payload.get("native_baseline") or ""))
    for row in payload.get("rows") or []:
        path = work / str(row.get("instance_id") or "") / str(row.get("arm") or "") / "artifacts" / "codex.events.jsonl"
        if not path.is_file() and baseline.is_file():
            path = baseline.parent / "swe" / str(row.get("instance_id") or "") / str(row.get("arm") or "") / "artifacts" / "codex.events.jsonl"
        if path.is_file():
            row.update(codex_usage_from_events(path))
        elif row.get("token_accounting") != "inclusive":
            raise FileNotFoundError(f"Codex usage events missing for {row.get('instance_id')} {row.get('arm')}: {path}")


def repair_codex_swe(path: Path) -> dict[str, Any]:
    """Recompute a retained Codex SWE result without rerunning its grader."""
    from swe import attach_savings, summarize_swe

    payload = json.loads(path.read_text())
    if payload.get("host") != "codex":
        raise ValueError(f"not a Codex SWE result: {path}")
    backup = path.with_name("swe.pre-token-correction.json")
    if not backup.exists():
        backup.write_text(path.read_text())
    refresh_codex_swe_usage(payload, path.parent / "swe")
    rows = payload["rows"]
    payload["summary"] = attach_savings(summarize_swe(
        [r for r in rows if r.get("arm") == "native"],
        [r for r in rows if r.get("arm") == "superopen"],
    ))
    path.write_text(json.dumps(payload, indent=2) + "\n")
    summary_path = path.parent / "summary.json"
    if summary_path.is_file():
        summary = json.loads(summary_path.read_text())
        summary["swe"] = payload
        summary_path.write_text(json.dumps(summary, indent=2) + "\n")
    return payload["summary"]


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Repair token usage in a retained Codex SWE result")
    parser.add_argument("swe_json", type=Path)
    args = parser.parse_args()
    print(json.dumps(repair_codex_swe(args.swe_json), indent=2))
