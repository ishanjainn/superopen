# Benchmarks

Isolated comparison of Claude Code `haiku` with and without a code graph.
Protocol (isolation, Claude Code, natural product — no trimming): [AGENTS.md](AGENTS.md).
Default corpus is **torvalds/linux** (CBM publishes a ~3 minute full index of
the Linux kernel). `grafana` remains available via `--repo grafana`.

All arms receive the **same prompt text**. They differ only by prepared tooling.
Each arm uses that product's **real install command** (what a developer would
run). There is no reduced / always-on harness overlay.

Superopen prepare is `so init` (same argv as `/so init` on a fresh tree), timed
separately from agent USD so it can be compared to CBM `index_repository`.

## Isolation

Each arm gets a separate environment. The harness never writes the developer
`~/.claude.json` and never runs `so install` / `graphify install` / CBM `install`
/ `iai-mcp capture-hooks install` against the real home directory.

| Resource | Isolation |
|---|---|
| Git tree | `git worktree` from one shallow mirror under `cache/<linux\|grafana>` |
| Claude config | `CLAUDE_CONFIG_DIR=$arm/.claude` (auth files only; product install is copied from isolated HOME) |
| XDG / caches | `HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME` under `$arm/home` |
| Superopen | `SUPEROPEN_INSTALL_DIR` + `so init` in that worktree; `so install --vendor=claude-code` into isolated HOME; no `--mcp-config` |
| CBM | Arm-isolated `CBM_CACHE_DIR` under that arm's HOME. Docker indexes with the Linux CBM binary. Host daemon cache is never mounted. Product `install --yes --clients=claude` into isolated HOME. |
| Graphify | `graphify-out/` plus `graphify install` and `graphify claude install` in that arm |
| iai | `IAI_MCP_STORE=$arm/home/.iai-mcp`; empty store; MCP via `--mcp-config` only |

`--mcp-config` is passed **only** for CBM / iai. Superopen and Graphify are CLI. Vanilla gets
no MCP file.

Auth: prefer `ANTHROPIC_API_KEY`. If the machine only has a Claude subscription
login, the harness copies **credential files only** (not `mcpServers`, skills, or
hooks) into each `CLAUDE_CONFIG_DIR`.

Index runs **before** the timed agent questions. Index stdout is streamed live.
Index wall time, node/edge counts, and ingest USD (expected $0 for Superopen /
Graphify `--code-only` / CBM) are reported separately from agent USD.

## Arms

| Arm | What it is |
|---|---|
| `vanilla` | Default Claude tools only |
| `superopen` | `so init` + `so install --vendor=claude-code` (skill, durable CLAUDE.md, product hooks). No MCP. |
| `graphify` | `graphify extract . --code-only --force` then `graphify install` + `graphify claude install` |
| `cbm` | `codebase-memory-mcp cli index_repository` plus product `install --yes --clients=claude` |
| `iai` | MCP against an empty store (`memory-not-graph`, not a code-graph competitor) |

Aliases: `so` → `superopen`, `peer_cli` → `graphify`, `peer_mcp` → `cbm`.

`prepare.json` records `harness: product`. Do not overwrite older result dirs
such as `grafana-2task-harness/`; use a new stamp or `--out …/grafana-2task-product`.

## Questions and grading

`questions/linux.json` (default) and `questions/grafana.json` each have six
architecture prompts with `key_facts` aliases. Coverage is deterministic
substring matching (covered / partial / miss). The prompt is the question
only — identical across arms. Do not mention Superopen, graph, or memory in
the user prompt; skills and hooks are what should trip graph/memory.

For coding-agent effectiveness (complete the change, cheaper), use
`questions/grafana-2task.json`: two small sequential tasks on the same
worktree. Session 2 is a follow-up with no “use memory” wording.
`Task?` is a worktree/diff check. `Memory?` is transcript `so memory` / memory tools.

## Run

Peer binaries are not vendored; missing bins error only if that arm is selected.
First run shallow-clones the corpus into gitignored `cache/`; later runs reuse
the mirror SHA (pinned in `results/<stamp>/summary.md`).

```bash
go build -o /tmp/so ./cmd/so
python3 scripts/agent-graph-eval/run_eval.py \
  --repo linux \
  --so-bin /tmp/so \
  --graphify-bin "$(command -v graphify)" \
  --cbm-bin "$(command -v codebase-memory-mcp)" \
  --iai-mcp /Users/ishanjain/work/iai-personal-memory-engine/plugin/bin/iai-pme-mcp \
  --model haiku \
  --docker \
  --arms vanilla,superopen,graphify,cbm,iai
```

Grafana instead of the kernel:

```bash
python3 scripts/agent-graph-eval/run_eval.py --repo grafana --so-bin /tmp/so --arms vanilla,superopen
```

Reuse an already-indexed worktree (skip `so init`):

```bash
python3 scripts/agent-graph-eval/run_eval.py \
  --repo grafana --arms superopen --so-bin /tmp/so --model haiku \
  --work /tmp/so-eval-superopen \
  --out scripts/agent-graph-eval/results/grafana-2task-product \
  --skip-index
```

Timeouts default to 60 minutes for index and 15 minutes per question. Override
with `--index-timeout` / `--agent-timeout`.

## Tests

```bash
python3 scripts/agent-graph-eval/test_harness.py
```

No live Claude and no kernel/grafana clone.
