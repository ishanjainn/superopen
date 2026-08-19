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
| Natural-language / how does X work | `__SO_BIN__ graph query "..."` | `graph_query` |
| Find a symbol | `__SO_BIN__ graph search <name>` | `graph_search` |
| Function body | `__SO_BIN__ graph snippet <qn>` | `graph_snippet` |
| Callers / callees (slim) | `__SO_BIN__ graph trace <qn> --direction outgoing` | `graph_trace` |
| Architecture overview | `__SO_BIN__ graph architecture` | `graph_architecture` |

**Stop-early loop (prefer fewer turns):**
1. Run **one** `graph query` with the full question.
2. If the answer is already clear from NODE/EDGE lines and any included snippets, **stop**.
3. Otherwise `graph search` for the exact symbol, then `graph snippet <qualified_name>`.
4. Use `graph trace` only when you need callers/callees; prefer a fully qualified name (short names may return an ambiguous suggestion list).
5. Read source files only if graph/snippet context is still insufficient.

Do **not** run long multi-step graph checklists after a good query/snippet already answers the question.

## Graph vocabulary

Node labels: `Function`, `Method`, `Class`, `Struct`, `Interface`, `Type`, `Variable`, `EnvVar`,
`Decorator`, `Route`, `Module`, `Package`, `File`, `Folder`, `Section`, `Branch`, `Project`.

Edge types: `CALLS`, `CALL_REFERENCE`, `DEFINES`, `DEFINES_METHOD`, `IMPORTS`, `DEPENDS_ON`,
`IMPLEMENTS`, `OVERRIDE`, `USAGE`, `WRITES`, `RAISES`, `CONFIGURES`, `DECORATES`, `HTTP_CALLS`,
`TESTS`, `TESTS_FILE`, `CONTAINS_FILE`, `CONTAINS_FOLDER`, `FILE_CHANGES_WITH`, `HAS_BRANCH`,
`SIMILAR_TO`, `SEMANTICALLY_RELATED`.

Run `__SO_BIN__ graph schema` for this repository's live counts and edge properties.
For query recipes (dead code, fan-in/fan-out, routes, config keys, impact), load
`references/query.md` — do not paste it in unless the task needs it.

## Delegate to a subagent for multi-step graph work

`so install` ships three subagents. Delegating keeps the exploration turns and their tokens
out of the parent conversation:

| Agent | Use when |
|-------|----------|
| `so-scout` | Fast provisional lookup — where is X, what calls Y. No absence claims. |
| `so-verify` | Default. Task-directed evidence with snippets for anything you will act on. |
| `so-auditor` | Bounded-scope exhaustive claims (dead code, complete impact). Needs a scope. |

## Gotchas

1. `graph trace` needs an exact qualified name — a short name returns an ambiguous
   suggestion list and costs a turn. `graph search` first, then trace the qualified name.
2. `graph query` output is truncated to a budget. A `TRUNCATED` banner means narrow the
   question or move to `graph search` + `graph snippet`, not that the graph is incomplete.
3. Common short terms (`init`, `run`, `New`) match broadly. Add the package or type
   (`Openlit.init`, `engine.Query`) to get a usable ranking.
4. `--direction outgoing` shows callees only; callers need `incoming`, and cross-package
   answers usually need `both`.
5. Search covers indexed symbols. Literal strings, comments, and unindexed config need
   `code_search`/Grep — reach for those only after graph search comes back empty.
6. Compact text is the default view; `--json` (CLI) returns full fidelity including
   coverage and edge properties when you actually need them.

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
