# Memory

Prior-work memory for managed repos (`.so/sessions/` + tables in `.so/db/so.db`).

Parent index: [../../AGENTS.md](../../AGENTS.md). UI: [../../web/AGENTS.md](../../web/AGENTS.md).

## Package layout

| Path | Role |
|------|------|
| `store.go`, `migrate.go` | SQLite persistence |
| `search.go`, `recall.go`, `rank.go` | Retrieval |
| `capture.go`, `distill.go`, `teach.go` | Rollups |
| `pack.go`, `ingest.go` | Hook session packs |
| `embed.go`, `embedworker.go` | Optional embeddings |
| `crypto.go`, `privacy.go`, `shield.go` | Sensitivity |

CLI: `cmd/so/memory_cmd.go`

## Product contract

- End-user memory docs: `internal/agent/skills/so/references/memory.md` only — not skill description.
- Hooks: one-shot memory pack on prompt/compact — `internal/agent/hook/steer_context.go`.
- Memory is hints, not authority.

## Tests

```bash
go test ./internal/memory/ -count=1
```

## Change checklist

- [ ] Schema/migration if tables change.
- [ ] `memory_cmd.go` matches `internal/memory` API.
- [ ] Hook packs fail-open.
- [ ] Privacy/shield reviewed for new user content fields.
- [ ] UI routes updated if exposed on web.

Rules: [.agents/rules/memory.mdc](../../.agents/rules/memory.mdc)
