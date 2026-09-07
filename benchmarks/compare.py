"""Native vs Superopen session compare."""

from __future__ import annotations

import json
import shutil
import time
from pathlib import Path
from typing import Any

import isolate
from hosts import claude_code, opencode
from grade import grade_answer, grade_gold_files, load_questions, wrap_code_prompt
from resume import load_baseline_rows, merge_arm_rows, selected_arms
from spend import SpendLedger


def _host_module(name: str):
    if name != "claude-code":
        return opencode
    return claude_code


def _prepare_superopen(
    so_bin: str,
    paths: dict[str, Path],
    env: dict[str, str],
    host: str,
    *,
    init: bool = True,
) -> None:
    vendor = "claude-code" if host == "claude-code" else "opencode"
    if init:
        isolate.run([so_bin, "init", "--root", str(paths["worktree"])], cwd=paths["worktree"], env=env)
    isolate.run(
        [so_bin, "install", f"--vendor={vendor}"],
        cwd=paths["worktree"],
        env=env,
    )
    dest = paths["home"] / ".superopen" / "bin" / "so"
    dest.parent.mkdir(parents=True, exist_ok=True)
    src = isolate.guest_so() or so_bin
    shutil.copy2(src, dest)


def select_compare_questions(questions: list[dict[str, Any]], args: Any) -> list[dict[str, Any]]:
    """Keep the 6-question Django bank; always append the sibling-file task."""
    sibling = [q for q in questions if q.get("suite") == "sibling"]
    bank = [q for q in questions if q.get("suite") != "sibling"]
    compare_ids = str(getattr(args, "compare_ids", "") or "")
    compare_n = int(getattr(args, "compare_n", 6) or 0)
    if compare_ids.strip():
        want = {x.strip() for x in compare_ids.split(",") if x.strip()}
        return [q for q in questions if str(q.get("id")) in want]
    if compare_n < 6:
        raise RuntimeError("compare requires the full 6-question Django bank (or --compare-ids)")
    return bank[:compare_n] + sibling


def _row(arm_name: str, q: dict[str, Any], metrics: dict[str, Any], grade: dict[str, Any], files: dict[str, Any], extra: dict[str, Any] | None = None) -> dict[str, Any]:
    row = {
        "arm": arm_name,
        "id": q["id"],
        "suite": q.get("suite") or "bank",
        "coverage": grade["coverage"],
        "gold_file_coverage": files.get("coverage"),
        "input_tokens": metrics.get("input_tokens", 0),
        "cache_read_tokens": metrics.get("cache_read_tokens", 0),
        "cache_creation_tokens": metrics.get("cache_creation_tokens", 0),
        "output_tokens": metrics.get("output_tokens", 0),
        "tool_calls": metrics.get("tool_calls", 0),
        "graph_calls": metrics.get("graph_calls", 0),
        "source_reads": metrics.get("source_reads", 0),
        "read_after_bodies": bool(metrics.get("read_after_bodies")),
        "tools_likely": int(metrics.get("output_tokens") or 0) >= 50,
        "verbose_output": int(metrics.get("output_tokens") or 0) >= 50,
        "so_invoked": bool(metrics.get("so_invoked")),
        "cost_usd": metrics.get("cost_usd"),
        "ok": metrics.get("ok"),
    }
    if extra:
        row.update(extra)
    return row


def run_compare_mode(args: Any, out: Path, so_bin: str, ledger: SpendLedger) -> dict[str, Any]:
    if not ledger.allow_llm():
        payload = {"skipped": "compare requires --max-spend > 0"}
        (out / "compare.json").write_text(json.dumps(payload, indent=2) + "\n")
        return payload

    started = time.perf_counter()

    host = _host_module(args.host)
    if not host.available():
        raise RuntimeError(f"host binary not on PATH: {args.host}")

    cache = Path("benchmarks/cache")
    mirror, sha = isolate.ensure_django_mirror(cache)
    questions = select_compare_questions(load_questions(Path("benchmarks/questions/django.json")), args)
    work = out / "compare"
    arms = selected_arms(getattr(args, "compare_arms", None))
    baseline = load_baseline_rows(
        getattr(args, "compare_baseline", None),
        {"native", "superopen"} - set(arms),
    )
    rows: list[dict[str, Any]] = []
    seed_so: Path | None = None
    if True in arms.values():
        seed_paths = isolate.arm_paths(work / "_seed", "superopen")
        isolate.ensure_dirs(seed_paths, args.host)
        if args.host == "opencode":
            isolate.copy_auth(seed_paths["opencode"])
        else:
            isolate.copy_auth(seed_paths["claude"])
        seed_env = isolate.arm_env(seed_paths, {"SUPEROPEN_SO_BIN": so_bin})
        isolate.add_worktree(mirror, seed_paths["worktree"])
        isolate.ensure_container(seed_paths, so_bin)
        _prepare_superopen(so_bin, seed_paths, seed_env, args.host)
        seed_so = seed_paths["worktree"]

    stopped = False
    for q in questions:
        if stopped:
            break
        for arm_name, use_so in arms.items():
            if ledger.max_spend > 0 and ledger.total_usd >= ledger.max_spend:
                stopped = True
                break
            paths = isolate.arm_paths(work / str(q["id"]), arm_name)
            isolate.ensure_dirs(paths, args.host)
            if args.host == "opencode":
                isolate.copy_auth(paths["opencode"])
            else:
                isolate.copy_auth(paths["claude"])
            env = isolate.arm_env(paths, {"SUPEROPEN_SO_BIN": so_bin})
            isolate.add_worktree(mirror, paths["worktree"])
            if use_so:
                if seed_so is None:
                    raise RuntimeError("compare superopen arm needs a seeded so init")
                isolate.copy_so_store(seed_so, paths["worktree"])
            isolate.ensure_container(paths, so_bin)
            if use_so:
                _prepare_superopen(so_bin, paths, env, args.host, init=False)
            prompt = wrap_code_prompt(q["prompt"])
            t0 = time.perf_counter()
            metrics = host.run_prompt(prompt, paths["worktree"], env, args.model, args.agent_timeout)
            stderr = str(metrics.get("stderr") or "")
            if not metrics.get("ok") and (
                "index.lock" in stderr or "timeout" in stderr.lower() or "Another git process" in stderr
            ):
                metrics = host.run_prompt(prompt, paths["worktree"], env, args.model, args.agent_timeout)
            extra: dict[str, Any] = {"wall_sec": time.perf_counter() - t0}
            grade = grade_answer(metrics.get("result", ""), q.get("key_facts") or [])
            files = grade_gold_files(metrics.get("result", ""), q.get("gold_files") or [])
            cost = metrics.get("cost_usd")
            if cost is not None:
                ledger.record(f"compare:{arm_name}:{q['id']}", cost)
                try:
                    ledger.check()
                except RuntimeError:
                    extra["stopped_on_spend"] = True
                    stopped = True
            rows.append(_row(arm_name, q, metrics, grade, files, extra))

    rows = merge_arm_rows(rows, baseline)

    native = [r for r in rows if r["arm"] == "native"]
    so_rows = [r for r in rows if r["arm"] == "superopen"]
    bank_native = [r for r in native if r.get("suite") != "sibling"]
    bank_so = [r for r in so_rows if r.get("suite") != "sibling"]
    sibling_native = [r for r in native if r.get("suite") == "sibling"]
    sibling_so = [r for r in so_rows if r.get("suite") == "sibling"]

    def _sum(group: list[dict[str, Any]], key: str) -> int | float:
        total = 0.0
        for r in group:
            total += float(r.get(key) or 0)
        return total

    def _avg(group: list[dict[str, Any]], key: str) -> float | None:
        if not group:
            return None
        return sum(float(r.get(key) or 0) for r in group) / len(group)

    payload = {
        "host": args.host,
        "model": args.model,
        "scale": getattr(args, "scale", "small"),
        "corpus": "django/django",
        "sha": sha,
        "rows": rows,
        "summary": {
            "native_coverage_avg": _avg(bank_native, "coverage"),
            "superopen_coverage_avg": _avg(bank_so, "coverage"),
            "sibling_native_coverage": _avg(sibling_native, "coverage"),
            "sibling_superopen_coverage": _avg(sibling_so, "coverage"),
            "sibling_native_gold_files": _avg(sibling_native, "gold_file_coverage"),
            "sibling_superopen_gold_files": _avg(sibling_so, "gold_file_coverage"),
            "native_tool_calls": int(_sum(native, "tool_calls")),
            "superopen_tool_calls": int(_sum(so_rows, "tool_calls")),
            "native_graph_calls": int(_sum(native, "graph_calls")),
            "superopen_graph_calls": int(_sum(so_rows, "graph_calls")),
            "native_source_reads": int(_sum(native, "source_reads")),
            "superopen_source_reads": int(_sum(so_rows, "source_reads")),
            "superopen_read_after_bodies": any(r.get("read_after_bodies") for r in so_rows),
            "native_uncached_input": int(_sum(native, "input_tokens")),
            "superopen_uncached_input": int(_sum(so_rows, "input_tokens")),
            "native_cache_read_tokens": int(_sum(native, "cache_read_tokens")),
            "superopen_cache_read_tokens": int(_sum(so_rows, "cache_read_tokens")),
            "native_output_tokens": int(_sum(native, "output_tokens")),
            "superopen_output_tokens": int(_sum(so_rows, "output_tokens")),
            "native_cost_usd": _sum(native, "cost_usd"),
            "superopen_cost_usd": _sum(so_rows, "cost_usd"),
            "native_wall_sec": _sum(native, "wall_sec"),
            "superopen_wall_sec": _sum(so_rows, "wall_sec"),
            "graph_first_unproven": not any(r.get("tools_likely") for r in native + so_rows),
        },
        "duration_sec": time.perf_counter() - started,
    }
    (out / "compare.json").write_text(json.dumps(payload, indent=2) + "\n")
    return payload
