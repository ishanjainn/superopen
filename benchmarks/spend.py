"""Spend ledger and --max-spend enforcement."""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


@dataclass
class SpendLedger:
    max_spend: float
    entries: list[dict[str, Any]] = field(default_factory=list)
    total_usd: float = 0.0

    def record(self, label: str, usd: float | None, meta: dict[str, Any] | None = None) -> None:
        amount = float(usd or 0.0)
        self.entries.append({"label": label, "usd": amount, "meta": meta or {}})
        self.total_usd += amount

    def allow_llm(self) -> bool:
        return self.max_spend > 0

    def check(self) -> None:
        if self.total_usd > self.max_spend:
            raise RuntimeError(f"spend {self.total_usd:.4f} exceeds --max-spend {self.max_spend:.4f}")

    def write(self, path: Path) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            json.dumps(
                {
                    "max_spend": self.max_spend,
                    "total_usd": self.total_usd,
                    "entries": self.entries,
                },
                indent=2,
            )
            + "\n"
        )
