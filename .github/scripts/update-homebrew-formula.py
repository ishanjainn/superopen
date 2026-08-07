#!/usr/bin/env python3
"""Render and push the Homebrew formula for a new CLI release.

Replaces dawidd6/action-homebrew-bump-formula, which cannot bump this
formula: it requires a single top-level `url`/`sha256` pair, but so.rb
has four, nested under on_macos/on_linux + on_arm/on_intel blocks (one
per platform). This script downloads the published *.sha256 release
assets, rewrites version + each platform's sha256 in place, and pushes
the result directly to the tap's default branch.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request
from pathlib import Path

PLATFORMS = ("darwin-arm64", "darwin-amd64", "linux-arm64", "linux-amd64")
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")


class FormulaUpdateError(RuntimeError):
    pass


def run(*args: str, cwd: Path | None = None, redact: str | None = None) -> str:
    result = subprocess.run(args, cwd=cwd, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        stderr = result.stderr
        command = " ".join(args)
        if redact:
            stderr = stderr.replace(redact, "***")
            command = command.replace(redact, "***")
        raise FormulaUpdateError(f"command failed: {command}\n{stderr}")
    return result.stdout.strip()


def fetch_sha256(repository: str, tag: str, platform: str) -> str:
    url = f"https://github.com/{repository}/releases/download/{tag}/so-{platform}.tar.gz.sha256"
    try:
        with urllib.request.urlopen(url, timeout=30) as response:  # noqa: S310 (fixed https github.com URL)
            body = response.read().decode("utf-8").strip()
    except urllib.error.URLError as exc:
        raise FormulaUpdateError(f"could not download {url}: {exc}") from exc
    checksum = body.split()[0]
    if not SHA256_RE.match(checksum):
        raise FormulaUpdateError(f"unexpected sha256 file contents at {url}: {body!r}")
    return checksum


def update_formula(content: str, version: str, checksums: dict[str, str]) -> str:
    version_pattern = re.compile(r'(?m)^(\s*version )"[^"]+"')
    updated, count = version_pattern.subn(rf'\1"{version}"', content, count=1)
    if count != 1:
        raise FormulaUpdateError("could not find version stanza in formula")

    for platform, checksum in checksums.items():
        os_name, arch = platform.split("-", 1)
        url_fragment = f"so-{platform}.tar.gz"
        pattern = re.compile(
            rf'(url "[^"]*{re.escape(url_fragment)}"\n\s*sha256 )"[^"]*"',
        )
        new_updated, count = pattern.subn(rf'\1"{checksum}"', updated)
        if count != 1:
            raise FormulaUpdateError(
                f"expected exactly one sha256 stanza for {os_name}/{arch} ({url_fragment}), found {count}"
            )
        updated = new_updated

    return updated


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repository", required=True, help="owner/repo of the CLI source (for release assets)")
    parser.add_argument("--tag", required=True, help="release tag, e.g. cli-0.2.0")
    parser.add_argument("--version", required=True, help="bare semver, e.g. 0.2.0")
    parser.add_argument("--tap", required=True, help="owner/repo of the homebrew tap")
    parser.add_argument("--formula-path", default="Formula/so.rb")
    parser.add_argument("--token", required=True, help="token with push access to the tap")
    args = parser.parse_args()

    checksums = {platform: fetch_sha256(args.repository, args.tag, platform) for platform in PLATFORMS}

    with tempfile.TemporaryDirectory() as tmp:
        clone_dir = Path(tmp) / "tap"
        remote = f"https://x-access-token:{args.token}@github.com/{args.tap}.git"
        run("git", "clone", "--depth", "1", remote, str(clone_dir), redact=args.token)

        formula_path = clone_dir / args.formula_path
        original = formula_path.read_text(encoding="utf-8")
        updated = update_formula(original, args.version, checksums)

        if updated == original:
            print(f"::notice::formula already up to date for {args.tag}")
            return 0

        formula_path.write_text(updated, encoding="utf-8")

        run("git", "config", "user.name", "github-actions[bot]", cwd=clone_dir)
        run("git", "config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com", cwd=clone_dir)
        run("git", "add", "--", args.formula_path, cwd=clone_dir)
        run("git", "commit", "-m", f"so {args.version}", cwd=clone_dir)
        run("git", "push", "origin", "HEAD", cwd=clone_dir, redact=args.token)

    print(f"::notice::updated {args.formula_path} in {args.tap} for {args.tag}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except FormulaUpdateError as exc:
        print(f"::error::{exc}", file=sys.stderr)
        raise SystemExit(1) from exc
