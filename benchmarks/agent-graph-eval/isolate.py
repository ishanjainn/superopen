"""Per-arm HOME / Claude / XDG isolation and git worktrees."""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import threading
import time
from pathlib import Path
from typing import Any

GRAFANA_URL = "https://github.com/grafana/grafana.git"
LINUX_URL = "https://github.com/torvalds/linux.git"
AUTH_FILES = (".credentials.json", "credentials.json")

KNOWN_REPOS = {
    "grafana": (GRAFANA_URL, "grafana/grafana", "grafana"),
    "grafana/grafana": (GRAFANA_URL, "grafana/grafana", "grafana"),
    "linux": (LINUX_URL, "torvalds/linux", "linux"),
    "linux/linux": (LINUX_URL, "torvalds/linux", "linux"),
    "torvalds/linux": (LINUX_URL, "torvalds/linux", "linux"),
}

ARM_LABELS = {
    "vanilla": "grep-read baseline",
    "superopen": "code graph",
}


def run(
    cmd: list[str],
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
    timeout: int = 600,
) -> subprocess.CompletedProcess[str]:
    merged = env if env is not None else os.environ.copy()
    return subprocess.run(
        cmd,
        cwd=str(cwd) if cwd else None,
        env=merged,
        text=True,
        capture_output=True,
        timeout=timeout,
    )


def run_streaming(
    cmd: list[str],
    cwd: Path | None,
    env: dict[str, str] | None,
    timeout: int,
    log: Path,
) -> subprocess.CompletedProcess[str]:
    """Run a long index command, echoing stdout/stderr live and tee-ing to log."""
    merged = env if env is not None else os.environ.copy()
    log.parent.mkdir(parents=True, exist_ok=True)
    chunks: list[str] = []
    proc = subprocess.Popen(
        cmd,
        cwd=str(cwd) if cwd else None,
        env=merged,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        bufsize=1,
    )
    assert proc.stdout is not None

    def pump() -> None:
        for line in proc.stdout:
            chunks.append(line)
            sys.stdout.write(line)
            sys.stdout.flush()

    reader = threading.Thread(target=pump, daemon=True)
    reader.start()
    try:
        proc.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        proc.kill()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            pass
        reader.join(timeout=5)
        body = "".join(chunks)
        log.write_text(body + f"\ntimeout after {timeout}s\n")
        raise subprocess.TimeoutExpired(cmd, timeout, output=body)
    reader.join(timeout=5)
    body = "".join(chunks)
    log.write_text(body)
    return subprocess.CompletedProcess(cmd, proc.returncode or 0, body, "")


def arm_paths(work: Path, arm: str) -> dict[str, Path]:
    work = work.resolve()
    root = work / "arms" / arm
    home = root / "home"
    claude = root / ".claude"
    return {
        "root": root,
        "home": home,
        "claude": claude,
        "xdg_config": home / ".config",
        "xdg_cache": home / ".cache",
        "xdg_data": home / ".local" / "share",
        "worktree": work / "worktrees" / arm / "repo",
    }


def arm_env(paths: dict[str, Path], extra: dict[str, str] | None = None) -> dict[str, str]:
    """Isolated process env. Does not read or write the developer ~/.claude.json."""
    env = os.environ.copy()
    env["HOME"] = str(paths["home"])
    env["CLAUDE_CONFIG_DIR"] = str(paths["claude"])
    env["XDG_CONFIG_HOME"] = str(paths["xdg_config"])
    env["XDG_CACHE_HOME"] = str(paths["xdg_cache"])
    env["XDG_DATA_HOME"] = str(paths["xdg_data"])
    env["SUPEROPEN_INSTALL_DIR"] = str(paths["home"] / ".superopen" / "bin")
    env.pop("SUPEROPEN_ROOT", None)
    env.pop("SUPEROPEN_HOOK_STRICT", None)
    if extra:
        env.update(extra)
    return env


def ensure_dirs(paths: dict[str, Path]) -> None:
    for key in ("home", "claude", "xdg_config", "xdg_cache", "xdg_data"):
        paths[key].mkdir(parents=True, exist_ok=True)
    (paths["home"] / ".superopen" / "bin").mkdir(parents=True, exist_ok=True)
    (paths["home"] / "Library" / "Caches").mkdir(parents=True, exist_ok=True)
    settings = paths["claude"] / "settings.json"
    if not settings.exists():
        settings.write_text("{}\n")


def copy_claude_auth(dest_claude: Path, src_claude: Path | None = None) -> list[str]:
    """Copy credential files only (never skills or hooks)."""
    src = src_claude if src_claude is not None else Path.home() / ".claude"
    dest_claude.mkdir(parents=True, exist_ok=True)
    copied: list[str] = []
    if not src.is_dir():
        return copied
    for name in AUTH_FILES:
        p = src / name
        if p.is_file():
            shutil.copy2(p, dest_claude / name)
            copied.append(name)
    return copied


def ensure_git_mirror(cache: Path, url: str, timeout: int = 1800) -> tuple[Path, str]:
    cache.parent.mkdir(parents=True, exist_ok=True)
    git_dir = cache / ".git"
    if not git_dir.exists():
        if cache.exists():
            shutil.rmtree(cache)
        print(f"cloning {url} -> {cache}", flush=True)
        proc = run(["git", "clone", "--depth", "1", url, str(cache)], timeout=timeout)
        if proc.returncode != 0:
            raise RuntimeError(f"git clone failed: {(proc.stderr or proc.stdout)[-2000:]}")
    sha_proc = run(["git", "-C", str(cache), "rev-parse", "HEAD"], timeout=60)
    if sha_proc.returncode != 0:
        raise RuntimeError(f"rev-parse failed: {sha_proc.stderr}")
    return cache, sha_proc.stdout.strip()


def ensure_grafana_mirror(cache: Path, url: str = GRAFANA_URL, timeout: int = 1800) -> tuple[Path, str]:
    return ensure_git_mirror(cache, url, timeout=timeout)


def ensure_linux_mirror(cache: Path, url: str = LINUX_URL, timeout: int = 1800) -> tuple[Path, str]:
    return ensure_git_mirror(cache, url, timeout=timeout)


def add_worktree(mirror: Path, dest: Path, timeout: int = 1800, sha: str = "") -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists():
        if sha:
            ensure_worktree_sha(dest, mirror, sha, timeout=timeout)
        return
    cmd = ["git", "-C", str(mirror), "worktree", "add", "--detach", str(dest)]
    if sha:
        cmd.append(sha)
    proc = run(cmd, timeout=timeout)
    if proc.returncode != 0 and sha:
        fetch = run(["git", "-C", str(mirror), "fetch", "--depth", "1", "origin", sha], timeout=timeout)
        if fetch.returncode != 0:
            raise RuntimeError(f"git fetch {sha} failed: {(fetch.stderr or fetch.stdout)[-2000:]}")
        proc = run(cmd, timeout=timeout)
    if proc.returncode != 0:
        raise RuntimeError(f"worktree add {dest} failed: {(proc.stderr or proc.stdout)[-2000:]}")
    if sha:
        ensure_worktree_sha(dest, mirror, sha, timeout=timeout)


def ensure_worktree_sha(dest: Path, mirror: Path, sha: str, timeout: int = 1800) -> None:
    head = run(["git", "-C", str(dest), "rev-parse", "HEAD"], timeout=60)
    current = (head.stdout or "").strip()
    if current.startswith(sha) or sha.startswith(current):
        return
    fetched = run(["git", "-C", str(mirror), "fetch", "--depth", "1", "origin", sha], timeout=timeout)
    if fetched.returncode != 0:
        raise RuntimeError(f"git fetch {sha} failed: {(fetched.stderr or fetched.stdout)[-2000:]}")
    checkout = run(["git", "-C", str(dest), "checkout", "--detach", sha], timeout=timeout)
    if checkout.returncode != 0:
        raise RuntimeError(f"checkout {sha} in {dest} failed: {(checkout.stderr or checkout.stdout)[-2000:]}")


def prepend_path(env: dict[str, str], directory: Path) -> None:
    env["PATH"] = str(directory) + os.pathsep + env.get("PATH", "")


def parse_rss_bytes(text: str) -> int | None:
    match = re.search(r"^\s*(\d+)\s+maximum resident set size", text or "", flags=re.M)
    if match:
        return int(match.group(1))
    return None


def parse_nodes_edges(text: str) -> tuple[int | None, int | None]:
    blob = text or ""
    pair = re.search(r"(\d+)\s+nodes?\b.*?(\d+)\s+edges?\b", blob, flags=re.I | re.S)
    if pair:
        return int(pair.group(1)), int(pair.group(2))
    nodes = edges = None
    parts = blob.replace(",", " ").split()
    for i, token in enumerate(parts):
        tl = token.lower().rstrip(".,:")
        if tl in {"nodes", "node"} and i > 0 and parts[i - 1].isdigit():
            nodes = int(parts[i - 1])
        if tl in {"edges", "edge"} and i > 0 and parts[i - 1].isdigit():
            edges = int(parts[i - 1])
    return nodes, edges


def sync_isolated_claude(home: Path, claude_dir: Path) -> None:
    """Copy product install output from isolated HOME into CLAUDE_CONFIG_DIR."""
    src = home / ".claude"
    if src.is_dir():
        shutil.copytree(src, claude_dir, dirs_exist_ok=True)


_REWRITE_SUFFIXES = {".json", ".md", ".txt", ".ts", ".jsonl"}


def _host_path_aliases(path: Path) -> list[str]:
    aliases: list[str] = []
    candidates = [path]
    try:
        candidates.append(path.resolve())
    except OSError:
        pass
    for p in candidates:
        s = str(p)
        if s and s not in aliases:
            aliases.append(s)
        if s.startswith("/") and not s.startswith("/private/"):
            priv = "/private" + s
            if priv not in aliases:
                aliases.append(priv)
        if s.startswith("/private/"):
            stripped = s[len("/private") :]
            if stripped and stripped not in aliases:
                aliases.append(stripped)
    return aliases


def _rewrite_text(text: str, replacements: list[tuple[str, str]]) -> str:
    out = text
    for src, dest in replacements:
        if src and src != dest:
            out = re.sub(re.escape(src) + r'(?=/|$|[\s"\'])', dest, out)
    return out


def rewrite_docker_agent_paths(
    paths: dict[str, Path],
    *,
    so_bin: Path | None = None,
) -> int:
    """Rewrite host-absolute install paths in isolated copies for Docker."""
    replacements: list[tuple[str, str]] = []

    def add(src: Path | None, dest: str) -> None:
        if src is None:
            return
        for alias in _host_path_aliases(src):
            replacements.append((alias, dest))

    add(paths.get("home"), "/eval/home")
    add(paths.get("claude"), "/eval/claude")
    add(so_bin, "/usr/local/bin/so")
    add(Path("/tmp/so"), "/usr/local/bin/so")
    replacements.sort(key=lambda pair: len(pair[0]), reverse=True)

    roots = [paths["claude"], paths["home"], paths["worktree"] / ".claude"]
    n = 0
    for root in roots:
        if not root.is_dir():
            continue
        for file in root.rglob("*"):
            if not file.is_file() or file.suffix.lower() not in _REWRITE_SUFFIXES:
                continue
            try:
                prev = file.read_text()
            except (OSError, UnicodeDecodeError):
                continue
            nxt = _rewrite_text(prev, replacements)
            if nxt != prev:
                file.write_text(nxt)
                n += 1
    return n
