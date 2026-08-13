# Agent instructions

Prefer `so graph query` and this file before broad code search.

## Architecture

Superopen is open-source **agent harness engineering**: a local-first CLI (`so`) plus web UI that builds a repository graph, injects shared context into coding agents, scores sessions, and proposes harness improvements.

## Where to look first

1. `so graph query "<question>"` when `.so/graph/graph.json` exists.
2. `AGENTS.md` and `.so/guardrails.yaml` for project policy.
3. The package or directory that matches your task (see below).

## Top-level layout

| Path | Role |
|------|------|
| `cmd/so/` | CLI entrypoint and subcommands (`init`, `graph`, `memory`, `eval`, `dev`, …) |
| `internal/` | Core Go packages: graph, harness, memory, session telemetry, evals, guardrails, inject, seed, mcp, coding hooks |
| `web/` | Next.js local sessions UI (`so dev` on `:4444`) |
| `sdk/go/` | Go SDK consumed by plugins and external callers |
| `plugins/` | Coding-agent hook integrations and marketplace sync |
| `templates/` | Seed content for harness docs, skills, rules, guardrails |
| `scripts/` | Install, release, and plugin-sync automation |
| `.so/` | Per-repo harness data: graph, config, sessions, memory, evals |

## Internal package map (high signal)

- `internal/graph` — repository graph build and query (Graphify-backed).
- `internal/harness`, `internal/seed`, `internal/initcmd` — bootstrap and upgrade flows.
- `internal/memory`, `internal/retrieve` — cross-session recall and corpus search.
- `internal/session`, `internal/coding`, `internal/codingotlp` — hook telemetry and OTLP export.
- `internal/eval`, `internal/recommend` — session scoring and harness recommendations.
- `internal/guardrails`, `internal/inject` — policy enforcement and agent injectors.
- `internal/mcp` — MCP server projection for agents.
- `internal/port` — chat/session porting across vendors.

## Service boundaries

- **CLI** (`cmd/so` + `internal/*`) owns harness lifecycle, graph, memory, evals, and vendor hooks.
- **Web UI** (`web/`) reads/writes `.so/` for session replay and harness inspection; no separate backend service.
- **Plugins/SDK** wire agent runtimes to Superopen telemetry; they are not a general LLM SDK.

## Data & APIs

- All harness state is file-backed under `.so/` (config, graph, sessions, memory).
- Graph data: `.so/graph/graph.json` (regenerable via `so graph`).
- External deps: OpenTelemetry, Cobra, YAML; optional LLM backends for evals only.

## Conventions

## Workflow

- Prefer `so graph query` and `AGENTS.md` before broad grep or glob exploration.
- Keep changes focused; avoid unrelated refactors or drive-by formatting.
- Discuss substantial changes in an issue before large PRs; small doc fixes can go straight to PR.

## Go / CLI

- Match existing package layout under `internal/` and `cmd/so/`.
- Run `go test -race -count=1 ./...` and `go vet ./...` for Go changes.
- Add `_test.go` coverage for behavior changes; use table-driven tests where the repo already does.
- Follow existing error-handling and Cobra command patterns in sibling files.

## Web UI (`web/`)

- Run `npm test`, `npm run typecheck`, and `npm run lint` after UI changes.
- Use existing component and test patterns (Vitest, TypeScript strict).

## Plugins

- After plugin/marketplace edits, run `bash scripts/sync-plugins.sh` and commit any drift.

## Commits & PRs

- Use Conventional Commit titles: `feat: …`, `fix(web): …`, `docs: …`.
- Explain problem and solution; link related issues.
- Include tests for behavior changes and update user-facing docs when relevant.
- Complete the PR template honestly.

## Lockfiles & generated output

- Change lockfiles only via the package manager (`go mod`, `npm ci`), not by hand-editing.
- Do not edit generated artifacts (`dist/`, `vendor/`, `*.generated.*`).

## Superopen harness edits

- Prune obsolete guidance instead of only appending to rules/skills.
- After meaningful `.so/` or injector edits, run `so sync`.

<!-- superopen:learned:start -->
## Superopen learned


Start with `so graph query` for codebase questions and read AGENTS.md plus `.so/guardrails.yaml`. For Go work run `go test -race -count=1` and `go vet`; for `web/` run npm test, typecheck, and lint. Use vendor-native rules/skills dirs (`.cursor/`, `.claude/`, etc.) and project skills `gen-test` / `pr-check` when relevant. After harness edits run `so sync`.
<!-- superopen:learned:end -->
