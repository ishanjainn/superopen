# Harvest

Playbook harvest: after a coding session, propose small patches to instruction files (AGENTS.md, vendor rules, skills). Humans approve before anything becomes always-on.

Parent index: [../../AGENTS.md](../../AGENTS.md). CLI: [../../cmd/so/AGENTS.md](../../cmd/so/AGENTS.md). UI: [../../web/AGENTS.md](../../web/AGENTS.md).

## Why

A bad always-on rule is paid on every future session. Harvest stays **off the hot path**: no Stop/SessionEnd inject, no methodology in `Block()`. Live agent first: SessionStart and the first prompt-submit inject `HARVEST pending` (`so harvest brief <id>` then `so harvest propose`, or `so harvest skip <id>`). SessionEnd uses the live vendor's own one-shot CLI when it is authenticated; Cursor/Copilot/Gemini and failed one-shot runs mark pending for the next SessionStart. There is no cross-vendor fallback. Prefer **simplify/delete**. Cap 3 proposals. Skip when already harvested, empty, duplicate, worker session, or the live file already contains the change. Headless workers set `SUPEROPEN_HEADLESS=1` and must not be recorded as sessions. SessionEnd finalize is **single-flight per session**. Harvest/distill run at most once per session; duplicate SessionEnd must not spawn another worker.

## Layout

| File | Role |
|------|------|
| `store.go` | SQLite tables on `.so/db/so.db` (`CREATE TABLE IF NOT EXISTS`, no ALTER of memory tables) |
| `inventory.go` | Discover playbook files |
| `protect.go` | Sentinel, `/so` skill tripwire, Superopen hooks |
| `patch.go` | Unified diff parse/apply |
| `propose.go` | Ingest JSON from live agent or headless |
| `generate.go` | Skip gates + bounded headless |
| `apply.go` | Gated live write |
| `start.go` | Pending live line (`brief` then `propose` or `skip`), never OPEN review |

## Product rules

- Never write live playbooks from finalize or headless.
- Graph refresh does not wait on harvest.
- Headless prompt: inventory hashes + compact session digest, **not** full transcripts or full playbook bodies.
- `reason` required. `evidence` required when `session_id` is known.

CLI: `cmd/so/harvest_cmd.go`
