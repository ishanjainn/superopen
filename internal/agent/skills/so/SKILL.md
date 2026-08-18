---
name: so
description: "Superopen native code graph and coding-session observability. Use automatically for structural codebase questions: architecture, callers/callees, where X is defined, dependencies, impact analysis, how does Y work, explore the codebase, find symbols, trace call chains. Also use when initializing a repo or inspecting agent sessions."
---

# Superopen (`/so`)

Superopen provides a native repository code graph and observability-derived coding sessions.

**Use the graph automatically** for structural questions — do not wait for the user to say `/so`.

## Binary

Prefer this absolute binary (set at `so install` time):

```text
__SO_BIN__
```

If that path is missing, fall back to `$SUPEROPEN_SO_BIN` or `so` on `PATH`.

## Prerequisites

1. Run **`so install` once per machine** (from any directory) to install this skill, observability hooks, durable graph-first guidance, and user-global MCP.
2. In a repository, run **`/so init`** (or `__SO_BIN__ init`) to create `.so/` and build the graph.
3. `so init` defaults to the **repository root** (nearest existing `.so` or git top-level). Use `--root` / `SUPEROPEN_ROOT` only for an explicit nested package graph.

## Graph playbook (prefer graph over Read)

Do **not** Read/Grep/Glob source until graph/snippet context is insufficient.

| Intent | CLI | MCP (when connected) |
|--------|-----|----------------------|
| Architecture overview | `__SO_BIN__ graph architecture` | `graph_architecture` |
| Find a symbol | `__SO_BIN__ graph search <name>` | `graph_search` |
| Call / config chain | `__SO_BIN__ graph trace <qn> --direction outgoing` | `graph_trace` |
| Function body | `__SO_BIN__ graph snippet <qn>` | `graph_snippet` |
| Natural-language question | `__SO_BIN__ graph query "..."` | `graph_query` |

Suggested loop: **search → trace → snippet** (or one `graph query`, which already includes top-seed snippets).

## Other commands

| Intent | Command |
|--------|---------|
| Initialize this repo | `__SO_BIN__ init` |
| Rebuild / refresh graph | `__SO_BIN__ graph build` or `__SO_BIN__ graph refresh` |
| Force full rebuild | `__SO_BIN__ graph build --force` |
| Session list | `__SO_BIN__ sessions list` |
| Local UI + live watcher | `__SO_BIN__ dev` or `__SO_BIN__ dev -d` |
| Print MCP config (diagnostic) | `__SO_BIN__ graph mcp config` |

## MCP

`so install` / `so init` / `so dev` ensure **user-global** MCP entries (repo-neutral; no project files). Agents spawn `__SO_BIN__ graph mcp serve`, which resolves the active repo from cwd. Prefer MCP tools over shelling out when connected. Skills-first CLI works without MCP.

## Layout

```text
.so/
  sessions/   # observability sessions (gitignored)
  db/so.db    # shared Superopen SQLite store (gitignored)
```

Optional path excludes: `.soignore` at the repository root (gitignore-style patterns).

A user-wide project index lives under the Superopen config dir (OS-agnostic: XDG / `%APPDATA%\superopen`).

## Notes

- Durable graph-first guidance is installed into each agent’s **user-level** instruction surface (sentinel-managed; uninstall removes only Superopen’s block).
- Graph refresh also runs detached on SessionStart / SessionEnd via observability hooks.
- Live poll while `so dev` / MCP is up defaults to **60 seconds**. Builds are local Tree-sitter + SQLite — they do **not** call an LLM or the live coding agent.
- Optional LLM API keys are not required for the AST graph.
- `so init` does **not** start `so dev` unless `--dev` is passed.
