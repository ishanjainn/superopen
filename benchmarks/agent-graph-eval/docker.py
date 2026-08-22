"""Docker isolation for eval arms. Claude runs in Linux; host HOME is not mounted."""

from __future__ import annotations

import os
import platform
import shutil
import subprocess
from pathlib import Path

IMAGE = "so-eval-claude"
DOCKERFILE = Path(__file__).resolve().parent / "Dockerfile"
CLAUDE_CODE_PIN = "2.1.234"  # >= 72h old at plan time; npm install --ignore-scripts


def docker_arch() -> str:
    machine = platform.machine().lower()
    if machine in {"arm64", "aarch64"}:
        return "arm64"
    return "amd64"


def linux_so_bin(repo_root: Path, dest: Path) -> list[str]:
    dest.parent.mkdir(parents=True, exist_ok=True)
    return [
        "go",
        "build",
        "-o",
        str(dest),
        "./cmd/so",
    ]


def linux_so_env() -> dict[str, str]:
    env = os.environ.copy()
    env["CGO_ENABLED"] = "0"
    env["GOOS"] = "linux"
    env["GOARCH"] = docker_arch()
    return env


def build_image_cmd() -> list[str]:
    return [
        "docker",
        "build",
        "--build-arg",
        f"CLAUDE_CODE_VERSION={CLAUDE_CODE_PIN}",
        "-t",
        IMAGE,
        "-f",
        str(DOCKERFILE),
        str(DOCKERFILE.parent),
    ]


def forbidden_volume_source(source: str, home: Path | None = None) -> str | None:
    home = home or Path.home()
    src = Path(source).expanduser()
    try:
        resolved = src.resolve()
    except OSError:
        resolved = src
    claude = (home / ".claude").resolve()
    if resolved == home.resolve():
        return str(resolved)
    if resolved == claude or claude in resolved.parents or resolved == claude:
        return str(resolved)
    if "/.claude/hooks" in str(resolved):
        return str(resolved)
    return None


def run_argv(
    inner: list[str],
    *,
    worktree: Path,
    claude_dir: Path,
    home: Path,
    so_bin: Path | None = None,
    out_dir: Path | None = None,
) -> list[str]:
    argv = [
        "docker",
        "run",
        "--rm",
        "--user",
        "1000:1000",
        "-e",
        "HOME=/eval/home",
        "-e",
        "CLAUDE_CONFIG_DIR=/eval/claude",
        "-e",
        "SUPEROPEN_ROOT=/work",
        "-w",
        "/work",
        "-v",
        f"{worktree.resolve()}:/work",
        "-v",
        f"{claude_dir.resolve()}:/eval/claude",
        "-v",
        f"{home.resolve()}:/eval/home",
    ]
    if so_bin is not None:
        argv.extend(["-e", "SUPEROPEN_SO_BIN=/usr/local/bin/so"])
        argv.extend(["-v", f"{so_bin.resolve()}:/usr/local/bin/so:ro"])
    if os.environ.get("ANTHROPIC_API_KEY"):
        argv.extend(["-e", "ANTHROPIC_API_KEY"])
    if out_dir is not None:
        argv.extend(["-v", f"{out_dir.resolve()}:/out"])
    argv.append(IMAGE)
    argv.extend(inner)
    return argv


def assert_isolated(argv: list[str], home: Path | None = None) -> None:
    home = home or Path.home()
    i = 0
    while i < len(argv):
        if argv[i] in {"-v", "--volume"} and i + 1 < len(argv):
            spec = argv[i + 1]
            source = spec.split(":", 1)[0]
            bad = forbidden_volume_source(source, home=home)
            if bad:
                raise RuntimeError(f"docker isolation leak: mounting {bad}")
            i += 2
            continue
        i += 1


def docker_available() -> bool:
    return shutil.which("docker") is not None


def build_linux_so(repo_root: Path, dest: Path) -> None:
    cmd = linux_so_bin(repo_root, dest)
    proc = subprocess.run(cmd, cwd=str(repo_root), env=linux_so_env(), text=True, capture_output=True)
    if proc.returncode != 0:
        raise RuntimeError(f"linux so build failed: {proc.stderr or proc.stdout}")
