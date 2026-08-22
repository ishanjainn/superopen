# Memory

Prior-work memory for managed repos (`.so/sessions/` + tables in `.so/db/so.db`).

Parent index: [../../AGENTS.md](../../AGENTS.md). UI: [../../web/AGENTS.md](../../web/AGENTS.md).

## Package layout

| Path | Role |
|------|------|
| `store.go` | SQLite persistence (schema_version=1, no ALTER migrate) |
| `search.go`, `recall.go`, `rank.go` | Retrieval and ranking |
| `capture.go`, `distill.go`, `teach.go`, `observer.go` | Rollups + typed observations |
| `pack.go`, `ingest.go`, `format.go` | SessionStart index, ingest, AXI lines |
| `embed.go`, `embedworker.go` | Optional embeddings; hashed fallback |
| `crypto.go`, `privacy.go`, `shield.go` | Sensitivity |

CLI: `cmd/so/memory_cmd.go`

## Product contract

- End-user memory docs: `internal/agent/skills/so/references/memory.md` only — not skill description. `SKILL.md` stays a one-line pointer so `/so` does not spend graph turns inside a memory encyclopedia.
- Graph-owned channels (do not put memory playbooks here): `Block()`, `CursorRule()`, `SKILL.md` body, PreToolUse `graphGate` / `SearchNudge` / `ReadNudge`, Codex PreToolUse (must stay empty), UserPromptSubmit / PostToolUse / Stop. File Read Gate on Read is out — graph owns Read; file memory is `so memory search --file`.
- Age retention (`so gc`, Settings) deletes unpinned prompts/session rollups older than `SUPEROPEN_MEMORY_RETENTION_HOURS` (default 168). Teachings, pins, and `never_decay` are never deleted by age. Session transcripts use `SUPEROPEN_SESSION_RETENTION_HOURS`. This is an explicit user setting, not ranking.
- SessionStart may emit a ≤350 token ID-index that **starts with a graph-first line**. Empty episode store falls back to recent `session.json` titles. Pending distill appends `LiveDistillInstruction`. Fail-open.
- UserPromptSubmit / beforeSubmitPrompt stay silent unless the prompt matches a prior-work cue (`last time`, `we decided`, `remember`, `what did we`); then inject index lines only (no bodies).
- Cursor `preCompact` stays a ≤200 token working snapshot with no search CTA. SubagentStart stays the graph reminder only.
- Memory is hints, not authority. Ranking quality lives in `so memory search` / `so memory recall`; injection channels stay graph-first so agents still run `so graph query`.

Third-party algorithm attribution: see repository `NOTICE`.

## Memory contracts (M)

| ID | Contract |
|----|----------|
| **M1** | Ingest writes user prompts verbatim (`KindPrompt`) and always writes `KindWorking`. No LLM on this path. Skip fenced transcript dumps (trimmed text starting with ` ``` `) and compact graph dumps (three or more `NODE`/`EDGE` lines) so pasted traces are not stored as prompts. |
| **M2** | Tool spans become observations `{tool, path, state}` without storing results. Same `coding_agent.file_path` stamp as sessions. `memory search --file` hits those rows. |
| **M3** | Local rollup always sets `request:` from the prompt and omits `learned:` unless a typed observation exists. Never invent design facts. Headless distill is opt-in (`so memory distill`), never auto-run on finalize. Interrogative prompts stay `KindPrompt` only — they are not typed as `decision`. Search downranks `learned:` lines unless the row is an explicit agent/teach capture. |
| **M4** | Index lines use `~Nt` (tokens), never bare `~N` that looks like a source line. |

## Tests

```bash
go test ./internal/memory/ ./cmd/so/ ./internal/agent/hook/ ./internal/agent/steer/ ./internal/agent/skills/ -count=1
go test -bench=. -benchmem ./internal/memory/
python3 benchmarks/agent-graph-eval/test_harness.py
```

## Change checklist

- [ ] Schema lives in `store.go` DDL only (no ALTER paths).
- [ ] `memory_cmd.go` matches `internal/memory` API + AXI `help[]`.
- [ ] Hook packs fail-open; PreToolUse stays graph-only.
- [ ] Privacy/shield reviewed for new user content fields.
- [ ] `TestPublishPreservesMemory` still passes.
- [ ] UI routes updated if exposed on web.

Rules: [.agents/rules/memory.mdc](../../.agents/rules/memory.mdc)
