# Benchmarks

Isolated Claude Code comparison: vanilla vs Superopen. This directory is the
Superopen **benchmarks** harness (CLI: `run_eval.py`). How to invoke a run
lives in [README.md](README.md). This file is the protocol every run must keep.

Parent index: [../../AGENTS.md](../../AGENTS.md).

## Non-negotiables

1. **Isolation.** Never write the developer `~/.claude.json`, never run
   `so install` against the real home. Each arm gets its own git worktree,
   `HOME`, `CLAUDE_CONFIG_DIR`, and XDG dirs. Copy **credential files only**
   into the arm Claude dir — not the developer’s skills or hooks.
2. **Claude Code is the eval host.** All arms run the same `claude` binary and
   the same model.
3. **Natural product, no trimming.** Superopen is `so init` +
   `so install --vendor=claude-code`. Do **not** shrink skills, drop hooks, or
   overwrite `settings.json` with a harness overlay. Vanilla is default Claude
   tools only.
4. **Same user prompt.** Identical question text across arms. Do not mention
   Superopen, graph, or memory in the prompt — skills and hooks are what should
   trip the tool.
5. **New stamp dirs.** Never overwrite `results/<stamp>/`. Pass `--out` or a
   new name.

`--skip-index` reuses an already-built graph in that worktree. It does not
authorize a reduced harness.

## Arms

| Arm | Natural install |
|-----|-----------------|
| `vanilla` | Claude defaults |
| `superopen` | `so init` + `so install --vendor=claude-code` |

## Memory matrix

Same non-negotiables. Questions: `questions/grafana-memory.json`. Arms:
`vanilla`, `superopen`.

`Memory?` counts active `so memory` tools **and** passive SessionStart hook
injection (`memory_injected` in transcript metrics). Coverage is still
substring `key_facts`. Question 2 expects prior-session recall without “use
memory” wording; the harness runs `so sessions finalize` between questions for
Superopen.

Grafana `so init` defaults to **10800s** index timeout on the memory matrix;
**7200s** on other Grafana runs; Linux stays **3600s** unless `--index-timeout`
overrides.

Tests (no live Claude, no corpus clone):

```bash
python3 benchmarks/agent-graph-eval/test_harness.py
```
