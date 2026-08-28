# Architecture

One Go binary (`cmd/so`) with the graph engine compiled in. There is no server,
no daemon, no API key. Everything below runs locally.

## Components

| Component | Source | Role |
|-----------|--------|------|
| CLI | `cmd/so/`, `internal/cli/` | All user commands; AXI output conventions |
| Graph engine | `internal/graph/engine/` | Tree-sitter extract → SQLite store → query/BFS/render |
| Sessions | `internal/session/`, `internal/agent/export/` | Observability spans → materialized session documents |
| Memory | `internal/memory/` | Project diary over sessions; retrieval, distill, expiry |
| Harvest | `internal/harvest/` | Proposed playbook patches, gated apply |
| Agent harness | `internal/agent/` (`install/`, `hook/`, `steer/`, `skills/`) | What `so install` writes into agent configs |
| Web UI | `web/` (served by `so dev`) | Sessions/memory/graph visualization over a local BFF |

## Data flow

```text
 coding agent (Claude Code / Cursor / Codex / …)
   │  hooks + /so skill            (user-global config)
   ▼
 so sessions hook ──► .so/sessions/  raw observability spans
   │                      │
   │                so sessions finalize
   │                      ▼
   │                 session docs + maps ──► so memory ingest/distill
   ▼                                              │
 so graph build/refresh ◄── source tree          ▼
   │                                      .so/db/so.db (memory tables)
   ▼
 .so/db/so.db (graph tables) ──► so graph query/search/trace
                                          ▲
                    hooks steer the agent here instead of grep/read
```

## Storage map

| Location | Written by | Contents |
|----------|------------|----------|
| `<repo>/.so/sessions/` | `so init`, session hooks | Raw spans, events.jsonl, checkpoints (gitignored) |
| `<repo>/.so/db/so.db` | graph build, memory writes | One SQLite store: graph tables **and** memory tables (gitignored) |
| `<repo>/.so/.gitignore` | `so init` | Keeps `.so/` out of git |
| User skill/config dirs | `so install` | `/so` skill, hooks, durable guidance — never per-repo |
| Config dir (`~/.config/superopen` / `%APPDATA%\superopen`) | `so init` | `projects.json` cross-repo index |

A repo without `.so/` stays unmanaged: no hook context, no token spend there.

## Invariants

- **Local-first**: graph builds are Tree-sitter + SQLite; they never call an
  LLM or the live agent.
- **`.so/` is the switch**: harness behavior activates only in inited repos.
- **Hooks fail open**: telemetry or steer failures never block the agent.
- **Human-approved always-on rules**: harvest never writes instruction files
  without an explicit apply.
- **Output contracts**: `so graph *` prints compact `NODE`/`EDGE`;
  `so memory` / `so sessions` print axi.md TOON lists with `help[]`;
  hidden `so sessions hook` speaks host JSON. See [commands.md](commands.md).

## Build tags

- Default test build uses portable stubs.
- Native parsers + FTS5: `-tags tsnative,sqlite_fts5` (`make build`,
  `make test-native`). This is what release binaries ship.
