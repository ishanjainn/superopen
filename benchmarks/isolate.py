"""Per-arm isolation for product benchmarks (Claude Code / OpenCode)."""

from __future__ import annotations

import atexit
import json
import os
import shutil
import subprocess
from pathlib import Path
from typing import Any

DJANGO_URL = "https://github.com/django/django.git"
DJANGO_TAG = "5.2.4"
AUTH_FILES = (".credentials.json", "credentials.json", "auth.json")

ISOLATE_HOST = "host"
ISOLATE_DOCKER = "docker"

_mode = ISOLATE_HOST
_so_linux: str | None = None


def set_mode(mode: str) -> None:
    global _mode
    if mode not in {ISOLATE_HOST, ISOLATE_DOCKER}:
        raise ValueError(mode)
    _mode = mode


def mode() -> str:
    return _mode


def set_linux_so(path: str) -> None:
    global _so_linux
    _so_linux = path


def guest_so() -> str | None:
    return _so_linux


def ensure_container(paths: dict[str, Path], so_bin: str | None = None) -> None:
    if _mode != ISOLATE_DOCKER:
        return
    import docker as bench_docker

    so = Path(so_bin or _so_linux or "")
    if not so.is_file():
        raise RuntimeError("docker isolate needs a local Superopen CLI to mount")
    bench_docker.start_arm(
        worktree=paths["worktree"],
        home=paths["home"],
        claude=paths["claude"],
        so_linux=so,
    )


def host_run(
    cmd: list[str],
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
    timeout: int = 600,
) -> subprocess.CompletedProcess[str]:
    merged = env if env is not None else os.environ.copy()
    try:
        return subprocess.run(
            cmd,
            cwd=str(cwd) if cwd else None,
            env=merged,
            text=True,
            capture_output=True,
            timeout=timeout,
        )
    except subprocess.TimeoutExpired as exc:
        return subprocess.CompletedProcess(
            cmd,
            124,
            stdout=exc.stdout or "",
            stderr=(exc.stderr or "") + "\ntimeout",
        )


def run(
    cmd: list[str],
    cwd: Path | None = None,
    env: dict[str, str] | None = None,
    timeout: int = 600,
) -> subprocess.CompletedProcess[str]:
    merged = env if env is not None else os.environ.copy()
    if _mode == ISOLATE_DOCKER and cwd is not None and merged.get("HOME") and merged.get("CLAUDE_CONFIG_DIR"):
        import docker as bench_docker

        so_host = _so_linux or merged.get("SUPEROPEN_SO_BIN")
        return bench_docker.exec_cmd(
            cmd,
            worktree=Path(cwd),
            home=Path(merged["HOME"]),
            claude=Path(merged["CLAUDE_CONFIG_DIR"]),
            so_host=so_host,
            env=merged,
            timeout=timeout,
        )
    return host_run(cmd, cwd=cwd, env=merged, timeout=timeout)


def ensure_django_mirror(cache: Path) -> tuple[Path, str]:
    mirror = cache / "django"
    if not (mirror / ".git").is_dir():
        mirror.parent.mkdir(parents=True, exist_ok=True)
        proc = host_run(["git", "clone", "--depth", "1", "--branch", DJANGO_TAG, DJANGO_URL, str(mirror)], timeout=1800)
        if proc.returncode != 0:
            raise RuntimeError(proc.stderr or proc.stdout or "django clone failed")
    sha = host_run(["git", "-C", str(mirror), "rev-parse", "HEAD"]).stdout.strip()
    return mirror, sha


def arm_paths(work: Path, arm: str) -> dict[str, Path]:
    root = work / "arms" / arm
    home = root / "home"
    return {
        "root": root,
        "home": home,
        "claude": root / ".claude",
        "opencode": home / ".config" / "opencode",
        "xdg_config": home / ".config",
        "xdg_cache": home / ".cache",
        "xdg_data": home / ".local" / "share",
        "worktree": work / "worktrees" / arm / "repo",
    }


def arm_env(paths: dict[str, Path], extra: dict[str, str] | None = None) -> dict[str, str]:
    env = os.environ.copy()
    env["HOME"] = str(paths["home"])
    env["XDG_CONFIG_HOME"] = str(paths["xdg_config"])
    env["XDG_CACHE_HOME"] = str(paths["xdg_cache"])
    env["XDG_DATA_HOME"] = str(paths["xdg_data"])
    env["CLAUDE_CONFIG_DIR"] = str(paths["claude"])
    env["SUPEROPEN_INSTALL_DIR"] = str(paths["home"] / ".superopen" / "bin")
    bin_dir = str(paths["home"] / ".superopen" / "bin")
    path = env.get("PATH") or env.get("Path") or ""
    sep = os.pathsep
    if bin_dir not in path.split(sep):
        env["PATH"] = bin_dir + sep + path if path else bin_dir
        env["Path"] = env["PATH"]
    env.pop("SUPEROPEN_ROOT", None)
    if extra:
        env.update(extra)
    return env


def ensure_dirs(paths: dict[str, Path], host: str) -> None:
    for key in ("home", "xdg_config", "xdg_cache", "xdg_data"):
        paths[key].mkdir(parents=True, exist_ok=True)
    (paths["home"] / ".superopen" / "bin").mkdir(parents=True, exist_ok=True)
    if host == "claude-code":
        paths["claude"].mkdir(parents=True, exist_ok=True)
        settings = paths["claude"] / "settings.json"
        if not settings.exists():
            settings.write_text("{}\n")
    else:
        paths["opencode"].mkdir(parents=True, exist_ok=True)


def copy_auth(dest: Path, names: tuple[str, ...] = AUTH_FILES) -> list[str]:
    dest.mkdir(parents=True, exist_ok=True)
    if dest.name == ".claude":
        creds_json = os.environ.get("CLAUDE_CREDENTIALS_JSON", "").strip()
        if creds_json:
            path = dest / ".credentials.json"
            path.write_text(creds_json if creds_json.endswith("\n") else creds_json + "\n")
            os.chmod(path, 0o600)
            return [".credentials.json"]
        # Stale host OAuth breaks headless `claude -p`; API key billing works without it.
        if os.environ.get("ANTHROPIC_API_KEY"):
            for name in (*AUTH_FILES, ".credentials.json", "credentials.json"):
                (dest / name).unlink(missing_ok=True)
            return []
    copied: list[str] = []
    candidates = [
        Path.home() / ".config" / "opencode",
        Path.home() / ".claude",
        Path.home() / ".local" / "share" / "opencode",
    ]
    for src in candidates:
        if not src.is_dir():
            continue
        for name in names:
            p = src / name
            if p.is_file() and not (dest / name).exists():
                shutil.copy2(p, dest / name)
                copied.append(name)
        for extra in (".credentials.json", "credentials.json"):
            p = src / extra
            if p.is_file() and not (dest / extra).exists():
                shutil.copy2(p, dest / extra)
                copied.append(extra)
    claude_json = Path.home() / ".claude.json"
    if claude_json.is_file():
        target = dest / ".claude.json"
        if dest.name != ".claude":
            target = dest.parent / ".claude.json"
        if not target.exists():
            shutil.copy2(claude_json, target)
            copied.append(".claude.json")
    return copied


def add_worktree(mirror: Path, worktree: Path) -> None:
    if worktree.is_dir():
        shutil.rmtree(worktree, ignore_errors=True)
    worktree.parent.mkdir(parents=True, exist_ok=True)
    if _mode == ISOLATE_DOCKER:
        proc = host_run(
            ["git", "clone", "--local", "--no-hardlinks", str(mirror), str(worktree)],
            timeout=600,
        )
    else:
        proc = host_run(["git", "worktree", "add", "--detach", str(worktree), "HEAD"], cwd=mirror)
    if proc.returncode != 0:
        raise RuntimeError(proc.stderr or proc.stdout or "worktree add failed")


def parse_graph_status(stdout: str) -> dict[str, Any]:
    try:
        return json.loads(stdout)
    except json.JSONDecodeError:
        return {}


def _cleanup_docker() -> None:
    if _mode != ISOLATE_DOCKER:
        return
    import docker as bench_docker

    bench_docker.stop_all()


def stop_docker() -> None:
    _cleanup_docker()


atexit.register(_cleanup_docker)
