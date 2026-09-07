"""Docker isolation for product benchmarks. Host HOME is never mounted."""

from __future__ import annotations

import hashlib
import os
import platform
import shutil
import subprocess
from pathlib import Path

IMAGE = "so-bench"
DOCKERFILE = Path(__file__).resolve().parent / "docker" / "Dockerfile"
CLAUDE_CODE_PIN = "2.1.241"  # published 2026-08-22; >= 72h old
OPENCODE_PIN = "1.18.21"  # published 2026-08-21; >= 72h old

_containers: dict[str, str] = {}


def docker_available() -> bool:
    if shutil.which("docker") is None:
        return False
    proc = subprocess.run(["docker", "info"], capture_output=True, text=True, timeout=20)
    return proc.returncode == 0


def docker_arch() -> str:
    machine = platform.machine().lower()
    if machine in {"arm64", "aarch64"}:
        return "arm64"
    return "amd64"


def linux_so_path(repo: Path) -> Path:
    return (repo / "benchmarks" / "cache" / "so-linux").resolve()


def is_linux_elf(path: Path) -> bool:
    try:
        return path.is_file() and path.read_bytes()[:4] == b"\x7fELF"
    except OSError:
        return False


def prepare_guest_so(so_bin: str, repo: Path) -> Path:
    """Bind-mount the local Superopen CLI. Never bake `so` into the image.

    A Linux ELF (`--so-bin` built on Linux, or a prior linux compile) is used
    as-is. Darwin `bin/so` cannot exec in a Linux container, so we compile this
    same checkout for Linux and mount that artifact.
    """
    src = Path(so_bin)
    if is_linux_elf(src):
        return src.resolve()
    print(
        f"=== isolate docker: mounting a Linux build of this tree "
        f"(host {src} is not a Linux ELF) ===",
        flush=True,
    )
    return ensure_linux_so(repo, newer_than=src if src.is_file() else None)


def forbidden_volume_source(source: str, home: Path | None = None) -> str | None:
    home = home or Path.home()
    src = Path(source).expanduser()
    try:
        resolved = src.resolve()
    except OSError:
        resolved = src
    claude = (home / ".claude").resolve()
    opencode = (home / ".config" / "opencode").resolve()
    if resolved == home.resolve():
        return str(resolved)
    if resolved == claude or claude in resolved.parents:
        return str(resolved)
    if resolved == opencode or opencode in resolved.parents:
        return str(resolved)
    return None


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


def build_image() -> None:
    inspect = subprocess.run(["docker", "image", "inspect", IMAGE], capture_output=True, text=True)
    if inspect.returncode == 0:
        print(f"=== isolate docker: reusing image {IMAGE} ===", flush=True)
        return
    cmd = [
        "docker",
        "build",
        "--build-arg",
        f"CLAUDE_CODE_VERSION={CLAUDE_CODE_PIN}",
        "--build-arg",
        f"OPENCODE_VERSION={OPENCODE_PIN}",
        "-t",
        IMAGE,
        "-f",
        str(DOCKERFILE),
        str(DOCKERFILE.parent),
    ]
    proc = subprocess.run(cmd, text=True, capture_output=True)
    if proc.returncode != 0:
        raise RuntimeError(f"docker build failed: {proc.stderr or proc.stdout}")


def ensure_linux_so(repo: Path, newer_than: Path | None = None) -> Path:
    dest = linux_so_path(repo)
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.is_file() and dest.stat().st_size > 0:
        if newer_than is None or not newer_than.is_file() or dest.stat().st_mtime >= newer_than.stat().st_mtime:
            return dest
    cmd = [
        "docker",
        "run",
        "--rm",
        "-u",
        "0",
        "-v",
        f"{repo.resolve()}:/src",
        "-v",
        f"{dest.parent}:/out",
        "-w",
        "/src",
        "-e",
        "CGO_ENABLED=1",
        "-e",
        "GOCACHE=/tmp/gocache",
        "-e",
        "GOMODCACHE=/tmp/gomod",
        IMAGE,
        "go",
        "build",
        "-tags",
        "tsnative,sqlite_fts5",
        "-o",
        "/out/so-linux",
        "./cmd/so",
    ]
    assert_isolated(cmd)
    proc = subprocess.run(cmd, text=True, capture_output=True)
    if proc.returncode != 0:
        raise RuntimeError(f"linux so build failed: {proc.stderr or proc.stdout}")
    dest.chmod(0o755)
    return dest


def _guest_maps(worktree: Path, home: Path, claude: Path) -> list[tuple[str, str]]:
    return [
        (str(worktree.resolve()), "/work"),
        (str(home.resolve()), "/eval/home"),
        (str(claude.resolve()), "/eval/claude"),
    ]


def to_container_path(raw: str, maps: list[tuple[str, str]]) -> str:
    try:
        resolved = str(Path(raw).resolve())
    except OSError:
        return raw
    for host, guest in maps:
        if resolved == host:
            return guest
        prefix = host.rstrip("/\\") + os.sep
        if resolved.startswith(prefix):
            rest = resolved[len(host) :].replace("\\", "/")
            if not rest.startswith("/"):
                rest = "/" + rest
            return guest + rest
    return raw


def _is_so_bin(arg: str, so_host: str | None) -> bool:
    if Path(arg).name in {"so", "so.exe", "so-linux"}:
        return True
    if not so_host:
        return False
    try:
        return Path(arg).resolve() == Path(so_host).resolve()
    except OSError:
        return False


def rewrite_cmd(cmd: list[str], maps: list[tuple[str, str]], so_host: str | None) -> list[str]:
    out: list[str] = []
    for i, arg in enumerate(cmd):
        if i == 0 and _is_so_bin(arg, so_host):
            out.append("/usr/local/bin/so")
            continue
        if arg.startswith("/") or (len(arg) > 2 and arg[1] == ":"):
            out.append(to_container_path(arg, maps))
            continue
        out.append(arg)
    return out


def container_name(home: Path) -> str:
    digest = hashlib.sha1(str(home.resolve()).encode()).hexdigest()[:12]
    return f"so-bench-{digest}"


def _host_user() -> str:
    if hasattr(os, "getuid"):
        return f"{os.getuid()}:{os.getgid()}"
    return "1000:1000"


def start_arm_argv(
    *,
    worktree: Path,
    home: Path,
    claude: Path,
    so_linux: Path,
    name: str | None = None,
) -> list[str]:
    name = name or container_name(home)
    argv = [
        "docker",
        "run",
        "-d",
        "--name",
        name,
        "--add-host",
        "host.docker.internal:host-gateway",
        "-e",
        "HOME=/eval/home",
        "-e",
        "CLAUDE_CONFIG_DIR=/eval/claude",
        "-e",
        "SUPEROPEN_SO_BIN=/usr/local/bin/so",
        "-e",
        "SO_EMBED_URL=http://host.docker.internal:18765",
        "-e",
        "PATH=/usr/local/bin:/usr/bin:/bin",
        "-w",
        "/work",
        "-v",
        f"{worktree.resolve()}:/work",
        "-v",
        f"{home.resolve()}:/eval/home",
        "-v",
        f"{claude.resolve()}:/eval/claude",
        "-v",
        f"{so_linux.resolve()}:/usr/local/bin/so:ro",
        "--user",
        _host_user(),
    ]
    for key in ("ANTHROPIC_API_KEY", "OPENAI_API_KEY", "OPENCODE_API_KEY"):
        if os.environ.get(key):
            argv.extend(["-e", key])
    argv.append(IMAGE)
    argv.extend(["sleep", "infinity"])
    assert_isolated(argv)
    return argv


def start_arm(
    *,
    worktree: Path,
    home: Path,
    claude: Path,
    so_linux: Path,
) -> str:
    name = container_name(home)
    if name in _containers:
        return _containers[name]
    worktree.mkdir(parents=True, exist_ok=True)
    home.mkdir(parents=True, exist_ok=True)
    claude.mkdir(parents=True, exist_ok=True)
    argv = start_arm_argv(worktree=worktree, home=home, claude=claude, so_linux=so_linux, name=name)
    subprocess.run(["docker", "rm", "-f", name], capture_output=True, text=True)
    proc = subprocess.run(argv, text=True, capture_output=True)
    if proc.returncode != 0:
        raise RuntimeError(f"docker run failed: {proc.stderr or proc.stdout}")
    cid = (proc.stdout or "").strip() or name
    _containers[name] = cid
    return cid


def exec_cmd(
    cmd: list[str],
    *,
    worktree: Path,
    home: Path,
    claude: Path,
    so_host: str | None,
    env: dict[str, str] | None,
    timeout: int,
) -> subprocess.CompletedProcess[str]:
    name = container_name(home)
    if name not in _containers:
        so_linux = Path(so_host) if so_host else linux_so_path(Path.cwd())
        start_arm(worktree=worktree, home=home, claude=claude, so_linux=so_linux)
    maps = _guest_maps(worktree, home, claude)
    inner = rewrite_cmd(cmd, maps, so_host)
    argv = ["docker", "exec", "-w", "/work"]
    if env:
        for key in ("ANTHROPIC_API_KEY", "OPENAI_API_KEY", "OPENCODE_API_KEY", "SO_EMBED_URL"):
            if not env.get(key):
                continue
            val = env[key]
            if key == "SO_EMBED_URL":
                val = val.replace("127.0.0.1", "host.docker.internal").replace("localhost", "host.docker.internal")
            argv.extend(["-e", f"{key}={val}"])
    argv.append(container_name(home))
    argv.extend(inner)
    try:
        return subprocess.run(argv, text=True, capture_output=True, timeout=timeout)
    except subprocess.TimeoutExpired as exc:
        return subprocess.CompletedProcess(
            argv,
            124,
            stdout=exc.stdout or "",
            stderr=(exc.stderr or "") + "\ntimeout",
        )


def stop_all() -> None:
    for name in list(_containers):
        subprocess.run(["docker", "rm", "-f", name], capture_output=True, text=True)
        _containers.pop(name, None)
