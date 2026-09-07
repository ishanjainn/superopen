# Sessions

Observability for coding-agent sessions. Installed hooks record what happened;
`so sessions` turns that into inspectable documents with token and cost totals.

## Pipeline

1. **Record** — while you work in an inited repo, `so sessions hook`
   (invoked by installed plugin manifests) appends observability spans to
   `.so/sessions/`: session start/end, user prompts, tool calls, assistant
   turns. It always exits 0 on telemetry failure and stays silent — it never
   adds model-visible text on this path.
2. **Materialize** — `so sessions finalize` (also detached on SessionEnd)
   folds raw spans into a completed session document plus a session map.
3. **Feed memory** — finalized sessions are what `so memory ingest` / distill
   read. See [memory.md](memory.md).

## Commands

| Command | Purpose |
|---------|---------|
| `so status` | Active sessions right now |
| `so sessions` / `list` | List coding sessions |
| `so sessions show <id>` | The materialized session document |
| `so sessions refresh <id>` | Re-read current spans into an active session |
| `so sessions tokens [<id>]` | Token and cost totals |
| `so sessions checkpoint` | Manage restorable checkpoints under `.so/sessions/<id>/checkpoints/` |
| `so sessions demo` | Synthetic session for UI testing |

Output follows the axi.md conventions: TOON lists, definitive empty state
(`0 sessions`) with `help[]`, `--json` envelope when asked.

## What is recorded (and what is not)

- Read/Edit/Write tool calls are stamped with the repo-relative
  `coding_agent.file_path`; shell commands are never stamped. Prompts and tool
  arguments are redacted; VCS revision metadata is not.
- Cost accounting counts each usage span once (session root, else the latest
  stop event). No usage spans → zero tokens; nothing is inferred from
  character counts.
- Finalize never replaces a longer event log with a shorter one; duplicate
  thoughts and double-recorded reads are deduped at storage time.

## Retention

`so gc` applies retention: unpinned prompts and session rollups older than
`SUPEROPEN_MEMORY_RETENTION_HOURS` (default 168) are deleted; transcripts and
closed harvest history use `SUPEROPEN_SESSION_RETENTION_HOURS`. Pins, teachings,
open harvest proposals, and pending harvest runs are never aged out.

## For contributors

Contracts S1–S4 and vendor hook channels:
[`internal/agent/AGENTS.md`](../internal/agent/AGENTS.md),
store implementation in [`internal/session/store.go`](../internal/session/store.go).
