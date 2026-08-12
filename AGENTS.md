# Agent instructions

Prefer `so graph query` and this file before broad code search.

## Architecture

Superopen is a local-first agent-harness platform. The Go CLI builds and maintains repository context, shared memory, guardrails, session telemetry, evaluations, and recommendations; the Next.js application renders the same file-backed harness as a local UI. Repository state is stored under `.so/`, so CLI and UI features should agree on its schema.

## Entry points and boundaries

- `cmd/so/` is the Cobra CLI entrypoint and command wiring. Keep orchestration thin here and place reusable behavior in `internal/`.
- `internal/initcmd`, `internal/seed`, `internal/graph`, `internal/harness`, and `internal/sync` create and refresh the harness and repository graph.
- `internal/session`, `internal/coding`, `internal/codingotlp`, `internal/otlp`, and `internal/tracestore` capture and persist coding-agent sessions and telemetry.
- `internal/memory`, `internal/retrieve`, `internal/learn`, and `internal/nativedocs` manage durable context and native instruction documents.
- `internal/eval`, `internal/recommend`, `internal/guardrails`, and `internal/audit` score sessions, propose controlled changes, enforce policy, and record decisions.
- `internal/port` and vendor adapters bridge supported coding agents; `plugins/` contains distributable integrations. Preserve vendor-specific behavior behind these boundaries.
- `sdk/go/` is the local Go SDK module, linked from the root module with a `replace` directive.
- `web/` is a Next.js/React/TypeScript application. Its API routes read local `.so/` artifacts, while components visualize graph, sessions, memory, evals, and harness documents. Session replay lives in `web/src/map`.
- `templates/` owns seed content for rules, skills, guardrails, evals, and memory. Changes can affect newly initialized repositories and should remain compatible with generated artifacts.
- `scripts/` supports installation, release, plugin synchronization, and repository automation; `npm/` contains package-distribution assets.

## Data flow

A coding-agent integration emits local hook events, which are stored as `.so/sessions/<id>/` data. Evaluation turns session evidence into scores and recommendations; approved changes update harness guidance consumed by later sessions. Graphify produces `.so/graph/graph.json` and the HTML graph used by CLI queries and the UI.

## Where to look first

Use `so graph query` for codebase questions, then inspect the surfaced `internal/` package and its tests. For persisted formats, read `docs/so-schema.md` and the relevant loader before changing writers. For CLI/UI parity, trace both the Go command and the corresponding `web/src/app/api/` route or `web/src/lib/so/` reader.

## Conventions

- Query the Superopen graph and read repository guidance before broad code search.
- Keep command handlers in `cmd/so/` small; implement reusable logic in focused `internal/` packages.
- Preserve the local-first, file-backed contract and CLI/UI parity when changing `.so/` artifacts.
- Follow existing Go package boundaries, naming, error handling, and table-driven test patterns.
- Format Go changes and run `go test -race -count=1 ./...` plus `go vet ./...` for CLI or shared-package work.
- Run `cd web && npm test && npm run typecheck && npm run lint` for web changes; run the Next build when routing, configuration, or production behavior changes.
- Add or update tests for behavior changes and update user-facing documentation when commands, configuration, schemas, or workflows change.
- Run `bash scripts/sync-plugins.sh` after plugin or shared integration changes, and include any intentional marketplace drift.
- Preserve pre-existing, non-Superopen agent rules and skills during install, sync, and uninstall flows.
- Keep repository mutations scoped and recoverable; avoid unrelated formatting or refactors.
- Use clear branch names such as `fix/short-description` and prefer Conventional Commit pull-request titles such as `feat: add project prune` or `fix(web): restore file search`.
- Explain the problem and solution in pull requests, link relevant issues, and complete the pull-request template honestly.
- Keep credentials and sensitive logs out of source, fixtures, generated harness data, and public issues; follow `SECURITY.md` for vulnerability reports.

<!-- superopen:learned:start -->
## Superopen learned

<!-- superopen:learned:end -->
