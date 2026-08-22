# Benchmarks

Isolated Claude Code comparison of coding-agent graph products. This directory
is the Superopen **benchmarks** harness (CLI: `run_eval.py`). How to invoke a
run lives in [README.md](README.md). This file is the protocol every run must
keep.

Parent index: [../AGENTS.md](../AGENTS.md).

## Non-negotiables

1. **Isolation.** Never write the developer `~/.claude.json`, never run
   `so install` / `graphify install` / CBM `install` against the real home.
   Each arm gets its own git worktree, `HOME`, `CLAUDE_CONFIG_DIR`, and XDG
   dirs. Copy **credential files only** into the arm Claude dir — not the
   developer’s `mcpServers`, skills, or hooks.
2. **Claude Code is the eval host.** All arms run the same `claude` binary and
   the same model. Other vendors are product-install targets, not this matrix.
3. **Natural product, no trimming.** Each arm is that product’s real install
   command (`so init` + `so install --vendor=claude-code`, `graphify extract`
   + `graphify install` + `graphify claude install`, CBM `index_repository` +
   `install --yes --clients=claude`). Do **not** shrink skills, drop hooks,
   strip MCP, overwrite `settings.json` with a PreToolUse-only overlay, or
   otherwise “fairness-edit” a competitor. Vanilla is default Claude tools
   only. Superopen and Graphify are CLI (no `--mcp-config`). CBM / iai get
   MCP because that is how those products work.
4. **Same user prompt.** Identical question text across arms. Do not mention
   Superopen, Graphify, CBM, graph, or memory in the prompt — skills and hooks
   are what should trip the tool.
5. **New stamp dirs.** Never overwrite `results/<stamp>/`. Pass `--out` or a
   new name.

`--skip-index` reuses an already-built graph in that worktree. It does not
authorize a reduced harness.

## Arms

| Arm | Natural install |
|-----|-----------------|
| `vanilla` | Claude defaults |
| `superopen` | `so init` + `so install --vendor=claude-code` |
| `graphify` | `graphify extract . --code-only --force` + product install |
| `cbm` | `codebase-memory-mcp cli index_repository` + product install |
| `iai` | MCP against an empty store (memory, not a code-graph competitor) |

## What this matrix is not

- Not OpenCode / Pi / Codex / Cursor / Gemini / Copilot. Those get the same
  product contract in `so install`; they are not Claude-hosted evals.
- Not a reason to pad `internal/agent/skills/so/SKILL.md` to match Graphify’s
  slash-command pipeline or CBM’s MCP encyclopedia.

Tests (no live Claude, no corpus clone):

```bash
python3 scripts/agent-graph-eval/test_harness.py
```
