#!/usr/bin/env python3
"""Superopen benchmark harness — mode router."""

from __future__ import annotations

import argparse
import json
import shutil
import subprocess
import sys
import tempfile
import time
from datetime import datetime, timezone
from pathlib import Path

BENCH = Path(__file__).resolve().parent
REPO = BENCH.parent
sys.path.insert(0, str(BENCH))


def load_repo_env(repo: Path | None = None) -> None:
    """Load repo-root `.env` into os.environ (plain KEY=val lines; no export required)."""
    import os

    path = (repo or REPO) / ".env"
    if not path.is_file():
        return
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].strip()
        if "=" not in line:
            continue
        key, _, val = line.partition("=")
        key = key.strip()
        if not key:
            continue
        val = val.strip()
        if len(val) >= 2 and val[0] == val[-1] and val[0] in "\"'":
            val = val[1:-1]
        os.environ[key] = val

from compare import run_compare_mode  # noqa: E402
from graph import run_graph_mode, run_index, run_temporal_mode  # noqa: E402
from memory.runner import run_contradiction_mode, run_latency_mode, run_memory_mode  # noqa: E402
from spend import SpendLedger  # noqa: E402
from debug import run_debug_mode  # noqa: E402
from scale import apply_scale, default_model, require_agent_credentials, require_coding_host, validate_sizes  # noqa: E402
from swe import run_swe_mode  # noqa: E402


ALL_MODES = (
    "offline",
    "memory",
    "graph",
    "compare",
    "swe",
    "contradict",
    "latency",
    "index",
    "temporal",
    "all",
)
PUBLIC_MODES = ALL_MODES


def work_dir(*, keep: bool, out: str | None) -> Path:
    """Ephemeral harness dir. Kept on disk only when keep=True."""
    if keep:
        if out:
            p = Path(out)
        else:
            ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
            p = BENCH / "results" / ts
        p.mkdir(parents=True, exist_ok=True)
        return p.resolve()
    (BENCH / "work").mkdir(parents=True, exist_ok=True)
    return Path(tempfile.mkdtemp(prefix="so-bench-", dir=str(BENCH / "work"))).resolve()


PRODUCT_MODES = frozenset({"memory", "graph", "compare", "swe", "index", "temporal"})


def resolve_so_bin(path: str | None) -> str:
    if path:
        return str(Path(path).resolve())
    for candidate in (REPO / "bin" / "so", Path("/tmp/so")):
        if candidate.is_file():
            return str(candidate)
    return "so"


def setup_isolate(isolate_flag: str, modes: list[str], so_bin: str) -> str:
    import isolate

    import docker as bench_docker

    need = isolate_flag == "docker" and any(m in PRODUCT_MODES for m in modes)
    if not need:
        isolate.set_mode(isolate.ISOLATE_HOST)
        return so_bin
    if not bench_docker.docker_available():
        raise SystemExit(
            "product benchmarks default to Docker isolation so Claude/OpenCode "
            "never see developer HOME. Start Docker Desktop, or pass --isolate host."
        )
    print("=== isolate docker ===", flush=True)
    print("=== bind-mount local Superopen CLI at /usr/local/bin/so ===", flush=True)
    bench_docker.build_image()
    guest = bench_docker.prepare_guest_so(so_bin, REPO)
    isolate.set_mode(isolate.ISOLATE_DOCKER)
    isolate.set_linux_so(str(guest))
    return str(guest)


def run_offline(so_bin: str, out: Path) -> int:
    tests = BENCH / "tests" / "test_harness.py"
    proc = subprocess.run([sys.executable, str(tests)], cwd=str(REPO))
    payload = {"ok": proc.returncode == 0}
    (out / "offline.json").write_text(json.dumps(payload, indent=2) + "\n")
    return proc.returncode


def run_index_mode(args: argparse.Namespace, out: Path, so_bin: str) -> dict:
    import isolate
    from graph import run_index

    cache = Path("benchmarks/cache")
    mirror, sha = isolate.ensure_django_mirror(cache)
    work = out / "index"
    paths = isolate.arm_paths(work, "superopen")
    isolate.ensure_dirs(paths, "opencode")
    env = isolate.arm_env(paths, {"SUPEROPEN_SO_BIN": so_bin})
    isolate.add_worktree(mirror, paths["worktree"])
    isolate.ensure_container(paths, so_bin)
    result = run_index(so_bin, paths["worktree"], env, args.index_timeout)
    result["sha"] = sha
    (out / "index.json").write_text(json.dumps(result, indent=2) + "\n")
    return result


def parse_modes(raw: str) -> list[str]:
    modes = [m.strip() for m in raw.split(",") if m.strip()]
    if "all" in modes:
        expanded = [
            "offline",
            "contradict",
            "latency",
            "graph",
            "memory",
            "compare",
            "temporal",
            "swe",
        ]
        extra = [m for m in modes if m not in {"all", "debug"} and m not in expanded]
        return expanded + extra
    return [m for m in modes if m != "debug"]


def apply_all_defaults(args: Any, requested: list[str]) -> None:
    """`--mode all` writes a complete BENCHMARKS.md: memory QA + official SWE grades."""
    if "all" not in requested:
        return
    args.phase = 3
    if not getattr(args, "no_swe_grade", False):
        args.swe_grade = True


def main() -> int:
    load_repo_env()
    parser = argparse.ArgumentParser(description="Superopen benchmarks")
    parser.add_argument("--mode", default="offline", help=f"comma list or all ({', '.join(PUBLIC_MODES)})")
    parser.add_argument("--phase", type=int, default=2, choices=(1, 2, 3))
    parser.add_argument("--split", default="locomo", choices=("locomo", "longmemeval"))
    parser.add_argument("--scale", default="small", choices=("small", "full"), help="small = valid gate; full = publishable")
    parser.add_argument("--n", type=int, default=None, help="Override QA/retrieve count (must meet scale minimums)")
    parser.add_argument("--adapters", default="superopen,bm25,bow,rrf")
    parser.add_argument("--max-spend", type=float, default=0.0, dest="max_spend")
    parser.add_argument("--repo", default="django")
    parser.add_argument("--host", default="claude-code", choices=("opencode", "claude-code"))
    parser.add_argument("--model", default=None)
    parser.add_argument("--index-timeout", type=int, default=1800, dest="index_timeout")
    parser.add_argument("--agent-timeout", type=int, default=900, dest="agent_timeout")
    parser.add_argument("--so-bin", default=None, dest="so_bin")
    parser.add_argument(
        "--isolate",
        default="docker",
        choices=("docker", "host"),
        help="docker (default for product modes): bind-mount local so into an isolated container. host: isolated HOME on this machine",
    )
    parser.add_argument("--out", default=None, help=argparse.SUPPRESS)
    parser.add_argument("--qa-n", type=int, default=None, dest="qa_n", help="Memory QA sample size (default = --n)")
    parser.add_argument("--compare-n", type=int, default=None, dest="compare_n", help="Compare question cap (min 6)")
    parser.add_argument("--compare-ids", default="", dest="compare_ids", help="Comma-separated compare question ids")
    parser.add_argument("--compare-arms", default="native,superopen", dest="compare_arms", help="Comma-separated compare arms to run (native,superopen)")
    parser.add_argument("--compare-baseline", default="", dest="compare_baseline", help="Existing compare.json whose rows fill arms not in --compare-arms")
    parser.add_argument("--swe-n", type=int, default=None, dest="swe_n", help="SWE-bench instance cap (small=5 issues / 10 sessions, full=50)")
    parser.add_argument("--swe-ids", default="", dest="swe_ids", help="Comma-separated SWE-bench short ids or instance_ids")
    parser.add_argument("--swe-grade", action="store_true", default=False, dest="swe_grade", help="Run official swebench 4.1.0 grader on collected patches")
    parser.add_argument("--swe-arms", default="native,superopen", dest="swe_arms", help="Comma-separated SWE arms to run (native,superopen)")
    parser.add_argument("--swe-baseline", default="", dest="swe_baseline", help="Existing swe.json whose rows fill arms not in --swe-arms")
    parser.add_argument(
        "--fill-from",
        default="",
        dest="fill_from",
        help="Comma-separated result dirs whose missing suites fill BENCHMARKS.md",
    )
    parser.add_argument(
        "--no-fill",
        action="store_true",
        default=False,
        dest="no_fill",
        help="Do not merge previous suite scores into BENCHMARKS.md",
    )
    parser.add_argument("--debug", action="store_true", default=False, help=argparse.SUPPRESS)
    parser.add_argument(
        "--also-lme",
        action="store_true",
        default=False,
        dest="also_lme",
        help="After locomo memory, also run LongMemEval-S when the dataset file exists",
    )
    args = parser.parse_args()
    args.host = require_coding_host(args.host)
    args.model = default_model(args.host, args.model or "")
    apply_scale(args)
    validate_sizes(args)

    keep = bool(args.debug)
    out = work_dir(keep=keep, out=args.out)
    host_so = resolve_so_bin(args.so_bin)
    requested = [m.strip() for m in args.mode.split(",") if m.strip()]
    modes = parse_modes(args.mode)
    require_agent_credentials(modes=modes, phase=args.phase, max_spend=args.max_spend)
    if any(m == "memory" or m == "all" for m in requested):
        from memory.fetch_datasets import ensure_readme, maybe_fetch_from_env

        ensure_readme()
        maybe_fetch_from_env()
    so_bin = setup_isolate(args.isolate, modes, host_so)
    ledger = SpendLedger(max_spend=args.max_spend)
    summary: dict = {
        "modes": modes,
        "scale": args.scale,
        "host": args.host,
        "model": args.model,
        "n": args.n,
        "isolate": args.isolate if any(m in PRODUCT_MODES for m in modes) else "host",
        "so_bin": so_bin,
    }
    from report import collect_machine, merge_previous_suites, parse_fill_from, save_last_summary, write_benchmarks_md

    summary["machine"] = collect_machine()
    started = time.perf_counter()
    mode_duration: dict[str, float] = {}

    import isolate

    try:
        for mode in modes:
            if mode == "index" and "graph" in modes and "index" not in requested:
                print("=== mode index skipped; graph so init fills the index row ===", flush=True)
                continue
            print(f"=== mode {mode} ===", flush=True)
            t0 = time.perf_counter()
            if mode == "offline":
                code = run_offline(so_bin, out)
                if code != 0:
                    return code
            elif mode == "memory":
                summary["memory"] = run_memory_mode(args, out, so_bin, ledger)
                if isinstance(summary["memory"], dict) and summary["memory"].get("embedder_id"):
                    summary["embedder_id"] = summary["memory"]["embedder_id"]
                lme = Path("benchmarks/datasets/longmemeval/longmemeval_s.json")
                if (args.also_lme or "all" in requested) and lme.is_file() and args.split == "locomo":
                    saved_split, saved_n = args.split, args.n
                    args.split, args.n = "longmemeval", 50
                    print("=== mode memory (longmemeval) ===", flush=True)
                    lme_t0 = time.perf_counter()
                    summary["memory_longmemeval"] = run_memory_mode(args, out, so_bin, ledger)
                    mode_duration["memory_longmemeval"] = time.perf_counter() - lme_t0
                    args.split, args.n = saved_split, saved_n
            elif mode == "graph":
                summary["graph"] = run_graph_mode(args, out, so_bin)
            elif mode == "compare":
                summary["compare"] = run_compare_mode(args, out, so_bin, ledger)
            elif mode == "swe":
                summary["swe"] = run_swe_mode(args, out, so_bin, ledger)
            elif mode == "contradict":
                summary["contradict"] = run_contradiction_mode(out)
            elif mode == "latency":
                summary["latency"] = run_latency_mode(out, host_so)
            elif mode == "index":
                summary["index"] = run_index_mode(args, out, so_bin)
            elif mode == "temporal":
                summary["temporal"] = run_temporal_mode(args, out, so_bin)
            else:
                raise SystemExit(f"unknown mode: {mode}")
            mode_duration[mode] = mode_duration.get(mode, 0.0) + (time.perf_counter() - t0)

        if keep:
            summary["debug"] = run_debug_mode(out, so_bin)

        try:
            ledger.check()
        except RuntimeError as exc:
            summary["spend_stopped"] = str(exc)

        summary["duration_sec"] = time.perf_counter() - started
        summary["mode_duration_sec"] = {k: round(v, 3) for k, v in mode_duration.items()}
        summary["total_cost_usd"] = round(ledger.total_usd, 6)
        if not args.no_fill:
            extras = parse_fill_from(args.fill_from)
            summary = merge_previous_suites(summary, fill_from=extras)

        md = write_benchmarks_md(summary, dest=REPO / "BENCHMARKS.md")
        print(f"benchmarks.md: {md}", flush=True)
        if not args.no_fill:
            save_last_summary(summary)
        if keep:
            ledger.write(out / "spend.json")
            (out / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
            write_benchmarks_md(summary, dest=out / "BENCHMARKS.md")
    finally:
        isolate.stop_docker()
        if not keep:
            shutil.rmtree(out, ignore_errors=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
