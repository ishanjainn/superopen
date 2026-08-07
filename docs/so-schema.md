# Superopen `.so/` schema (CLI ↔ UI parity)

<p align="center">
  <img src="../web/public/brand-mark.png" alt="SO shield" width="72" />
  <img src="../web/public/brand-wordmark.png" alt="SUPEROPEN" width="280" />
</p>


Canonical state lives under `.so/`. Both the Go CLI (`so`) and the TypeScript UI
(Next.js App Router) read/write these paths. Heavy algorithms use `so … --json`.

## Memory model

**Write** lessons/prefs → **Store** under `.so/memory/` → **Inject** as `active-context.md` on every SessionStart.

Session port (`so sessions port`) moves chat text between agents; after a successful port Superopen refreshes the same ACTIVE pack so the destination agent gets prefs/lessons plus continuity. Port is not a second memory system.

| Path | Role |
|------|------|
| `config.yaml` | Harness config (`memory.backend`, `evals.models`, `recommendations.auto_apply_tiers`, observability) |
| `traces/` | Local OTLP export (`observability.exporters: [{type: local_jsonl, path: .so/traces}]` only) |
| `memory/preferences.md` | Standing prefs (`# Preferences` + bullets) |
| `memory/projects.md` | Project context (`# Projects` + Current focus / Active areas / Do not touch / Notes) |
| `memory/history/YYYY-MM-DD.md` | Decaying daily history |
| `memory/lessons.jsonl` | Structured lessons (source of truth) |
| `memory/lessons.md` | Human export of lessons |
| `memory/semantic.jsonl` | Semantic key/value entries |
| `memory/episodic.jsonl` | Episodic fragments (incl. port breadcrumbs) |
| `memory/active-context.md` | Budget-capped pack injected at SessionStart (Inject) |
| `memory/last-refresh.json` | Last `so refresh` / post-merge marker |
| `memory/refresh-status.json` | Latest `so dev` watcher refresh status |
| `context/` | Feedforward docs (architecture, conventions) |
| `rules/` | Coding rules |
| `skills/` | Named skills (markdown) |
| `guardrails/guardrails.yaml` | Advisory rules + enforcement (denied commands, sensitive paths) |
| `audit/events.jsonl` | Append-only SEL audit trail |
| `graph/graph.json` | Structural graph |
| `graph/retrieve_index.json` | Harness corpus keyword index |
| `sessions/` | Materialized sessions |
| `port/ledger.json` | Session port idempotency ledger |
| `recommendations/pending.json` | Pending harness updates |
| `recommendations/history.json` | Applied / dismissed / reverted recommendations |

## Version-control policy

`.so/` is a portable team harness and should normally be committed. This keeps
the project graph, durable memory, materialized sessions, recommendations, and
agent guidance available to every contributor.

The generated `.so/.gitignore` excludes only local operational data: raw OTLP
`traces/`, audit logs, active-process state (`run/`, `session-state/`, and
`port/`), graph caches, local UI preferences, and transient memory/session
markers. Session records under `sessions/` are intentionally shared so a
teammate can inspect a session after it has been materialized. Do not commit
session content, memory, or configuration that contains secrets or information
your team is not authorized to share.

## AXI output (`so … --json` / `--full`)

Agent eXperience Interface: compact text by default, machine JSON on demand.

| Flag / env | Effect |
|---|---|
| `--json` / `SO_JSON=1` | JSON envelope on stdout (errors on stderr) |
| `--full` / `SO_FULL=1` | Do not truncate long fields |

```json
{ "ok": true, "kind": "sessions", "count": 1, "items": […], "next": ["so doctor"] }
```

Errors:

```json
{ "ok": false, "code": 1, "error": "…", "hint": "so sync" }
```

Exit codes: `0` ok · `1` fail · `2` usage · `3` not found.

Bare `so` prints a content-first status snapshot (not only help).

## Parity matrix (Phase 1+)

| Capability | CLI | UI |
|------------|-----|-----|
| Memory CRUD / search / active-context | `so memory *` (Write→Store→Inject) | `/memory` (Active Context → Lessons → Prefs → Projects) |
| Active Context inject | SessionStart when `memory.enabled` | Memory → Active Context |
| Learn lesson | `so learn add` | Memory → Lessons |
| Retrieve corpus | `so retrieve` / `so graph query` | Graph search + `/api/retrieve` |
| Knowledge | `so knowledge` / `.so/knowledge/` | `/knowledge` |
| Guardrails | `so guard` / `so guard show\|check` | `/guardrails` |
| Audit | `so audit list` | Settings → Audit Trail |
| Session harvest | `so harvest run\|idle` | (automatic on session end) |
| Session port | `so sessions detect\|port\|verify` | Sessions → Port |
| Open UI | `so open memory` | - |

## Observability (local)

Edit `.so/config.yaml` only - the UI shows values as read-only:

```yaml
observability:
  listen: http://127.0.0.1:4318
  exporters:
    - type: local_jsonl
      path: .so/traces
```

Only `local_jsonl` is supported. Other exporter types are ignored by the CLI.

## Session porting

Hub-and-spoke session porting (`internal/port/`):

```bash
so sessions detect
so sessions port --from claude --to codex --preview
so sessions port --from claude --to so --id <session-id>
so sessions verify --from claude --sample 3
```

Harness IDs: `claude`, `codex`, `opencode`, `cursor`, `pi`, `so`. Text turns only in v1
(tools/thinking dropped). Ledger: `.so/port/ledger.json`. Cursor export writes a
first-class resume pack under `.cursor/so-port/<id>/` plus hub mirror; resume with
`so sessions resume --vendor=cursor --id=…`. Pi writes `~/.pi/agent/sessions/so-port/`.
