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
| `so memory distill` | Session rollup (own one-shot CLI, `--apply` JSON, `--brief`, `--consolidate` cap 5 / `--all`) |
| `so memory sleep` | Decay edges, erase hinted memories, cluster topics |
| `so memory status` | Health, economy, pending distill |

## How agents see it

- **SessionStart** may append a ≤350-token pointer (graph one-liner or N-memories
  + recall). Empty store → silent except a pending distill line. Pending harvest
  or distill may add one extra line. OPEN harvest review is not injected.
- Prompt-submit stays silent unless the prompt matches prior-work cues
  ("last time", "we decided", "remember", "what did we"); then matching
  recalled bodies inject (pointer-only index if recall is empty).
- Memory is **hints, not authority**: injection never replaces
  `so graph query` for structural questions.

## Privacy & retention

Sensitive content goes through shield/privacy filters before persistence.
Episode bodies are AES-GCM sealed on disk (`so.db.key`, mode 0600). The
FTS search index stores **plaintext** of those bodies so keyword search
works — anyone with filesystem access to `.so/db/` can read the index
sidecar even though `memory_episodes.text` is encrypted.

`so memory distill` is the only path that sends session content off the
machine: when the session's own one-shot CLI is authenticated, a session
digest is posted to that provider. Workers are not recorded as sessions.
Live agents print the same prompt with `so memory distill --brief <id>` and
apply JSON with `so memory distill --apply <id>` (empty array if nothing
durable). It is opt-in via that CLI's login; no network embed or cloud
memory by default.

`so gc` deletes unpinned prompts/session rollups past
`SUPEROPEN_MEMORY_RETENTION_HOURS` (default 168). Search shows IDs/titles;
bodies stay behind `get`.

## For contributors

Contracts M1–M4 and package layout:
[`internal/memory/AGENTS.md`](../internal/memory/AGENTS.md); CLI wiring in
[`cmd/so/memory_cmd.go`](../cmd/so/memory_cmd.go).
