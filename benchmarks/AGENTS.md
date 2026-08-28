# Benchmarks

Superopen benchmark harness. Entry point: `python3 benchmarks/run.py`.

## Critical notice

Superopen is a CLI for **any coding agent on any OS**. These modes measure that
product.

1. Headline scores are isolated **Claude Code or OpenCode** sessions (`so init` +
   `so install`). Not HTTP APIs, not pasted recall packs.
2. **No benchmark-only hacks** in `so`. Fixes must help every repo, agent, and OS.
3. This runner is **mode-gated** (`--mode`, `--scale`). Do not copy another
   product's ingest/grader into Superopen source.
4. `--scale small` (default) is a **valid** gate (LOCOMO 100 stratified, LME 50,
   compare 6, graph 12). `--scale full` is publishable. Reject toy slices.

See repo-root [BENCHMARKS.md](../BENCHMARKS.md). Every run overwrites that
file. Harness work dirs are deleted afterwards. Manual CI: `.github/workflows/bench.yml`.

## Non-negotiables

1. **Isolation.** Never write the developer OpenCode or Claude config. Product
   modes default to `--isolate docker`. Each arm gets its own container, worktree,
   `HOME`, and XDG dirs. Bind-mount the locally built `so` at `/usr/local/bin/so`.
   Never mount host `HOME`, `~/.claude`, or `~/.config/opencode`. Copy **auth
   files only** into the arm HOME. `--isolate host` is weaker (isolated HOME on
   this machine). `--mode offline` does not use Docker.
2. **Same prompt.** Identical question text across compare arms. Do not mention Superopen in the prompt.
3. **Natural product.** Superopen arm is `so init` + `so install --vendor=<host>`. No trimmed hooks.
4. **Ephemeral work dirs.** Do not keep harness JSON. The report is `BENCHMARKS.md`.
5. **Coding-agent hosts only.** `--host claude-code` or `--host opencode`.

## Modes

| Mode | Purpose |
|------|---------|
| `offline` | Harness smoke only (grading math, fixtures) — **not** a Superopen benchmark |
| `memory` | LOCOMO / LongMemEval retrieve (+ phase 3 coding-agent QA) |
| `graph` | 12 graph-tool probes on Django |
| `compare` | Native vs Superopen session (token savings headline) |
| `contradict` | Rescue@10 + historical-verbatim |
| `latency` | Hardware-local latency probe |
| `index` | `so init` wall time + graph counts |
| `temporal` | 5 Django LTS checkpoints |
| `all` | Runs the full suite |

## Scale

| `--scale` | Locomo | LongMemEval | Compare | Graph |
|-----------|--------|-------------|---------|-------|
| `small` (default) | 100 QA, all sessions ingested, category-stratified | 50 | 6 | 12 |
| `full` | 100 (same cap) | 50 | 6 | 12 |

Locomo is capped at 100.

## Quick commands

```bash
python3 benchmarks/run.py --mode offline
python3 benchmarks/run.py --mode compare --scale small --host claude-code --isolate docker --so-bin ./bin/so --max-spend 20
./benchmarks/docker-run.sh --mode compare --scale small --host claude-code --max-spend 20
```

See [README.md](README.md) and [BENCHMARKS.md](../BENCHMARKS.md).
