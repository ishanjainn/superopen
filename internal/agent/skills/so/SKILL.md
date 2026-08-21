---
name: so
description: "Use for any question about a codebase, its architecture, file relationships, or project content — especially when .so/ exists, where the question should be treated as a Superopen graph query first. Native code graph with query, path, and snippet tools. If .so/ is missing, do not run so or so init unless the user explicitly asks."
---

# Superopen (`/so`)

**If `test -d .so` is false, stop.** Do not run `so` and do not run `so init` unless the user explicitly asked to initialize this repo.

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

If a NODE line already names the symbol, use `so graph snippet "<qualified_name>"` for the body or `so graph trace "<qn>"` for callers/callees. Do not start a `so graph search` spray after a truncated query — raise `--budget` or narrow the question instead.

Do not initialize a repository because `.so/` is missing.

Recipes (dead code, fan-in/out, routes): `references/query.md`. Prior-work memory: `references/memory.md`.
