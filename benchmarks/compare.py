"""Native vs Superopen session compare."""

from __future__ import annotations

import json
import shutil
import time
from pathlib import Path
from typing import Any

import isolate
from hosts import claude_code, opencode
from grade import grade_answer, load_questions, wrap_code_prompt
from spend import SpendLedger


def _host_module(name: str):
    if name != "claude-code":
        return opencode
    return claude_code


def _prepare_superopen(so_bin: str, paths: dict[str, Path], env: dict[str, str], host: str) -> None:
    vendor = "claude-code" if host == "claude-code" else "opencode"
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
    questions = load_questions(Path("benchmarks/questions/django.json"))
    compare_ids = str(getattr(args, "compare_ids", "") or "")
    compare_n = int(getattr(args, "compare_n", 6) or 0)
    if compare_ids.strip():
        want = {x.strip() for x in compare_ids.split(",") if x.strip()}
        questions = [q for q in questions if str(q.get("id")) in want]
    else:
        if compare_n < 6:
            raise RuntimeError("compare requires the full 6-question Django bank (or --compare-ids)")
        questions = questions[:compare_n]
    work = out / "compare"
    arms = {"native": False, "superopen": True}
    rows: list[dict[str, Any]] = []

    for arm_name, use_so in arms.items():
        paths = isolate.arm_paths(work, arm_name)
        isolate.ensure_dirs(paths, args.host)
        if args.host == "opencode":
            isolate.copy_auth(paths["opencode"])
        else:
            isolate.copy_auth(paths["claude"])
        env = isolate.arm_env(paths, {"SUPEROPEN_SO_BIN": so_bin})
        isolate.add_worktree(mirror, paths["worktree"])
        isolate.ensure_container(paths, so_bin)
        if use_so:
            _prepare_superopen(so_bin, paths, env, args.host)

        for q in questions:
            if ledger.max_spend > 0 and ledger.total_usd >= ledger.max_spend:
                break
            prompt = wrap_code_prompt(q["prompt"])
            metrics = host.run_prompt(prompt, paths["worktree"], env, args.model, args.agent_timeout)
            stderr = str(metrics.get("stderr") or "")
            if not metrics.get("ok") and (
                "index.lock" in stderr or "timeout" in stderr.lower() or "Another git process" in stderr
            ):
                metrics = host.run_prompt(prompt, paths["worktree"], env, args.model, args.agent_timeout)
            verbose_output = int(metrics.get("output_tokens") or 0) >= 50
            so_invoked = bool(metrics.get("so_invoked"))
            grade = grade_answer(metrics.get("result", ""), q.get("key_facts") or [])
            cost = metrics.get("cost_usd")
            if cost is not None:
                ledger.record(f"compare:{arm_name}:{q['id']}", cost)
                try:
                    ledger.check()
                except RuntimeError:
                    rows.append(
                        {
                            "arm": arm_name,
                            "id": q["id"],
                            "coverage": grade["coverage"],
                            "input_tokens": metrics.get("input_tokens", 0),
                            "cache_read_tokens": metrics.get("cache_read_tokens", 0),
                            "cache_creation_tokens": metrics.get("cache_creation_tokens", 0),
                            "output_tokens": metrics.get("output_tokens", 0),
                            "tools_likely": verbose_output,
                            "verbose_output": verbose_output,
                            "so_invoked": so_invoked,
                            "cost_usd": cost,
                            "ok": metrics.get("ok"),
                            "stopped_on_spend": True,
                        }
                    )
                    break
            rows.append(
                {
                    "arm": arm_name,
                    "id": q["id"],
                    "coverage": grade["coverage"],
                    "input_tokens": metrics.get("input_tokens", 0),
                    "cache_read_tokens": metrics.get("cache_read_tokens", 0),
                    "cache_creation_tokens": metrics.get("cache_creation_tokens", 0),
                    "output_tokens": metrics.get("output_tokens", 0),
                    "tools_likely": verbose_output,
                    "verbose_output": verbose_output,
                    "so_invoked": so_invoked,
                    "cost_usd": cost,
                    "ok": metrics.get("ok"),
                }
            )

    native = [r for r in rows if r["arm"] == "native"]
    so_rows = [r for r in rows if r["arm"] == "superopen"]

    def _sum(rows: list[dict[str, Any]], key: str) -> int | float:
        total = 0.0
        for r in rows:
            total += float(r.get(key) or 0)
        return total

    payload = {
        "host": args.host,
        "model": args.model,
        "scale": getattr(args, "scale", "small"),
        "corpus": "django/django",
        "sha": sha,
        "rows": rows,
        "summary": {
            "native_coverage_avg": sum(r["coverage"] for r in native) / len(native) if native else None,
            "superopen_coverage_avg": sum(r["coverage"] for r in so_rows) / len(so_rows) if so_rows else None,
            "native_uncached_input": int(_sum(native, "input_tokens")),
            "superopen_uncached_input": int(_sum(so_rows, "input_tokens")),
            "native_cache_read_tokens": int(_sum(native, "cache_read_tokens")),
            "superopen_cache_read_tokens": int(_sum(so_rows, "cache_read_tokens")),
            "native_output_tokens": int(_sum(native, "output_tokens")),
            "superopen_output_tokens": int(_sum(so_rows, "output_tokens")),
            "native_cost_usd": _sum(native, "cost_usd"),
            "superopen_cost_usd": _sum(so_rows, "cost_usd"),
            "graph_first_unproven": not any(r.get("tools_likely") for r in native + so_rows),
        },
        "duration_sec": time.perf_counter() - started,
    }
    (out / "compare.json").write_text(json.dumps(payload, indent=2) + "\n")
    return payload
