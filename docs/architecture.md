# Architecture

One Go binary (`cmd/so`) with the graph engine compiled in. There is no
server, no daemon, and no API key. Everything below runs locally.

## Components

| Component | Source | Role |
|-----------|--------|------|
| CLI | `cmd/so/`, `internal/cli/` | All user commands; compact text and JSON output |
| Graph engine | `internal/graph/engine/` | Tree-sitter extract to SQLite store to query/BFS/render |
| Sessions | `internal/session/`, `internal/agent/export/` | Observability spans to materialized session documents |
| Memory | `internal/memory/` | Project diary over sessions: retrieval, distill, expiry |
| Harvest | `internal/harvest/` | Proposed playbook patches, gated apply |
| Agent harness | `internal/agent/` (`install/`, `hook/`, `steer/`, `skills/`) | What `so install` writes into agent configs |
| Web UI | `web/` (served by `so dev`) | Sessions, memory, and graph visualization on localhost |

## Data flow

```text
 coding agent (Claude Code / Cursor / Codex / ...)
   |  hooks + /so skill            (user-global config)
   v
 so sessions hook --> .so/sessions/  raw observability spans
   |                      |
   |                so sessions finalize
   |                      v
   |                 session docs + maps --> so memory ingest/distill
   v                                              |
 so graph build/refresh <-- source tree          v
   |                                      .so/db/so.db (memory tables)
   v
 .so/db/so.db (graph tables) --> so graph query/search/trace
                                          ^
                    hooks steer the agent here instead of grep/read
```

## Storage map

| Location | Written by | Contents |
|----------|------------|----------|
| `<repo>/.so/sessions/` | `so init`, session hooks | Raw spans, events.jsonl, checkpoints (gitignored) |
| `<repo>/.so/db/so.db` | graph build, memory writes | One SQLite store: graph tables and memory tables (gitignored) |
| `<repo>/.so/.gitignore` | `so init` | Keeps `.so/` out of git |
| User skill/config dirs | `so install` | `/so` skill, hooks, durable guidance. Never per-repo (unless `so init --cursor-rules`). |
| Config dir (`~/.config/superopen` / `%APPDATA%\superopen`) | `so init`, `so gc` | `projects.json` cross-repo index, `config.env` |

A repo without `.so/` stays unmanaged: no hook context, no token spend there.

Details: [configuration.md](configuration.md).

## Invariants

- **Local-first**: graph builds are Tree-sitter + SQLite. They never call an
  LLM or the live agent.
- **`.so/` is the switch**: harness behavior activates only in inited repos.
- **Hooks fail open**: telemetry or steer failures never block the agent.
- **Human-approved always-on rules**: harvest never writes instruction files
  without an explicit apply.
- **Output contracts**: `so graph *` prints compact `NODE`/`EDGE`.
  `so memory` / `so sessions` print TOON lists with `help[]`.
  Hidden `so sessions hook` speaks host JSON. See [commands.md](commands.md).

## Build tags

- Default test build uses portable stubs.
- Native parsers + FTS5: `-tags tsnative,sqlite_fts5` (`make build`,
  `make test-native`). This is what release binaries ship.
