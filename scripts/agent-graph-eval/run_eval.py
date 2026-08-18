#!/usr/bin/env python3
"""Agent graph eval harness: vanilla / Superopen / peer_cli / peer_mcp on a fixture repo."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path

QUESTION = (
    "How does Openlit.init work in the TypeScript SDK? "
    "What does it configure and which instrumentations does it enable?"
)

BASELINE = {
    "superopen": {"turns": 4, "cost": 0.085, "input_side": 347584},
    "peer_cli": {"turns": 6, "cost": 0.054, "input_side": 210806},
    "peer_mcp": {"turns": 3, "cost": 0.126, "input_side": 466083},
}


def run(cmd: list[str], cwd: Path | None = None, env: dict | None = None, timeout: int = 600) -> subprocess.CompletedProcess:
    merged = os.environ.copy()
    if env:
        merged.update(env)
    return subprocess.run(
        cmd,
        cwd=str(cwd) if cwd else None,
        env=merged,
        text=True,
        capture_output=True,
        timeout=timeout,
    )


def ensure_fixture(src: Path, work: Path) -> Path:
    repo = work / "repo"
    if repo.exists():
        shutil.rmtree(repo)
    repo.mkdir(parents=True)
    ignore = shutil.ignore_patterns("node_modules", ".so", ".git", "dist", "coverage", "*-out")
    shutil.copytree(src, repo, dirs_exist_ok=True, ignore=ignore)
    run(["git", "init", "-q"], cwd=repo)
    run(["git", "add", "-A"], cwd=repo)
    run(
        ["git", "-c", "user.email=eval@test", "-c", "user.name=eval", "commit", "-qm", "eval fixture"],
        cwd=repo,
    )
    return repo


def parse_usage(path: Path) -> dict:
    data = json.loads(path.read_text())
    usage = data.get("usage") or {}
    model_usage = data.get("modelUsage") or {}
    model = next(iter(model_usage.values()), {}) if model_usage else {}
    inp = model.get("inputTokens", usage.get("input_tokens", 0)) or 0
    cc = model.get("cacheCreationInputTokens", usage.get("cache_creation_input_tokens", 0)) or 0
    cr = model.get("cacheReadInputTokens", usage.get("cache_read_input_tokens", 0)) or 0
    out = model.get("outputTokens", usage.get("output_tokens", 0)) or 0
    result = data.get("result") or ""
    purity = {
        "mentions_graph": any(s in result.lower() for s in ("graph query", "graph_search", "so graph", "mcp")),
        "mentions_read": "source read" in result.lower() or "files read" in result.lower() or "`src/" in result,
    }
    return {
        "turns": data.get("num_turns"),
        "cost_usd": data.get("total_cost_usd"),
        "duration_ms": data.get("duration_ms"),
        "input_tokens": inp,
        "cache_create": cc,
        "cache_read": cr,
        "output_tokens": out,
        "input_side_total": inp + cc + cr,
        "result": result,
        "purity": purity,
        "is_error": data.get("is_error"),
    }


def claude_json(prompt: str, cwd: Path, out_path: Path, mcp_config: Path | None, model: str) -> dict:
    cmd = [
        "claude",
        "-p",
        "--model",
        model,
        "--dangerously-skip-permissions",
        "--output-format",
        "json",
        prompt,
    ]
    if mcp_config is not None:
        cmd = [
            "claude",
            "--mcp-config",
            str(mcp_config),
            "-p",
            "--model",
            model,
            "--dangerously-skip-permissions",
            "--output-format",
            "json",
            prompt,
        ]
    proc = run(cmd, cwd=cwd, timeout=300)
    out_path.write_text(proc.stdout or "")
    (out_path.with_suffix(".err")).write_text(proc.stderr or "")
    if not proc.stdout.strip():
        return {"error": "empty_stdout", "returncode": proc.returncode, "stderr": (proc.stderr or "")[-2000:]}
    try:
        metrics = parse_usage(out_path)
        metrics["returncode"] = proc.returncode
        return metrics
    except Exception as exc:  # noqa: BLE001
        return {"error": str(exc), "returncode": proc.returncode, "stdout_head": proc.stdout[:500]}


def prepare_so(repo: Path, so_bin: Path) -> dict:
    proc = run([str(so_bin), "init", "--force"], cwd=repo, timeout=300)
    (repo.parent / "so-init.out").write_text(proc.stdout + "\n" + proc.stderr)
    nodes = edges = None
    db = repo / ".so" / "db" / "so.db"
    if db.exists():
        q = run(["sqlite3", str(db), "SELECT COUNT(*) FROM nodes; SELECT COUNT(*) FROM edges;"])
        lines = [ln for ln in q.stdout.splitlines() if ln.strip()]
        if len(lines) >= 2:
            nodes, edges = int(lines[0]), int(lines[1])
    return {"nodes": nodes, "edges": edges, "ok": proc.returncode == 0}


def prepare_peer_cli(repo: Path, peer_cli: Path) -> dict:
    proc = run([str(peer_cli), "update", ".", "--force"], cwd=repo, timeout=300)
    (repo.parent / "peer-cli-build.out").write_text(proc.stdout + "\n" + proc.stderr)
    nodes = edges = None
    for line in (proc.stdout + proc.stderr).splitlines():
        if "Rebuilt:" in line and "nodes" in line:
            parts = line.replace(",", "").split()
            for i, p in enumerate(parts):
                if p == "nodes" and i > 0:
                    nodes = int(parts[i - 1])
                if p == "edges" and i > 0:
                    edges = int(parts[i - 1].rstrip(","))
    return {"nodes": nodes, "edges": edges, "ok": proc.returncode == 0}


def prepare_peer_mcp(repo: Path, peer_mcp: Path) -> dict:
    proc = run([str(peer_mcp), "cli", "--json", "index_repository", "--repo-path", str(repo)], timeout=600)
    (repo.parent / "peer-mcp-index.out").write_text(proc.stdout + "\n" + proc.stderr)
    nodes = edges = project = None
    try:
        outer = json.loads(proc.stdout)
        inner = json.loads(outer["content"][0]["text"]) if isinstance(outer, dict) and "content" in outer else outer
        nodes, edges, project = inner.get("nodes"), inner.get("edges"), inner.get("project")
    except Exception:  # noqa: BLE001
        pass
    mcp = {"mcpServers": {"peer-graph-mcp": {"command": str(peer_mcp), "args": []}}}
    mcp_path = repo.parent / "peer-mcp.json"
    mcp_path.write_text(json.dumps(mcp))
    return {"nodes": nodes, "edges": edges, "project": project, "mcp_config": str(mcp_path), "ok": proc.returncode == 0}


def arm_vanilla(repo: Path, model: str, out: Path) -> dict:
    prompt = f"""Answer using only Read/Grep/Glob (no graph tools, no Superopen CLI/MCP).
Question: {QUESTION}
Give a concise architecture answer, then report which files you read."""
    return claude_json(prompt, repo, out, None, model)


def arm_superopen(repo: Path, so_bin: Path, model: str, out: Path) -> dict:
    prompt = f"""Answer this question now (do not ask clarifying questions):
{QUESTION}

Use Superopen graph tools. Prefer this playbook and do not Read source until graph/snippet is insufficient:
1. Run: {so_bin} graph query "{QUESTION}"
2. Optionally: {so_bin} graph search Openlit.init
3. Optionally: {so_bin} graph snippet <qualified_name>
4. Optionally: {so_bin} graph architecture

Give a concise architecture answer, then report which tools/files you used."""
    return claude_json(prompt, repo, out, None, model)


def arm_peer_cli(repo: Path, peer_cli: Path, model: str, out: Path) -> dict:
    prompt = f"""Answer using the peer CLI graph tool only when possible.
First run exactly: {peer_cli} query "{QUESTION}"
Prefer graph over reading source. Only Read if graph context is insufficient.
Give a concise architecture answer, then report which tools/files you used."""
    return claude_json(prompt, repo, out, None, model)


def arm_peer_mcp(repo: Path, mcp_config: Path, project: str, model: str, out: Path) -> dict:
    prompt = f"""Answer using the peer graph MCP tools only when possible.
Project name: {project}
Prefer graph MCP search/trace/snippet/architecture tools over reading source files.
Question: {QUESTION}
Give a concise architecture answer, then report which MCP tools/files you used."""
    return claude_json(prompt, repo, out, mcp_config, model)


def write_summary(out_dir: Path, results: dict, graphs: dict) -> None:
    lines = [
        "# Agent graph eval summary",
        "",
        f"Generated: {datetime.now(timezone.utc).isoformat()}",
        f"Question: {QUESTION}",
        "",
        "## Graph sizes",
        "",
        "| Arm | Nodes | Edges |",
        "|-----|------:|------:|",
    ]
    for arm in ("superopen", "peer_cli", "peer_mcp"):
        g = graphs.get(arm) or {}
        lines.append(f"| {arm} | {g.get('nodes')} | {g.get('edges')} |")
    lines += [
        "",
        "## Metrics",
        "",
        "| Arm | Turns | Cost USD | Duration ms | Input | Cache create | Cache read | Output | Input-side total | Graph? | Read? |",
        "|-----|------:|---------:|------------:|------:|-------------:|-----------:|-------:|-----------------:|:------:|:-----:|",
    ]
    for arm in ("vanilla", "superopen", "peer_cli", "peer_mcp"):
        r = results.get(arm) or {}
        if r.get("error"):
            lines.append(f"| {arm} | ERR | | | | | | | | | |")
            continue
        p = r.get("purity") or {}
        lines.append(
            "| {arm} | {turns} | {cost:.6f} | {dur} | {inp} | {cc} | {cr} | {out} | {side} | {g} | {rd} |".format(
                arm=arm,
                turns=r.get("turns"),
                cost=float(r.get("cost_usd") or 0),
                dur=r.get("duration_ms"),
                inp=r.get("input_tokens"),
                cc=r.get("cache_create"),
                cr=r.get("cache_read"),
                out=r.get("output_tokens"),
                side=r.get("input_side_total"),
                g="Y" if p.get("mentions_graph") else "N",
                rd="Y" if p.get("mentions_read") else "N",
            )
        )
    lines += [
        "",
        "## Baseline (prior fair openlit run)",
        "",
        "| Arm | Turns | Cost | Input-side |",
        "|-----|------:|-----:|-----------:|",
    ]
    for arm, b in BASELINE.items():
        lines.append(f"| {arm} | {b['turns']} | {b['cost']} | {b['input_side']} |")
    lines += ["", "## Answer excerpts", ""]
    for arm in ("vanilla", "superopen", "peer_cli", "peer_mcp"):
        r = results.get(arm) or {}
        excerpt = (r.get("result") or r.get("error") or "")[:800].replace("\n", " ")
        lines.append(f"### {arm}")
        lines.append("")
        lines.append(excerpt)
        lines.append("")
    (out_dir / "summary.md").write_text("\n".join(lines) + "\n")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo-src", default=str(Path.home() / "private/openlit/sdk/typescript"))
    parser.add_argument("--work", default="")
    parser.add_argument("--out", default="")
    parser.add_argument("--model", default="haiku")
    parser.add_argument("--so-bin", default="/tmp/so")
    parser.add_argument("--peer-cli", default="", help="Optional peer CLI binary for comparison arm")
    parser.add_argument("--peer-mcp", default="", help="Optional peer MCP binary for comparison arm")
    parser.add_argument("--arms", default="vanilla,superopen")
    args = parser.parse_args()

    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    root = Path(__file__).resolve().parent
    work = Path(args.work) if args.work else root / "work" / stamp
    out_dir = Path(args.out) if args.out else root / "results" / stamp
    work.mkdir(parents=True, exist_ok=True)
    out_dir.mkdir(parents=True, exist_ok=True)

    src = Path(args.repo_src)
    if not src.is_dir():
        print(f"missing --repo-src {src}", file=sys.stderr)
        return 2

    print(f"fixture from {src} -> {work}")
    repo = ensure_fixture(src, work)
    so_bin = Path(args.so_bin)
    peer_cli = Path(args.peer_cli) if args.peer_cli else None
    peer_mcp = Path(args.peer_mcp) if args.peer_mcp else None
    arms = [a.strip() for a in args.arms.split(",") if a.strip()]

    graphs: dict = {}
    results: dict = {}

    if "superopen" in arms or "so" in arms:
        print("prepare superopen...")
        graphs["superopen"] = prepare_so(repo, so_bin)
        print("  ", graphs["superopen"])
    if "peer_cli" in arms:
        if not peer_cli:
            print("peer_cli arm requires --peer-cli", file=sys.stderr)
            return 2
        print("prepare peer_cli...")
        graphs["peer_cli"] = prepare_peer_cli(repo, peer_cli)
        print("  ", graphs["peer_cli"])
    if "peer_mcp" in arms:
        if not peer_mcp:
            print("peer_mcp arm requires --peer-mcp", file=sys.stderr)
            return 2
        print("prepare peer_mcp...")
        graphs["peer_mcp"] = prepare_peer_mcp(repo, peer_mcp)
        print("  ", graphs["peer_mcp"])

    for arm in arms:
        if arm == "so":
            arm = "superopen"
        print(f"run arm={arm}")
        t0 = time.time()
        out = out_dir / f"{arm}.json"
        if arm == "vanilla":
            results[arm] = arm_vanilla(repo, args.model, out)
        elif arm == "superopen":
            results[arm] = arm_superopen(repo, so_bin, args.model, out)
        elif arm == "peer_cli":
            results[arm] = arm_peer_cli(repo, peer_cli, args.model, out)
        elif arm == "peer_mcp":
            mcp = Path(graphs["peer_mcp"]["mcp_config"])
            results[arm] = arm_peer_mcp(repo, mcp, graphs["peer_mcp"].get("project") or "", args.model, out)
        else:
            results[arm] = {"error": f"unknown arm {arm}"}
        results[arm]["wall_s"] = round(time.time() - t0, 1)
        slim = {k: v for k, v in results[arm].items() if k != "result"}
        slim["result_excerpt"] = (results[arm].get("result") or "")[:1200]
        (out_dir / f"{arm}.metrics.json").write_text(json.dumps(slim, indent=2))
        print("  ", {k: slim.get(k) for k in ("turns", "cost_usd", "input_side_total", "error")})

    write_summary(out_dir, results, graphs)
    (out_dir / "graphs.json").write_text(json.dumps(graphs, indent=2))
    print(f"wrote {out_dir / 'summary.md'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
