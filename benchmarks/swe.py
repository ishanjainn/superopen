"""SWE-bench Verified: native Claude Code vs Superopen.

`--scale small` runs 5 Verified instances (10 sessions). `--scale full` runs 50
(28 named + 22 repo-stratified fills). Grading uses the official `swebench`
harness when installed (4.1.0): apply the patch, run maintainer tests, resolved
or not.
"""

from __future__ import annotations

import json
import subprocess
import sys
import time
import urllib.request
from pathlib import Path
from typing import Any

import isolate
from compare import _prepare_superopen
from grade import wrap_swe_prompt
from hosts import claude_code, opencode
from resume import load_baseline_rows, merge_arm_rows, selected_arms
from spend import SpendLedger

BENCH = Path(__file__).resolve().parent
SWE_JSON = BENCH / "questions" / "swebench_verified.json"
VERIFIED = "princeton-nlp/SWE-bench_Verified"
HF_ROWS = "https://datasets-server.huggingface.co/rows?dataset=princeton-nlp/SWE-bench_Verified&config=default&split=test&offset={offset}&length=100"


def _host_module(name: str):
    if name != "claude-code":
        return opencode
    return claude_code


def load_swe_bank() -> list[dict[str, Any]]:
    doc = json.loads(SWE_JSON.read_text())
    instances = doc.get("instances") if isinstance(doc, dict) else doc
    if not isinstance(instances, list) or not instances:
        raise ValueError(f"no instances in {SWE_JSON}")
    return instances


def select_swe_instances(questions: list[dict[str, Any]], args: Any) -> list[dict[str, Any]]:
    wanted_ids = str(getattr(args, "swe_ids", "") or "")
    if wanted_ids.strip():
        want = {x.strip() for x in wanted_ids.split(",") if x.strip()}
        selected = [q for q in questions if q.get("id") in want or q.get("instance_id") in want]
        missing = want - {q.get("id") for q in selected} - {q.get("instance_id") for q in selected}
        if missing:
            raise RuntimeError(f"unknown SWE-bench ids: {sorted(missing)}")
        return selected
    scale = str(getattr(args, "scale", "small") or "small")
    n = int(getattr(args, "swe_n", 0) or 0)
    wins = [q for q in questions if q.get("suite") == "small"]
    named = [q for q in questions if q.get("suite") in {"small", "named"}]
    if scale != "full":
        take = n if n else len(wins)
        return wins[:take]
    take = n if n else 50
    if take <= len(named):
        return named[:take]
    return questions[:take]


def _cache_dir() -> Path:
    path = BENCH / "cache" / "swebench"
    path.mkdir(parents=True, exist_ok=True)
    return path


def _instance_cache(instance_id: str) -> Path:
    return _cache_dir() / f"{instance_id}.json"


def fetch_verified_records(instance_ids: list[str]) -> dict[str, dict[str, Any]]:
    wanted = set(instance_ids)
    out: dict[str, dict[str, Any]] = {}
    for instance_id in instance_ids:
        path = _instance_cache(instance_id)
        if path.is_file():
            out[instance_id] = json.loads(path.read_text())
            wanted.discard(instance_id)
    if not wanted:
        return out
    offset = 0
    while wanted and offset < 500:
        url = HF_ROWS.format(offset=offset)
        with urllib.request.urlopen(url, timeout=60) as handle:
            payload = json.loads(handle.read().decode())
        rows = payload.get("rows") or []
        if not rows:
            break
        for item in rows:
            row = item.get("row") or {}
            instance_id = str(row.get("instance_id") or "")
            if instance_id not in wanted:
                continue
            slim = {
                "instance_id": instance_id,
                "repo": row.get("repo"),
                "base_commit": row.get("base_commit"),
                "problem_statement": row.get("problem_statement") or "",
                "FAIL_TO_PASS": row.get("FAIL_TO_PASS"),
                "PASS_TO_PASS": row.get("PASS_TO_PASS"),
            }
            _instance_cache(instance_id).write_text(json.dumps(slim) + "\n")
            out[instance_id] = slim
            wanted.discard(instance_id)
        offset += len(rows)
        if len(rows) < 100:
            break
    if wanted:
        raise RuntimeError(f"SWE-bench Verified records missing: {sorted(wanted)}")
    return out


def selected_swe_arms(args: Any) -> dict[str, bool]:
    return selected_arms(getattr(args, "swe_arms", None))


def load_swe_baseline(path: str | None, skip_arms: set[str]) -> list[dict[str, Any]]:
    return load_baseline_rows(path, skip_arms)


def merge_swe_rows(fresh: list[dict[str, Any]], baseline: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return merge_arm_rows(fresh, baseline)


_SUPEROPEN_IGNORE_BEGIN = "# BEGIN SUPEROPEN"
_SUPEROPEN_IGNORE_END = "# END SUPEROPEN"


def _drop_superopen_gitignore(patch: str) -> str:
    """Drop .gitignore hunks that only add the Superopen ignore fence."""
    if not patch:
        return ""
    chunks = [c for c in patch.split("diff --git ") if c]
    kept: list[str] = []
    for chunk in chunks:
        header = chunk.splitlines()[0] if chunk else ""
        paths = header.replace("a/", "").replace("b/", "")
        if ".gitignore" in paths.split() and _is_superopen_only_ignore_hunk(chunk):
            continue
        kept.append("diff --git " + chunk)
    return "".join(kept)


def _is_superopen_only_ignore_hunk(chunk: str) -> bool:
    added = [ln[1:] for ln in chunk.splitlines() if ln.startswith("+") and not ln.startswith("+++")]
    removed = [ln[1:] for ln in chunk.splitlines() if ln.startswith("-") and not ln.startswith("---")]
    if removed:
        return False
    meaningful = [ln.strip() for ln in added if ln.strip()]
    if _SUPEROPEN_IGNORE_BEGIN not in meaningful or _SUPEROPEN_IGNORE_END not in meaningful:
        return False
    for ln in meaningful:
        if ln in {_SUPEROPEN_IGNORE_BEGIN, _SUPEROPEN_IGNORE_END, ".so", ".so/"}:
            continue
        if ln.endswith(".so/"):
            continue
        return False
    return True


def _untracked_diffs(worktree: Path) -> str:
    listed = isolate.host_run(
        ["git", "-C", str(worktree), "ls-files", "--others", "--exclude-standard", "-z"],
        timeout=120,
    )
    if listed.returncode != 0 or not listed.stdout:
        return ""
    chunks: list[str] = []
    for rel in listed.stdout.split("\0"):
        if not rel:
            continue
        parts = Path(rel).parts
        if parts and (parts[0] == ".so" or ".so" in parts):
            continue
        proc = isolate.host_run(
            ["git", "-C", str(worktree), "diff", "--no-index", "--binary", "--", "/dev/null", rel],
            timeout=60,
        )
        if proc.returncode not in (0, 1) or not proc.stdout:
            continue
        chunks.append(proc.stdout)
    return "".join(chunks)


def collect_patch(worktree: Path) -> str:
    """Working-tree diff vs HEAD, excluding Superopen machine-local files.

    Reads the worktree (`git diff HEAD`) instead of `git add` + `diff --cached`
    so a leftover index.lock after a timed-out agent cannot silently yield an
    empty patch. `.so/` is excluded twice: gitignore plus pathspec. The
    Superopen `.gitignore` fence is stripped so it is not scored as model work.
    """
    proc = isolate.host_run(
        [
            "git",
            "-C",
            str(worktree),
            "diff",
            "HEAD",
            "--binary",
            "--",
            ".",
            ":(exclude).so",
            ":(exclude).so/**",
        ],
        timeout=120,
    )
    patch = proc.stdout or "" if proc.returncode in (0, 1) else ""
    patch += _untracked_diffs(worktree)
    return _drop_superopen_gitignore(patch)


def prediction_row(instance_id: str, model: str, patch: str) -> dict[str, Any]:
    return {
        "instance_id": instance_id,
        "model_name_or_path": model,
        "model_patch": patch,
    }


def _pct(new: float, old: float) -> str:
    if old <= 0:
        return "—"
    saved = (old - new) / old * 100.0
    sign = "+" if saved >= 0 else ""
    return f"{sign}{saved:.0f}%"


def _sum(rows: list[dict[str, Any]], key: str) -> float:
    return sum(float(r.get(key) or 0) for r in rows)


def _both_resolved(native: list[dict[str, Any]], so_rows: list[dict[str, Any]]) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    so_by = {r["id"]: r for r in so_rows}
    nat_keep = []
    so_keep = []
    for row in native:
        other = so_by.get(row["id"])
        if row.get("resolved") and other and other.get("resolved"):
            nat_keep.append(row)
            so_keep.append(other)
    return nat_keep, so_keep


def summarize_swe(native: list[dict[str, Any]], so_rows: list[dict[str, Any]]) -> dict[str, Any]:
    n = max(len(native), len(so_rows))
    native_res = sum(1 for r in native if r.get("resolved") is True)
    so_res = sum(1 for r in so_rows if r.get("resolved") is True)
    graded = all(r.get("resolved") is not None for r in native + so_rows) and bool(native or so_rows)
    eff_n, eff_s = _both_resolved(native, so_rows)
    if not eff_n:
        eff_n, eff_s = native, so_rows
    return {
        "n": n,
        "graded": graded,
        "native_resolved": native_res if graded else None,
        "superopen_resolved": so_res if graded else None,
        "native_resolved_pct": (native_res / n) if graded and n else None,
        "superopen_resolved_pct": (so_res / n) if graded and n else None,
        "native_tokens": int(_sum(eff_n, "input_tokens") + _sum(eff_n, "output_tokens") + _sum(eff_n, "cache_read_tokens") + _sum(eff_n, "cache_creation_tokens")),
        "superopen_tokens": int(_sum(eff_s, "input_tokens") + _sum(eff_s, "output_tokens") + _sum(eff_s, "cache_read_tokens") + _sum(eff_s, "cache_creation_tokens")),
        "native_cost_usd": _sum(eff_n, "cost_usd"),
        "superopen_cost_usd": _sum(eff_s, "cost_usd"),
        "native_tool_calls": int(_sum(eff_n, "tool_calls")),
        "superopen_tool_calls": int(_sum(eff_s, "tool_calls")),
        "native_api_requests": int(_sum(eff_n, "turns")),
        "superopen_api_requests": int(_sum(eff_s, "turns")),
        "native_wall_sec": _sum(eff_n, "wall_sec"),
        "superopen_wall_sec": _sum(eff_s, "wall_sec"),
        "efficiency_over": "both_resolved" if _both_resolved(native, so_rows)[0] else "all_completed",
    }


def swebench_python() -> str:
    venv = BENCH / "cache" / "swebench-venv" / "bin" / "python"
    if venv.is_file():
        return str(venv)
    return sys.executable


_pulled_eval_images: set[str] = set()


def eval_image_ref(instance_id: str) -> str:
    """Official x86_64 SWE-bench eval image."""
    slug = instance_id.lower().replace("__", "_1776_")
    return f"swebench/sweb.eval.x86_64.{slug}:latest"


def pull_eval_images(instance_ids: list[str]) -> None:
    """Pull SWE-bench eval images as linux/amd64 so Apple Silicon can grade."""
    seen: set[str] = set()
    for instance_id in instance_ids:
        if instance_id in seen or instance_id in _pulled_eval_images:
            continue
        seen.add(instance_id)
        ref = eval_image_ref(instance_id)
        print(f"=== swe grade pull {ref} (linux/amd64) ===", flush=True)
        proc = subprocess.run(
            ["docker", "pull", "--platform", "linux/amd64", ref],
            timeout=7200,
        )
        if proc.returncode != 0:
            raise RuntimeError(f"docker pull --platform linux/amd64 failed: {ref}")
        _pulled_eval_images.add(instance_id)


def _pred_instance_ids(preds_path: Path) -> list[str]:
    ids: list[str] = []
    for line in preds_path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        row = json.loads(line)
        iid = str(row.get("instance_id") or "")
        if iid:
            ids.append(iid)
    return ids


def parse_grade_report(data: dict[str, Any]) -> tuple[dict[str, bool], list[str]]:
    """Map official harness report to instance_id -> resolved. Errors are listed separately."""
    errors = [str(i) for i in (data.get("error_ids") or [])]
    errset = set(errors)
    resolved_ids = {str(i) for i in (data.get("resolved_ids") or [])}
    unresolved_ids = {str(i) for i in (data.get("unresolved_ids") or [])}
    submitted = [str(i) for i in (data.get("submitted_ids") or [])]
    if not submitted:
        raw = data.get("resolved") or {}
        if isinstance(raw, dict):
            return {str(k): bool(v) for k, v in raw.items()}, errors
        return {str(i): True for i in (data.get("resolved_ids") or [])}, errors
    out: dict[str, bool] = {}
    for iid in submitted:
        if iid in errset:
            continue
        if iid in resolved_ids:
            out[iid] = True
        elif iid in unresolved_ids:
            out[iid] = False
    return out, errors


def attach_savings(summary: dict[str, Any]) -> dict[str, Any]:
    summary["token_savings"] = _pct(summary["superopen_tokens"], summary["native_tokens"])
    summary["cost_savings"] = _pct(summary["superopen_cost_usd"], summary["native_cost_usd"])
    summary["tool_savings"] = _pct(summary["superopen_tool_calls"], summary["native_tool_calls"])
    summary["request_savings"] = _pct(summary["superopen_api_requests"], summary["native_api_requests"])
    summary["wall_savings"] = _pct(summary["superopen_wall_sec"], summary["native_wall_sec"])
    return summary


def apply_resolved(
    rows: list[dict[str, Any]],
    arm: str,
    resolved: dict[str, bool],
    errors: list[str],
) -> None:
    errset = set(errors)
    for row in rows:
        if row.get("arm") != arm:
            continue
        iid = str(row.get("instance_id") or "")
        if iid in errset:
            row["grade_error"] = row.get("grade_error") or "swebench eval image failed"
            continue
        if iid in resolved:
            row["resolved"] = resolved[iid]
            row.pop("grade_error", None)


def find_grade_report(work: Path, run_id: str) -> Path | None:
    """swebench 4.1.0 writes `{model}.{run_id}.json`, not `{run_id}.json`."""
    candidates = [work / f"{run_id}.json", work / "evaluation_results" / f"{run_id}.json"]
    candidates.extend(sorted(work.glob(f"*.{run_id}.json")))
    for path in candidates:
        if path.is_file():
            return path
    return None


def grade_predictions(preds_path: Path, run_id: str) -> tuple[dict[str, bool], list[str]]:
    """Official swebench 4.1.0 grader. Returns (instance_id -> resolved, error ids)."""
    py = swebench_python()
    # benchmarks/docker.py must not shadow the PyPI `docker` package swebench imports.
    probe = subprocess.run(
        [py, "-c", "import swebench"],
        capture_output=True,
        text=True,
        cwd=str(BENCH.parent),
    )
    if probe.returncode != 0:
        raise RuntimeError(
            "swebench 4.1.0 is required for --swe-grade. "
            "Create benchmarks/cache/swebench-venv and pip install swebench==4.1.0"
        )
    preds_path = preds_path.resolve()
    pull_eval_images(_pred_instance_ids(preds_path))
    proc = subprocess.run(
        [
            py,
            "-m",
            "swebench.harness.run_evaluation",
            "--dataset_name",
            VERIFIED,
            "--predictions_path",
            str(preds_path),
            "--max_workers",
            "1",
            "--run_id",
            run_id,
        ],
        cwd=str(preds_path.parent),
        timeout=36000,
    )
    report = find_grade_report(preds_path.parent, run_id)
    if report is not None:
        return parse_grade_report(json.loads(report.read_text()))
    if proc.returncode != 0:
        raise RuntimeError("swebench grader failed")
    raise RuntimeError(f"swebench report missing for run_id={run_id}")


def regrade_work_dir(out: Path) -> dict[str, Any]:
    """Grade existing native/superopen prediction files and rewrite swe.json."""
    payload = json.loads((out / "swe.json").read_text())
    rows = list(payload.get("rows") or [])
    work = out / "swe"
    for arm_name in ("native", "superopen"):
        path = work / f"{arm_name}.predictions.jsonl"
        if not path.is_file() or path.stat().st_size == 0:
            continue
        print(f"=== swe grade arm={arm_name} ===", flush=True)
        try:
            resolved, errors = grade_predictions(path, f"so-{arm_name}-amd64")
        except Exception as exc:
            print(f"=== swe grade {arm_name} failed: {exc} ===", flush=True)
            for row in rows:
                if row.get("arm") == arm_name:
                    row["grade_error"] = str(exc)
            continue
        apply_resolved(rows, arm_name, resolved, errors)
        if errors:
            print(f"=== swe grade {arm_name} errors: {errors} ===", flush=True)
    payload["rows"] = rows
    native = [r for r in rows if r.get("arm") == "native"]
    so_rows = [r for r in rows if r.get("arm") == "superopen"]
    payload["summary"] = attach_savings(summarize_swe(native, so_rows))
    (out / "swe.json").write_text(json.dumps(payload, indent=2) + "\n")
    summary_path = out / "summary.json"
    if summary_path.is_file():
        summary = json.loads(summary_path.read_text())
        summary["swe"] = payload
        summary_path.write_text(json.dumps(summary, indent=2) + "\n")
    return payload


def run_swe_mode(args: Any, out: Path, so_bin: str, ledger: SpendLedger) -> dict[str, Any]:
    if not ledger.allow_llm():
        payload = {"skipped": "swe requires --max-spend > 0"}
        (out / "swe.json").write_text(json.dumps(payload, indent=2) + "\n")
        return payload

    started = time.perf_counter()
    host = _host_module(args.host)
    if not host.available():
        raise RuntimeError(f"host binary not on PATH: {args.host}")

    bank = load_swe_bank()
    questions = select_swe_instances(bank, args)
    records = fetch_verified_records([str(q["instance_id"]) for q in questions])
    work = out / "swe"
    arms = selected_swe_arms(args)
    baseline = load_swe_baseline(
        getattr(args, "swe_baseline", None),
        {"native", "superopen"} - set(arms),
    )
    rows: list[dict[str, Any]] = []
    predictions: dict[str, list[dict[str, Any]]] = {"native": [], "superopen": []}
    timeout = max(int(getattr(args, "agent_timeout", 900) or 900), 1800)
    do_grade = bool(getattr(args, "swe_grade", False))

    for q in questions:
        instance_id = str(q["instance_id"])
        rec = records[instance_id]
        repo = str(rec["repo"])
        sha = str(rec["base_commit"])
        prompt = wrap_swe_prompt(str(rec.get("problem_statement") or ""))
        print(f"=== swe {q['id']} ({instance_id}) ===", flush=True)
        mirror = isolate.ensure_github_mirror(Path("benchmarks/cache"), repo)
        isolate.fetch_sha(mirror, sha)

        for arm_name, use_so in arms.items():
            if ledger.max_spend > 0 and ledger.total_usd >= ledger.max_spend:
                print("=== swe stopped: max-spend ===", flush=True)
                break
            print(f"=== swe {q['id']} arm={arm_name} ===", flush=True)
            paths = isolate.arm_paths(work / instance_id, arm_name)
            isolate.ensure_dirs(paths, args.host)
            if args.host == "opencode":
                isolate.copy_auth(paths["opencode"])
            else:
                isolate.copy_auth(paths["claude"])
            env = isolate.arm_env(paths, {"SUPEROPEN_SO_BIN": so_bin})
            isolate.checkout_sha(mirror, paths["worktree"], sha)
            isolate.ensure_container(paths, so_bin)
            if use_so:
                _prepare_superopen(so_bin, paths, env, args.host)
            t0 = time.perf_counter()
            metrics = host.run_prompt(prompt, paths["worktree"], env, args.model, timeout)
            wall = time.perf_counter() - t0
            patch = collect_patch(paths["worktree"])
            cost = metrics.get("cost_usd")
            extra: dict[str, Any] = {}
            if cost is not None:
                ledger.record(f"swe:{arm_name}:{q['id']}", cost)
                try:
                    ledger.check()
                except RuntimeError:
                    extra["stopped_on_spend"] = True
            print(
                f"=== swe {q['id']} arm={arm_name} done ok={metrics.get('ok')} "
                f"so_invoked={bool(metrics.get('so_invoked'))} cost={cost} "
                f"spend={ledger.total_usd:.2f} ===",
                flush=True,
            )
            row = {
                "arm": arm_name,
                "id": q["id"],
                "instance_id": instance_id,
                "suite": q.get("suite"),
                "repo": repo,
                "resolved": None,
                "patch_bytes": len(patch.encode()),
                "input_tokens": metrics.get("input_tokens", 0),
                "cache_read_tokens": metrics.get("cache_read_tokens", 0),
                "cache_creation_tokens": metrics.get("cache_creation_tokens", 0),
                "output_tokens": metrics.get("output_tokens", 0),
                "tool_calls": metrics.get("tool_calls", 0),
                "graph_calls": metrics.get("graph_calls", 0),
                "turns": metrics.get("turns") or 0,
                "cost_usd": cost,
                "wall_sec": wall,
                "ok": metrics.get("ok"),
                "so_invoked": bool(metrics.get("so_invoked")),
            }
            row.update(extra)
            rows.append(row)
            predictions[arm_name].append(prediction_row(instance_id, f"{args.model}-{arm_name}", patch))
            (work / instance_id / arm_name / "patch.diff").parent.mkdir(parents=True, exist_ok=True)
            (work / instance_id / arm_name / "patch.diff").write_text(patch)

    rows = merge_swe_rows(rows, baseline)

    for arm_name, preds in predictions.items():
        path = work / f"{arm_name}.predictions.jsonl"
        path.write_text("".join(json.dumps(p) + "\n" for p in preds))
        if do_grade and preds:
            try:
                resolved, errors = grade_predictions(path, f"so-{arm_name}")
            except Exception as exc:  # grader is optional; keep agent metrics
                for row in rows:
                    if row["arm"] == arm_name:
                        row["grade_error"] = str(exc)
            else:
                apply_resolved(rows, arm_name, resolved, errors)

    native = [r for r in rows if r["arm"] == "native"]
    so_rows = [r for r in rows if r["arm"] == "superopen"]
    payload = {
        "host": args.host,
        "model": args.model,
        "scale": getattr(args, "scale", "small"),
        "corpus": VERIFIED,
        "grader": "swebench==4.1.0",
        "source": "princeton-nlp/SWE-bench_Verified",
        "n": len(questions),
        "rows": rows,
        "summary": attach_savings(summarize_swe(native, so_rows)),
        "duration_sec": time.perf_counter() - started,
    }
    (out / "swe.json").write_text(json.dumps(payload, indent=2) + "\n")
    return payload
