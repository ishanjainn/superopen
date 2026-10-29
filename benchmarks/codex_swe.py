#!/usr/bin/env python3
"""Retained, paired Codex SWE-bench run with isolated base history and agent network."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import time
from pathlib import Path

import isolate
from grade import wrap_swe_prompt
from swe import (
    collect_patch,
    fetch_verified_records,
    find_grade_report,
    grade_predictions,
    load_swe_bank,
    prediction_row,
    summarize_swe,
)
from tokens import codex_luna_standard_credits, codex_usage_from_events, row_tokens


BENCH = Path(__file__).resolve().parent
REPO = BENCH.parent
MODEL = "gpt-5.6-luna"
SMALL_N = 5


def write_json(path: Path, value: object) -> None:
    path.write_text(json.dumps(value, indent=2) + "\n")


def write_run_report(out: Path, rows: list[dict]) -> None:
    lines = [
        "# Codex SWE-bench small run", "",
        f"Model: `{MODEL}`, high reasoning. Official SWE-bench grading after each arm.", "",
        ("Credits are estimated from [standard Luna token rates]"
         "(https://learn.chatgpt.com/docs/pricing#token-rates); Codex CLI does not report billed USD. "
         "Tokens include cached input once. Agent time excludes setup and grading; "
         "each Superopen arm performs a cold init and install (split in row.json). "
         "Grep/read counts classify shell command text, so they are approximate; "
         "a command containing several reads counts once."), "",
        "| Task | Arm | Resolved | Tokens | Est. credits | Agent s | Setup s | Graph | Failed graph | Grep | Source reads | History |",
        "| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
    ]
    for row in rows:
        lines.append("| " + " | ".join((
            str(row["instance_id"]), str(row["arm"]), str(row.get("resolved")),
            str(row_tokens(row)), f"{row.get('estimated_standard_credits', 0):.3f}",
            f"{row.get('agent_sec', 0):.1f}", f"{row.get('setup_sec', 0):.1f}",
            str(row.get("graph_commands", 0)), str(row.get("failed_graph_commands", 0)),
            str(row.get("grep_commands", 0)),
            str(row.get("source_read_commands", 0)), str(row.get("git_history_commands", 0)),
        )) + " |")
    lines.extend(("", "## Retained evidence", ""))
    for row in rows:
        root = f"swe/{row['instance_id']}/{row['arm']}/artifacts"
        lines.append(f"- `{row['instance_id']} / {row['arm']}`: [events]({root}/codex.events.jsonl), [cost]({root}/cost.json), [patch]({root}/patch.diff), [grader]({root}/grader.json), [row]({root}/row.json)")
    lines.append("")
    (out / "README.md").write_text("\n".join(lines))


def select_small_bank(requested_ids: str = "") -> list[dict]:
    bank = [q for q in load_swe_bank() if q.get("suite") == "small"]
    if requested_ids:
        wanted = set(requested_ids.split(","))
        bank = [q for q in bank if q["instance_id"] in wanted or q["id"] in wanted]
        if len(bank) != len(wanted):
            raise ValueError("Unknown or duplicate --ids")
        return bank
    return bank[:SMALL_N]


def command_counts(events: Path) -> dict[str, int]:
    counts = {"shell_commands": 0, "graph_commands": 0, "grep_commands": 0,
              "source_read_commands": 0, "git_history_commands": 0,
              "direct_discovery_commands": 0, "failed_shell_commands": 0,
              "failed_graph_commands": 0}
    for line in events.read_text().splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        item = event.get("item") or {}
        if event.get("type") != "item.completed" or item.get("type") != "command_execution":
            continue
        command = str(item.get("command") or "")
        counts["shell_commands"] += 1
        failed = item.get("exit_code") not in (None, 0)
        if failed:
            counts["failed_shell_commands"] += 1
        if "so graph " in command:
            counts["graph_commands"] += 1
            if failed:
                counts["failed_graph_commands"] += 1
        grep = any(word in command for word in ("rg ", "grep "))
        read = any(word in command for word in ("sed ", "cat ", "head ", "tail ")) and "/skills/so/SKILL.md" not in command
        history = any(word in command for word in ("git show", "git log", "git fsck"))
        if grep:
            counts["grep_commands"] += 1
        if read:
            counts["source_read_commands"] += 1
        if history:
            counts["git_history_commands"] += 1
        if grep or read or history:
            counts["direct_discovery_commands"] += 1
    return counts


def prepare_home(home: Path, use_so: bool, so_bin: Path, repo: Path,
                 python_bin: Path | None = None) -> tuple[dict[str, str], float, dict[str, float]]:
    codex_home = home / ".codex"
    codex_home.mkdir(parents=True, exist_ok=True)
    auth = Path.home() / ".codex" / "auth.json"
    if not auth.is_file():
        raise RuntimeError("Codex CLI auth.json is unavailable")
    shutil.copy2(auth, codex_home / "auth.json")
    os.chmod(codex_home / "auth.json", 0o600)
    env = os.environ.copy()
    env.update({"HOME": str(home), "CODEX_HOME": str(codex_home)})
    env["PATH"] = os.pathsep.join(("/opt/homebrew/bin", "/Applications/Xcode.app/Contents/Developer/usr/bin", env.get("PATH", "")))
    task_tmp = home / "tmp"
    task_tmp.mkdir(parents=True, exist_ok=True)
    env.update({"TMPDIR": str(task_tmp) + os.sep, "TMP": str(task_tmp), "TEMP": str(task_tmp)})
    started = time.perf_counter()
    setup_parts: dict[str, float] = {}
    readable: list[Path] = []
    if python_bin is not None:
        resolved_python = python_bin.resolve(strict=True)
        task_bin = home / "bin"
        task_bin.mkdir(parents=True, exist_ok=True)
        for name in ("python", "python3"):
            (task_bin / name).symlink_to(resolved_python)
        env["PATH"] = str(task_bin) + os.pathsep + env["PATH"]
        env["PYTHONPATH"] = str(repo) + os.pathsep + env.get("PYTHONPATH", "")
        library_paths = subprocess.check_output(
            [str(resolved_python), "-c", "import sys; print('\\n'.join(sys.path))"],
            text=True,
        ).splitlines()
        readable.extend((task_bin, resolved_python))
        readable.extend(Path(path) for path in library_paths if path.startswith("/") and Path(path).exists())
    if use_so:
        dest = home / ".superopen" / "bin" / "so"
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(so_bin, dest)
        env["PATH"] = str(dest.parent) + os.pathsep + env.get("PATH", "")
        for cmd in ([str(dest), "init", "--root", str(repo)], [str(dest), "install", "--vendor=codex"]):
            part_started = time.perf_counter()
            subprocess.run(cmd, cwd=repo, env=env, check=True, stdout=subprocess.DEVNULL)
            setup_parts[cmd[1] + "_sec"] = time.perf_counter() - part_started
        readable.extend((dest, home / ".agents" / "skills" / "so", codex_home / "AGENTS.md"))
    config = [
        'default_permissions = "swe"',
        'approval_policy = "never"',
        'allow_login_shell = false',
        'web_search = "disabled"',
        '[permissions.swe.filesystem]',
        '":minimal" = "read"',
        json.dumps(str(task_tmp)) + ' = "write"',
    ]
    if Path("/private/tmp").is_dir():
        config.append('"/private/tmp" = "write"')
    config.extend(json.dumps(str(path)) + ' = "read"' for path in readable)
    config.extend(('[permissions.swe.filesystem.":workspace_roots"]', '"." = "write"',
                   '[permissions.swe.network]', 'enabled = false'))
    (codex_home / "config.toml").write_text("\n".join(config) + "\n")
    return env, time.perf_counter() - started, setup_parts


def run_arm(instance: dict, arm: str, mirror: Path, out: Path, so_bin: Path,
            python_bin: Path | None = None) -> dict:
    instance_id = str(instance["instance_id"])
    root = out / "swe" / instance_id / arm
    artifacts = root / "artifacts"
    artifacts.mkdir(parents=True, exist_ok=True)
    prior = artifacts / "row.json"
    if prior.is_file():
        return json.loads(prior.read_text())
    repo = root / "repo"
    isolate.checkout_sha(mirror, repo, str(instance["base_commit"]))
    home = root / "home"
    started = time.perf_counter()
    env, setup_sec, setup_parts = prepare_home(home, arm == "superopen", so_bin, repo, python_bin)
    prompt = wrap_swe_prompt(str(instance.get("problem_statement") or ""))
    (artifacts / "prompt.txt").write_text(prompt)
    cmd = [
        "codex", "exec", "--json", "--dangerously-bypass-hook-trust",
        "-c", 'web_search="disabled"',
        "-c", 'approval_policy="never"', "-c", 'model_reasoning_effort="high"',
        "-m", MODEL, "-C", str(repo), prompt,
    ]
    agent_started = time.perf_counter()
    try:
        with (artifacts / "codex.events.jsonl").open("w") as events, (artifacts / "codex.stderr").open("w") as stderr:
            proc = subprocess.run(cmd, cwd=repo, env=env, stdout=events, stderr=stderr, timeout=1800)
        exit_code = proc.returncode
    except subprocess.TimeoutExpired:
        exit_code = 124
    finally:
        (home / ".codex" / "auth.json").unlink(missing_ok=True)
    agent_sec = time.perf_counter() - agent_started
    patch = collect_patch(repo)
    (artifacts / "patch.diff").write_text(patch)
    usage = codex_usage_from_events(artifacts / "codex.events.jsonl")
    counts = command_counts(artifacts / "codex.events.jsonl")
    cost = {
        "usd": None,
        "reason": "Codex CLI does not report dollar cost",
        "estimated_standard_credits": codex_luna_standard_credits(usage),
        "credit_rate_source": "https://learn.chatgpt.com/docs/pricing#token-rates",
        "usage": usage,
        "total_tokens": row_tokens(usage),
    }
    write_json(artifacts / "cost.json", cost)
    pred = prediction_row(instance_id, f"{MODEL}-{arm}", patch)
    predictions = artifacts / "prediction.jsonl"
    predictions.write_text(json.dumps(pred) + "\n")
    run_id = f"so-paired-{instance_id}-{arm}"
    grade_error = None
    resolved = None
    try:
        grades, errors = grade_predictions(predictions, run_id)
        resolved = grades.get(instance_id)
        if instance_id in errors:
            grade_error = "official grader error"
        report = find_grade_report(artifacts, run_id)
        if report:
            shutil.copy2(report, artifacts / "grader.json")
    except Exception as exc:
        grade_error = str(exc)
    row = {
        "arm": arm, "id": instance_id, "instance_id": instance_id,
        "repo": instance["repo"], "base_commit": instance["base_commit"],
        "resolved": resolved, "grade_error": grade_error,
        "patch_bytes": len(patch.encode()), "exit_code": exit_code,
        "setup_sec": setup_sec, "agent_sec": agent_sec,
        **setup_parts,
        "wall_sec": time.perf_counter() - started,
        "cost_usd": None, "estimated_standard_credits": cost["estimated_standard_credits"],
        "tool_calls": counts["shell_commands"],
        "graph_calls": counts["graph_commands"],
        **usage, **counts,
    }
    write_json(prior, row)
    return row


def refresh_rows(out: Path) -> list[dict]:
    """Recompute usage, credit estimates and discovery counts from retained events."""
    rows: list[dict] = []
    for prior in sorted(out.glob("swe/*/*/artifacts/row.json")):
        row = json.loads(prior.read_text())
        artifacts = prior.parent
        usage = codex_usage_from_events(artifacts / "codex.events.jsonl")
        counts = command_counts(artifacts / "codex.events.jsonl")
        credits = codex_luna_standard_credits(usage)
        row.update(usage)
        row.update(counts)
        row.update({"tool_calls": counts["shell_commands"], "graph_calls": counts["graph_commands"],
                    "estimated_standard_credits": credits})
        write_json(prior, row)
        write_json(artifacts / "cost.json", {
            "usd": None, "reason": "Codex CLI does not report dollar cost",
            "estimated_standard_credits": credits,
            "credit_rate_source": "https://learn.chatgpt.com/docs/pricing#token-rates",
            "usage": usage, "total_tokens": row_tokens(usage),
        })
        rows.append(row)
    if rows:
        summary = summarize_swe([r for r in rows if r["arm"] == "native"],
                                [r for r in rows if r["arm"] == "superopen"])
        write_json(out / "swe.json", {"host": "codex", "rows": rows, "summary": summary})
        write_run_report(out, rows)
    return rows


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--so-bin", type=Path, default=REPO / "bin" / "so")
    parser.add_argument("--python-bin", type=Path, help="Compatible Python executable for agent test commands")
    parser.add_argument("--ids", default="", help="Comma-separated SWE-bench IDs; default is all five small tasks")
    parser.add_argument("--arms", choices=("both", "native", "superopen"), default="both",
                        help="Run both arms or only the named arm")
    parser.add_argument("--refresh", action="store_true", help="Recompute retained costs and command counts without rerunning agents")
    args = parser.parse_args()
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=True)
    if args.refresh:
        print(f"refreshed {len(refresh_rows(out))} retained arms", flush=True)
        return
    so_bin = args.so_bin.resolve()
    python_bin = args.python_bin.resolve() if args.python_bin else None
    bank = select_small_bank(args.ids)
    records = fetch_verified_records([q["instance_id"] for q in bank])
    write_json(out / "manifest.json", {
        "model": MODEL, "reasoning_effort": "high", "sandbox": "swe permission profile",
        "agent_network": False, "agent_read": "workspace plus minimal runtime and installed graph tools",
        "web_search": "disabled", "history": "shallow base commit only",
        "so_sha256": hashlib.sha256(so_bin.read_bytes()).hexdigest(),
        "python_bin": str(python_bin) if python_bin else None,
        "arms": args.arms,
        "instance_ids": [q["instance_id"] for q in bank],
    })
    rows = []
    for q in bank:
        rec = records[q["instance_id"]]
        mirror = isolate.ensure_github_mirror(BENCH / "cache", str(rec["repo"]))
        isolate.fetch_sha(mirror, str(rec["base_commit"]))
        arms = ("native", "superopen") if args.arms == "both" else (args.arms,)
        for arm in arms:
            print(f"=== {q['instance_id']} {arm} ===", flush=True)
            row = run_arm(rec, arm, mirror, out, so_bin, python_bin)
            rows.append(row)
            print(f"resolved={row['resolved']} tokens={row_tokens(row)} agent_sec={row['agent_sec']:.1f} setup_sec={row['setup_sec']:.1f} graph={row['graph_commands']} direct={row['direct_discovery_commands']}", flush=True)
            write_json(out / "swe.json", {"host": "codex", "rows": rows})
    summary = summarize_swe([r for r in rows if r["arm"] == "native"], [r for r in rows if r["arm"] == "superopen"])
    write_json(out / "swe.json", {"host": "codex", "rows": rows, "summary": summary})
    write_run_report(out, rows)


if __name__ == "__main__":
    main()
