"""Shared native/superopen arm selection and baseline merge (compare + SWE)."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

PRODUCT_ARMS = {"native": False, "superopen": True}


def selected_arms(raw: str | None) -> dict[str, bool]:
    wanted = {part.strip().lower() for part in str(raw or "native,superopen").split(",") if part.strip()}
    out = {name: use_so for name, use_so in PRODUCT_ARMS.items() if name in wanted}
    if not out:
        raise RuntimeError("arms must include native and/or superopen")
    return out


def load_baseline_rows(path: str | None, skip_arms: set[str]) -> list[dict[str, Any]]:
    if not path or not str(path).strip():
        return []
    data = json.loads(Path(path).read_text())
    rows = data.get("rows") or []
    return [r for r in rows if isinstance(r, dict) and r.get("arm") in skip_arms]


def merge_arm_rows(fresh: list[dict[str, Any]], baseline: list[dict[str, Any]]) -> list[dict[str, Any]]:
    def key(row: dict[str, Any]) -> tuple[Any, Any]:
        return (row.get("arm"), row.get("instance_id") or row.get("id"))

    seen = {key(r) for r in fresh}
    out = list(fresh)
    for row in baseline:
        if key(row) not in seen:
            out.append(row)
    return out
