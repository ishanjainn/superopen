---
name: so
description: "Use for any question about a codebase, its architecture, file relationships, or project content — treat source questions as graph query first. Also use for prior-work facts (so memory recall) and when the user wants a fact stored (so memory capture). Superopen is a CLI binary invoked with Bash, not an MCP tool. If .so/ is missing, run one graph query anyway (a linked worktree may seed from the parent). If that prints the unmanaged message, stop and do not so init unless the user explicitly asked."
---

# Superopen (`/so`)

Superopen is a **CLI binary**, not an MCP tool and not a host plugin tool. There is no `so` tool schema. Invoke it with **Bash** using this install-time absolute path:

```bash
__SO_BIN__
```

If that path is missing, fall back to `$SUPEROPEN_SO_BIN` or `so` on `PATH`.

Never Grep `.so/` or installed skill/rule directories. Never pipe `so` through `head`/`tail`. If you spawn a Task/subagent, its prompt must say to run the same Bash line first (Explore children never see SessionStart).

If `.so/` is missing, still run **one** `graph query`. If stdout is `not a Superopen repo; run so init`, stop. Do not `so init` unless the user explicitly asked. A linked git worktree of a managed parent seeds on that call.

## Scenario table

| Situation | Command | Then |
|-----------|---------|------|
| Unknown location / how does X work | `__SO_BIN__ graph query "<question>"` | Stop if NODE/EDGE/BODIES answer |
| Known symbol, need body | `__SO_BIN__ graph snippet "<qn>"` | Only a NODE already listed |
| Callers | `__SO_BIN__ graph trace "<qn>"` | `--direction incoming` when you need callers |
| Rename / signature / multi-file edit | `__SO_BIN__ graph impact --files <path>` or `--base main` | Check siblings before you finish; do not skip dependents |
| Cold repo / architecture | `__SO_BIN__ graph architecture` | Stop; do not dump every hub |
| Prior work / diary | `__SO_BIN__ memory recall "<question>"` | Quote `#id`; not graph query |
| Persist a fact for later | `__SO_BIN__ memory capture --kind knowledge --horizon medium --title "…" --text "…"` | Intent, not a phrase; then stop |
| Exhaustive literal | `__SO_BIN__ graph code-search "<pat>"` | Grep only if that file is unindexed |

One call then act. Do not re-query unless the dump said TRUNCATED. Do not Read a whole file whose body is already in BODIES. Do not list the tree to confirm Superopen.

If a hook line says `HARVEST pending`, you are the live agent: run `__SO_BIN__ harvest brief <id>` then `__SO_BIN__ harvest propose` (JSON on stdin, include `session_id`) **before answering**. If nothing to propose, run `__SO_BIN__ harvest skip <id>`. Same for `DISTILL pending` (`memory distill --brief <id>`, then `memory distill --apply <id>`; empty array if nothing durable). See `references/harvest.md`.

Recipes (dead code, fan-in/out, routes): `references/query.md`. Prior-work memory: `references/memory.md`. Playbook harvest (on demand): `references/harvest.md`.
