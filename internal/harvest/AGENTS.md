# Harvest

Playbook harvest: after a coding session, propose small patches to instruction files (AGENTS.md, vendor rules, skills). Humans approve before anything becomes always-on.

Parent index: [../../AGENTS.md](../../AGENTS.md). CLI: [../../cmd/so/AGENTS.md](../../cmd/so/AGENTS.md). UI: [../../web/AGENTS.md](../../web/AGENTS.md).

## Why

A bad always-on rule is paid on every future session. Harvest stays **off the hot path**: no Stop/SessionEnd inject, no methodology in `Block()`. At most one bounded headless call after finalize. Prefer **simplify/delete**. Cap 3 proposals. Skip when already harvested, empty, duplicate, or the live file already contains the change.

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
| `start.go` | Optional SessionStart one-liner |

## Product rules

- Never write live playbooks from finalize or headless.
- Graph refresh does not wait on harvest.
- Headless prompt: inventory hashes + compact session digest, **not** full transcripts or full playbook bodies.
- `reason` required. `evidence` required when `session_id` is known.

CLI: `cmd/so/harvest_cmd.go`
