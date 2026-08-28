---
name: so
description: "Use for any question about a codebase, its architecture, file relationships, or project content — especially when .so/ exists, where the question should be treated as a Superopen graph query first. Also use for prior-work or personal facts saved in this workspace (so memory recall). Superopen is a CLI binary invoked with Bash, not an MCP tool. If .so/ is missing, do not run so or so init unless the user explicitly asks."
---

# Superopen (`/so`)

**If the `.so` directory is missing, stop.** Do not run `so` and do not run `so init` unless the user explicitly asked to initialize this repo.

Superopen is a **CLI binary**, not an MCP tool and not a host plugin tool. There is no `so` tool schema. Invoke it with **Bash** using this install-time absolute path:

```bash
__SO_BIN__
```

If that path is missing, fall back to `$SUPEROPEN_SO_BIN` or `so` on `PATH`.

Never Grep `.so/` or installed skill/rule directories. If you spawn a Task/subagent, its prompt must say to run the same Bash line first (Explore children never see SessionStart).

## Fast path — this repo's code

When `.so/` exists and the request is about **this repository's source** (how does X work, where is Y, callers, files, architecture):

```bash
__SO_BIN__ graph query "<question>"
```

Answer from NODE/EDGE lines, their `src=` paths, and any BODIES the query already printed. **If those answer the question, stop.** Do not Read whole modules. Do not list the tree to confirm Superopen.

If you still need a symbol body, run `__SO_BIN__ graph snippet "<qualified_name>"` for a NODE already listed. Do not run `so graph query` again unless the dump said TRUNCATED. Grep/Read only for a literal string the graph does not index. `__SO_BIN__ graph trace "<qn>"` for callers if the listed NODE is not enough. Do not start a `so graph search` spray.

## Fast path — prior work / personal / saved memories

When the question is personal, about prior decisions, or this workspace is a memory store (little/no application source):

```bash
__SO_BIN__ memory recall "<question>"
```

That command returns bodies. `so memory search` is a title-only index — `0 memories` there means no title match, not an empty store. Then `so memory get <id>` if you need one row. Do not run graph query for a diary question.

Do not initialize a repository because `.so/` is missing.

Recipes (dead code, fan-in/out, routes): `references/query.md`. Prior-work memory: `references/memory.md`. Playbook harvest (on demand): `references/harvest.md`.
