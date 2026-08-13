# Agent instructions

Prefer `so graph query` and this file before broad code search.

## Architecture

Superopen is **agent harness engineering** for coding agents (Claude Code, Cursor, Codex, Gemini, OpenCode, Pi, etc.) — not another coding agent. It builds a local repository graph, session telemetry, memory, guardrails, evals, and recommendations so each session starts with better context.

## Layout

| Area | Role |
|------|------|
| `cmd/so/` | Cobra CLI entrypoint (`so init`, `so graph`, `so sessions`, `so dev`, …) |
| `internal/` | Core packages: `graph`, `session`, `eval`, `memory`, `harvest`, `coding/hook`, `inject`, `seed`, `recommend`, `guardrails`, `config`, `doctor` |
| `web/` | Next.js session replay UI (`so dev` serves at localhost:4444) |
| `sdk/go/` | Public Go SDK (replaced locally via `go.mod`) |
| `plugins/` | Agent marketplace plugins (e.g. Pi) |
| `.so/` | Runtime harness: `config.yaml`, `guardrails.yaml`, `evals.yaml`, `graph/`, `sessions/`, `memory/` |

## Data flow

```
agent hooks → .so/sessions/<id>/events.jsonl
           → session.json + eval + recommendations
           → memory/state.json + vendor rules/skills updates
```

Graph builds via Graphify into `.so/graph/graph.json`. Hooks write OTLP-style events; `so dev` and the web UI are read-only consumers.

## Where to look first

- CLI commands: `cmd/so/main.go` and `cmd/so/*.go`
- Session lifecycle: `internal/session/`, `internal/eval/`, `internal/harvest/`
- Agent integration: `internal/coding/`, `internal/inject/`, vendor trees (`.cursor/`, `.claude/`, …)
- Graph/query: `internal/graph/`
- Schema/docs: `docs/so-schema.md`, `AGENTS.md`

Use `so graph query "<question>"` before broad grep when exploring behavior.

## Conventions

## Development workflow

1. Fork, branch (`fix/short-description`), focused commits, PR against `main`.
2. Keep diffs small — no unrelated refactors or drive-by formatting.
3. Include tests and docs for behavior changes.
4. Search existing issues before filing new ones; security issues go through `SECURITY.md`.

## Checks by area

| Area | Command |
|------|---------|
| Go / CLI | `go test -race -count=1 ./...` · `go vet ./...` |
| Web UI | `cd web && npm test && npm run typecheck && npm run lint` |
| Plugins | `bash scripts/sync-plugins.sh` then commit marketplace drift |

## Commits & PRs

- Use Conventional Commit titles: `feat: …`, `fix(web): …`, `docs: …`.
- Explain problem and solution in the PR; link related issues.
- Complete the PR template honestly.

## Go style

- Match existing package layout under `internal/`.
- Prefer table-driven tests with `_test.go` beside source.
- Use `so --json` when scripting CLI output.
- Do not edit `go.sum` or lockfiles directly — run the package manager.

## Frontend (`web/`)

- Run typecheck and lint before finishing UI work.
- Reuse existing components and design tokens.
- Sanitize untrusted HTML; prefer React `{}` bindings over `dangerouslySetInnerHTML`.

## Agent harness

- Prefer `so graph query` and `AGENTS.md` before broad code search.
- Read `.so/guardrails.yaml` for enforced safety rules.
- Use vendor-native rules/skills (`.cursor/rules/`, `.claude/skills/`) for editor-specific guidance.

<!-- superopen:learned:start -->
## Superopen learned


Superopen harness is active. Start with `so graph query` for codebase questions, read AGENTS.md for architecture/conventions, and check `.so/guardrails.yaml` for enforced safety. Vendor rules live under `.cursor/rules/` (Cursor) or the active agent's tree. Use `so memory search` for prior session lessons. Run focused tests before finishing: `go test -race -count=1 ./...` for Go, `cd web && npm test && npm run typecheck && npm run lint` for UI.
<!-- superopen:learned:end -->
