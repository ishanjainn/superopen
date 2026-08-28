# Memory

A project diary over your coding sessions, stored in the same SQLite file as
the graph (`.so/db/so.db`). It answers "what did we do/decide last time"
without re-reading transcripts.

## What gets in

| Source | Becomes |
|--------|---------|
| User prompts (recorded by session hooks) | Diary entries, verbatim |
| Tool spans | Observations `{tool, path, state}` — results are not stored |
| `so memory capture` / agent JSON | **Knowledge** and skills with a horizon |
| Post-session distill | Session rollups distilled into knowledge |

Diary material (prompts/tools) is never promoted to knowledge by itself;
knowledge exists only from explicit capture or distill.

## Horizons

`short | medium | long`. The horizon drives ranking and mechanical expiry —
teachings, pins, and `never_decay` items are never deleted by age.

## Commands

| Command | Purpose |
|---------|---------|
| `so memory` | Live dashboard (no args) |
| `so memory search "<query>"` | Hybrid search; TOON index rows (`id, kind, title, tokens`) |
| `so memory get <id>` | Full bodies |
| `so memory last` / `timeline` / `temporal-recall` | Recency, WHEN folders, time-bounded recall (`--as-of`) |
| `so memory capture --kind knowledge --horizon medium --title … --text …` | Write knowledge |
| `so memory teach <file\|--text>` | Study a source into chunked, deduped memories |
| `so memory contradict <id>` | Write a successor that supersedes an older memory |
| `so memory recall` | Budgeted pack of hits + anti-hits (on-demand) |
| `so memory pin / forget / fade / rescue` | Lifecycle: keep forever, hide, soft-decay, undo |
| `so memory ingest` | Project `events.jsonl` into episodes (local, no LLM) |
| `so memory distill` | Headless session rollup; pause/consolidate control |
| `so memory sleep` | Decay edges, erase hinted memories, cluster topics |
| `so memory status` | Health, economy, pending distill |

## How agents see it

- **SessionStart** may append a ≤350-token ID-index (titles only, no bodies)
  that starts graph-first. Empty store → silent.
- Prompt-submit stays silent unless the prompt matches prior-work cues
  ("last time", "we decided", "remember", "what did we"); then matching
  recalled bodies inject (pointer-only index if recall is empty).
- Memory is **hints, not authority**: injection never replaces
  `so graph query` for structural questions.

## Privacy & retention

Sensitive content goes through shield/privacy filters before persistence.
`so gc` deletes unpinned prompts/session rollups past
`SUPEROPEN_MEMORY_RETENTION_HOURS` (default 168). Search shows IDs/titles;
bodies stay behind `get`.

## For contributors

Contracts M1–M4 and package layout:
[`internal/memory/AGENTS.md`](../internal/memory/AGENTS.md); CLI wiring in
[`cmd/so/memory_cmd.go`](../cmd/so/memory_cmd.go).
