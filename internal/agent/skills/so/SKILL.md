---
name: so
description: "Use for any question about a codebase, its architecture, file relationships, or project content — especially when .so/ exists, where the question should be treated as a Superopen graph query first. Native code graph with query, path, and snippet tools. If .so/ is missing, do not run so or so init unless the user explicitly asks."
---

# Superopen (`/so`)

**If the `.so` directory is missing, stop.** Do not run `so` and do not run `so init` unless the user explicitly asked to initialize this repo.

Never Grep `.so/` or installed skill/rule directories to find the graph. When `.so/` exists and the request is about the codebase, run `so graph query` before grepping the repository. If you spawn a Task/subagent, its prompt must say to run `so graph query` first (Explore children never see SessionStart).

## Binary

Prefer this absolute binary (set at `so install` time):

```text
__SO_BIN__
```

If that path is missing, fall back to `$SUPEROPEN_SO_BIN` or `so` on `PATH`.

## Fast path — existing graph

When `.so/` exists and the request is about the codebase (how does X work, where is Y, callers, files, architecture — not an explicit rebuild): **run `so graph query "<question>"` immediately.** Do not detect. Do not spawn Explore/Agent. Do not grep first. The graph is already built — use it.

```bash
__SO_BIN__ graph query "<question>"
```

Answer from NODE/EDGE lines and their `src=` paths. Read those files to edit or debug specific lines. Grep only after query has oriented you, or for a literal string the graph does not index.

If a NODE line already names the symbol, use `so graph snippet "<qualified_name>"` for the body or `so graph trace "<qn>"` for callers/callees. After a truncated query, run `so graph snippet "<qn>"` from a NODE above or narrow the question — do not start a `so graph search` spray.

Do not initialize a repository because `.so/` is missing.

Recipes (dead code, fan-in/out, routes): `references/query.md`. Prior-work memory: `references/memory.md`. Playbook harvest (on demand): `references/harvest.md`.
