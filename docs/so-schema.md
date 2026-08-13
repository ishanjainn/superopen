# Superopen schema (layout v2)

Superopen keeps shared policy compact and separates vendor-owned guidance. Runtime files under `.so/` are ordinary, inspectable files; generated and machine-local state is ignored by Git.

## Native guidance

| Path | Ownership |
|---|---|
| `AGENTS.md` and nested `*/AGENTS.md` | Shared. Superopen changes only its managed learned sections. |
| `.claude`, `.codex`, `.cursor`, `.gemini`, `.opencode`, `.github`, `.pi` | Owned by that vendor. A session may update only its own vendor tree. |
| `.agents` | Shared opt-in only: `--shared-agents`, `--vendor agents`, or `vendors.shared_agents: true`. |

`so init` and `so install` enable detected vendors plus repeatable `--vendor` flags. `so sync` refreshes enabled plumbing and never copies **session** guidance between vendors. **Init/upgrade** is the exception: project-wide skill picks from `so apply-upgrade` fan out to every enabled vendor tree, and committed `mcp:` policy is projected into shared project MCP files.

## Compact `.so` layout

```text
.so/
├── .gitignore
├── config.yaml
├── guardrails.yaml
├── evals.yaml
├── graph/
│   ├── graph.json
│   ├── corpus.json
│   ├── graph.html
│   └── state.json
├── audit/
│   └── events.jsonl                 # repository-level audit history
├── sessions/
│   ├── index.json
│   ├── inbox.jsonl                   # lazy unresolved telemetry
│   └── <session-id>/
│       ├── events.jsonl
│       ├── session.json
│       └── checkpoints/              # lazy
│           └── manifest.json
└── memory/
    ├── context.md
    └── state.json
```

Every Superopen-created file says why it exists, its authority, and its updater:

- YAML and `.gitignore` use a leading `#` description.
- JSON uses a top-level `_about` object.
- JSONL begins with a `superopen.file_manifest` record.
- HTML and Markdown use a leading HTML comment.
- Checkpoint payloads retain exact source bytes; `checkpoints/manifest.json` documents them.

Committed policy is `.so/config.yaml` (including optional `mcp.servers`), `.so/guardrails.yaml`, and `.so/evals.yaml`. Graph, audit, session, and memory data stays local and rebuildable where applicable. Applied project skills live under vendor trees (for example `.claude/skills/`, `.cursor/skills/`) and are not gitignored. Project MCP projections (repo-root `.mcp.json`, `.cursor/mcp.json`) are written by `so sync` / `so apply-upgrade` from `mcp:` in config and should be committed so teammates share the same servers. The stable directories and their root files are created by `so init`; only unresolved telemetry, per-session directories, and checkpoints are event-driven. Graphify caches, server PID/locks, and port ledgers live in OS cache/runtime directories. Small debounce and sweep timestamps share one self-described OS-cache `runtime/state.json`; Superopen does not create `finalize-pending`, `pending-harvest.json`, `approval-mismatch-at`, or `idle-sweep-at`. UI preferences live in browser local storage. `so dev` only serves these files; graph refreshes run after changed sessions or through explicit CLI commands.

### MCP team policy

```yaml
# in .so/config.yaml
mcp:
  servers:
    - name: context7
      command: npx
      args: ["-y", "@upstash/context7-mcp@1.0.0"]
```

Authoritative list is `mcp.servers` (no secrets/env). `so sync` merges those entries into project-scoped vendor files for enabled agents (never under the user home directory). Extra human-added servers in `.mcp.json` are preserved. Unpinned `@latest` packages and Memory MCP are refused. After graph build, `so upgrade-brief` lists automation candidates from stack signals; the assistant picks 1–2 MCP and 1–2 skills in upgrade JSON; `so apply-upgrade` writes them as committed policy. MCP omitted from upgrade stays for the next refresh (not a recommendation type). High-signal skills/guardrails omitted from upgrade may enqueue as ordinary pending recommendations.

Fresh configuration does not advertise an API provider or model. Reviews use the same vendor's sealed CLI when possible, then another configured sealed CLI, then heuristics. The optional `llm:` section is written only when a maintainer explicitly configures an API or compatible local backend.

`.so/guardrails.yaml` is the one authoritative safety policy. `denied_tools`, `denied_commands`, and `sensitive_paths` are enforced at supported pre-tool hook boundaries; `rules` remains advisory guidance. Tool-name patterns support `*` wildcards and use the names reported by vendor hooks.

## Session lifecycle

Telemetry writes full secret-redacted local events directly to `<session-id>/events.jsonl`; there is no reduced-content capture setting. `session.json` materializes metadata, footprint, evaluation, recommendations, replay, port provenance, review state, and compact evidence references. Repository-level audit events live separately in `audit/events.jsonl`; session-associated audit events stay in that session's `events.jsonl`.

On a true SessionEnd, Superopen atomically marks review pending and launches a detached worker. Stop-only vendors are finalized by explicit close, idle handling, or when a different session ID starts for the same vendor. Startup checks only the immediately preceding same-vendor session, injects its completed review or a short pending notice, and never blocks or processes older backlog.

One review claim prevents duplicate end/start workers. The reviewer prefers the same vendor's sealed CLI and records the actual backend or heuristic fallback in `session.json`. The same review classifies corrections, recurring workflows, failures, successful verification, and guidance gaps. Durable counters and redacted summaries are consolidated into `memory/state.json`; proposed bodies remain only in recommendation records. Soft recommendations may update the originating vendor's existing rules/skills and managed shared `AGENTS.md` sections. New skills remain visible after their first supporting session and auto-create only after three same-vendor sessions with successful verification, or an explicit durable user workflow. Removals, restructures, guardrails, and evaluation-policy changes always require approval.

If a session edited source files, the worker builds Graphify and the corpus in a temporary directory, validates the described JSON/HTML plus query behavior, then atomically swaps the graph. Failures preserve the previous valid graph. Documentation-only changes rebuild `graph/corpus.json` without repeating the source graph.

## Observability

The only local exporter target is the unified session store:

```yaml
observability:
  exporters:
    - type: local_jsonl
      path: .so/sessions
```

Coding-agent hooks write this file store directly. `so dev` and the web UI
only read it; neither starts nor requires a telemetry receiver. Network
telemetry export is unavailable.

## AXI output

Use `so --json …` (or `SO_JSON=1`) for a JSON envelope and `--full` (or `SO_FULL=1`) to disable truncation. Bare `so` prints a compact status snapshot.
