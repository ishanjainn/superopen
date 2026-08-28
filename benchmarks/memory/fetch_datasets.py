"""Dataset layout documentation and optional fetch helpers."""

from __future__ import annotations

import os
from pathlib import Path
from urllib.parse import urlparse
from urllib.request import Request, urlopen

LAYOUT = """
benchmarks/datasets/
  locomo/
    locomo10.json          # n=300 conversational QA (not redistributed)
  longmemeval/
    longmemeval_s.json     # n=50 English subset (not redistributed)
"""

README = """# Dataset layout

Academic datasets are **not** redistributed. Place files locally:

- `benchmarks/datasets/locomo/locomo10.json`
- `benchmarks/datasets/longmemeval/longmemeval_s.json`

The harness reads these paths when `--mode memory` runs.

CI / GitHub Actions may populate them from https URLs in
`SUPEROPEN_LOCOMO_URL` and `SUPEROPEN_LME_URL`.
"""

ENV_URLS = {
    "SUPEROPEN_LOCOMO_URL": "locomo",
    "SUPEROPEN_LME_URL": "longmemeval",
}


def dataset_path(split: str) -> Path:
    if split == "locomo":
        return Path("benchmarks/datasets/locomo/locomo10.json")
    if split == "longmemeval":
        return Path("benchmarks/datasets/longmemeval/longmemeval_s.json")
    raise ValueError(f"unknown split: {split}")


def ensure_readme(base: Path | None = None) -> Path:
    root = (base or Path("benchmarks/datasets")).resolve()
    root.mkdir(parents=True, exist_ok=True)
    readme = root / "README.md"
    if not readme.exists():
        readme.write_text(README.strip() + "\n")
    return readme


def fetch_https(url: str, dest: Path) -> None:
    parsed = urlparse(url)
    if parsed.scheme != "https" or not parsed.netloc:
        raise ValueError("only https URLs are allowed for dataset fetch")
    dest.parent.mkdir(parents=True, exist_ok=True)
    req = Request(url, method="GET")
    with urlopen(req, timeout=300) as resp:
        data = resp.read()
    if not data:
        raise RuntimeError(f"empty dataset download for {dest.name}")
    dest.write_bytes(data)


def maybe_fetch_from_env() -> list[Path]:
    """Download missing academic datasets from https env URLs (CI)."""
    written: list[Path] = []
    for env_key, split in ENV_URLS.items():
        url = (os.environ.get(env_key) or "").strip()
        if not url:
            continue
        dest = dataset_path(split)
        if dest.is_file() and dest.stat().st_size > 0:
            continue
        fetch_https(url, dest)
        written.append(dest)
    return written
