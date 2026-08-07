# Agent instructions

Prefer `so graph query` and this file before broad code search.

## Architecture

Open-source **agent harness engineering** (not a coding agent). The `so` CLI + local `.so/` harness give coding agents (Claude Code, Cursor, Codex, Gemini, OpenCode, Copilot, Pi) shared graph context, memory, guardrails, session evals, and recommendations so each session wastes fewer tokens.

## Layout (look here first)

| Path | Role |
|---|---|
| `cmd/so` | Cobra CLI entrypoints (`init`, `graph`, `doctor`, `dev`, `eval`, `recommend`, …) |
| `internal/` | Core packages: harness seed/upgrade, graph, memory, inject, sessions, OTLP (`codingotlp`/`otlp`), eval, recommend, guardrails, harvest, sync |
| `web/` | Next.js Sessions UI (local `so dev`, default http://localhost:4444) |
| `sdk/go` | Go SDK (replace → `./sdk/go`) |
| `plugins/`, `templates/`, `npm/` | Agent/marketplace plugins, harness templates, npm packaging |
| `scripts/` | Install, plugin sync, release helpers |
| `docs/` | Product/docs |
| `.so/` | Per-repo harness data (graph, guardrails, sessions, memory) — ordinary files agents and UI read/write |

## How pieces relate

1. **`so init` / seed** — build or refresh repo graph (Graphify), seed `.so/`, write upgrade brief; assistant model upgrades AGENTS.md / guardrails / evals via `so apply-upgrade`.
2. **`so install`** — register `/so` skill into agent discovery paths (does not create `.so/`).
3. **Session loop** — agent OTLP → `.so/sessions/` → eval → recommendations → approve/apply → better next-session injectors.
4. **Injectors / sync** — `internal/inject`, `internal/sync` refresh agent context after harness edits.

## Agent navigation tips

- Codebase questions: prefer `so graph query "…"` over broad Grep when `.so/graph/graph.json` exists.
- Conventions: root `AGENTS.md`, `.cursor/rules/`, `.agents/skills/`, `.so/guardrails/guardrails.yaml`.
- CLI changes → `cmd/so` + matching `internal/<pkg>`; UI → `web/src`; telemetry → `internal/codingotlp` / `internal/otlp`.

## Conventions

## Scope and PRs
- Discuss substantial changes in an issue first; small docs can go straight to a PR.
- Keep PRs focused; no unrelated formatting or refactors.
- Prefer Conventional Commit titles (`feat: …`, `fix(web): …`).
- Explain problem + solution; link issues; include tests for behavior changes and update user-facing docs.

## Go / CLI
- Module: `github.com/ishanjainn/superopen` (Go 1.26+).
- Build: `go build -o bin/so ./cmd/so`.
- Before finishing Go work: `go test -race -count=1 ./...` and `go vet ./...` (or focused package tests).
- Prefer existing `internal/` package boundaries; avoid inventing parallel packages for the same concern.

## Web UI
- App lives in `web/` (Next.js).
- Checks: `cd web && npm ci --ignore-scripts` then `npm test`, `npm run typecheck`, `npm run lint`.
- Honor repo npm safety: prefer `--ignore-scripts` on installs.

## Plugins and smoke
- After plugin template changes: `bash scripts/sync-plugins.sh` and commit marketplace drift.
- Optional local smoke: `make smoke` (writes a local `.so/`).

## Security and secrets
- Never commit API keys, tokens, or credentials; do not invent secrets in docs or fixtures.
- Report vulnerabilities via SECURITY.md, not public issues.
- Redaction / sensitive session data: follow existing `internal/redact` and harness patterns.

## Agent harness edits
- Prefer `so graph query`, `so sync`, and single-file `.so/guardrails/guardrails.yaml` over ad-hoc duplicate rule files.
- When updating AGENTS.md / rules / skills, prune obsolete guidance—do not only append.

<!-- superopen:learned:start -->
## Superopen learned

<!-- superopen:learned:end -->
