# Benchmarks

Isolated comparison of Claude Code `haiku` with and without Superopen.
Protocol (isolation, Claude Code, natural product — no trimming): [AGENTS.md](AGENTS.md).
Default corpus is **torvalds/linux**. `grafana` is available via `--repo grafana`.

All arms receive the **same prompt text**. They differ only by prepared tooling.
Each arm uses that product's **real install command** (what a developer would
run). There is no reduced / always-on harness overlay.

Superopen prepare is `so init` (same argv as `/so init` on a fresh tree), timed
separately from agent USD.

## Isolation

Each arm gets a separate environment. The harness never writes the developer
`~/.claude.json` and never runs `so install` against the real home directory.

| Resource | Isolation |
|---|---|
| Git tree | `git worktree` from one shallow mirror under `cache/<linux\|grafana>` |
| Claude config | `CLAUDE_CONFIG_DIR=$arm/.claude` (auth files only; product install is copied from isolated HOME) |
| XDG / caches | `HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME` under `$arm/home` |
| Superopen | `SUPEROPEN_INSTALL_DIR` + `so init` in that worktree; `so install --vendor=claude-code` into isolated HOME |

Auth: prefer `ANTHROPIC_API_KEY`. If the machine only has a Claude subscription
login, the harness copies **credential files only** (not skills or hooks) into
each `CLAUDE_CONFIG_DIR`.

Index runs **before** the timed agent questions. Index stdout is streamed live.
Index wall time and node/edge counts are reported separately from agent USD.

## Arms

| Arm | What it is |
|---|---|
| `vanilla` | Default Claude tools only |
| `superopen` | `so init` + `so install --vendor=claude-code` (skill, durable CLAUDE.md, product hooks) |

Alias: `so` → `superopen`.

`prepare.json` records `harness: product`. Use a new stamp or `--out` for each run.

## Questions and grading

`questions/linux.json` (default) and `questions/grafana.json` each have six
architecture prompts with `key_facts` aliases. Coverage is deterministic
substring matching (covered / partial / miss). The prompt is the question
only — identical across arms. Do not mention Superopen, graph, or memory in
the user prompt; skills and hooks are what should trip graph/memory.

For coding-agent effectiveness (complete the change, cheaper), use
`questions/grafana-2task.json`: two small sequential tasks on the same
worktree. Session 2 is a follow-up with no “use memory” wording.
`Task?` is a worktree/diff check. `Memory?` is transcript `so memory` usage
or passive SessionStart injection.

## Memory matrix

Same protocol on one Grafana worktree (`questions/grafana-memory.json`):

1. Seed a small change (session diary).
2. Recall that change in a new Claude session — no “use memory” wording.
3. Ask a current-code architecture question.

Arms: `vanilla` and `superopen`. Superopen Grafana index timeout defaults to
**3h** on this matrix (`10800s`); override with `--index-timeout`.

Retrieval microbenches (no Claude, no corpus clone):

```bash
go test -bench=. -benchmem ./internal/memory/
```

## Run

First run shallow-clones the corpus into gitignored `cache/`; later runs reuse
the mirror SHA (pinned in `results/<stamp>/summary.md`).

```bash
go build -o /tmp/so ./cmd/so
python3 benchmarks/agent-graph-eval/run_eval.py \
  --repo linux \
  --so-bin /tmp/so \
  --model haiku \
  --docker \
  --arms vanilla,superopen
```

Grafana:

```bash
python3 benchmarks/agent-graph-eval/run_eval.py --repo grafana --so-bin /tmp/so --arms vanilla,superopen
```

Memory matrix:

```bash
python3 benchmarks/agent-graph-eval/run_eval.py \
  --repo grafana \
  --questions benchmarks/agent-graph-eval/questions/grafana-memory.json \
  --so-bin /tmp/so \
  --model haiku --docker \
  --arms vanilla,superopen
```

Reuse an already-indexed worktree (skip `so init`):

```bash
python3 benchmarks/agent-graph-eval/run_eval.py \
  --repo grafana --arms superopen --so-bin /tmp/so --model haiku \
  --work /tmp/so-eval-superopen \
  --out benchmarks/agent-graph-eval/results/grafana-memory \
  --skip-index
```

Timeouts default to 60 minutes for index and 15 minutes per question. Override
with `--index-timeout` / `--agent-timeout`.

## Tests

```bash
python3 benchmarks/agent-graph-eval/test_harness.py
```

No live Claude and no kernel/grafana clone.
