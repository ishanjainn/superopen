# Superopen

One CLI to rule them all.

## Install (user-global, any directory)

Works on **Linux, macOS, and Windows** across supported coding agents
(Claude Code, Cursor, Codex, Gemini CLI, OpenCode, Copilot CLI, Pi).

```bash
brew install ishanjainn/superopen/so   # or release installer / go install
so install                            # /so skill + hooks + guidance + MCP
```

`so install` writes into each agent’s **user** skill/plugin/config directories
(OS-agnostic home / XDG / `%APPDATA%` / `%LOCALAPPDATA%`). It is not tied to
the current repository. It installs:

- the `/so` skill
- observability hooks
- durable graph-first guidance (user-level instruction surfaces)
- user-global MCP entries (repo-neutral; agents spawn `so graph mcp serve`)

## Initialize a repository

In a coding agent (after `so install`), or from a shell inside the repo:

```bash
so init          # or /so init in the agent
```

Defaults to the **repository root** (nearest existing `.so` or git top-level).
Use `--root` / `SUPEROPEN_ROOT` for an explicit nested package graph.

Creates:

```text
.so/
  sessions/      # observability sessions (gitignored)
  db/so.db       # shared Superopen SQLite store (gitignored)
  .gitignore
```

Registers the repo in the user-wide project index under the Superopen config
dir (`~/.config/superopen` / `%APPDATA%\superopen`).

## Native graph (automatic for agents)

After install + init, coding agents are steered to use the graph for structural
questions without the user saying `/so`.

```bash
so graph build
so graph refresh              # skip when unchanged; --force for full rebuild
so graph search DataFlowingGate
so graph query "How does DataFlowingGate gate the UI?"
so graph architecture
so graph impact DataFlowingGate
```

Session hooks refresh the graph in the background on SessionStart / SessionEnd
(detached, fail-open). Builds are **local** (Tree-sitter + SQLite) — they do not
invoke an LLM or the live coding agent.

MCP is wired by `so install` (and ensured by `so init` / `so dev`). Diagnostic only:

```bash
so graph mcp config           # print user-global snippet
so graph mcp serve            # stdio MCP; root from cwd / SUPEROPEN_ROOT / FindRoot
```

## Sessions and UI

```bash
so sessions list
so sessions show <id>
so sessions finalize <id>
so projects                   # repos where Superopen has been used
so dev                        # Sessions UI + live graph refresh (~60s poll)
so dev -d                     # detached UI
```

`so dev` is repo-neutral for MCP: it ensures the same user-global MCP entries
and does not write project MCP files into the working tree.

## Layout summary

| Location | Purpose |
|----------|---------|
| User skill dirs | `/so` skill from `so install` |
| User instruction surfaces | Graph-first durable guidance |
| User MCP configs | `superopen-graph` → `so graph mcp serve` |
| `<repo>/.so/sessions` | Session documents |
| `<repo>/.so/db/so.db` | Shared DB (graph today; more features later) |
| Config dir `projects.json` | Cross-repo index of Superopen usage |

One `so` binary includes the native graph engine. There is no separate graph binary.
