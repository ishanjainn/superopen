# Agent instructions

Prefer `so graph query` and this file before broad code search.

## Architecture

Superopen is a local-first agent-harness platform. The Go CLI builds and manages repository knowledge, captures coding-agent telemetry, evaluates sessions, and updates a file-backed `.so/` harness. A Next.js application presents the same sessions, recommendations, memory, and 3D replay data from local files.

## Entry points and boundaries

- `cmd/so/` is the CLI entry point and command dispatch layer. Keep orchestration here thin and put reusable behavior in `internal/`.
- `internal/initcmd`, `internal/harness`, `internal/seed`, `internal/inject`, and `internal/sync` create and refresh `.so/`, native agent instructions, rules, skills, graphs, guardrails, and eval configuration.
- `internal/coding` detects supported agents, installs hooks, normalizes vendor events, tracks runtime state, and feeds session telemetry into `internal/session` and `internal/tracestore`.
- `internal/eval`, `internal/recommend`, `internal/improve`, `internal/learn`, and `internal/memory` implement the feedback loop from recorded sessions to scored evidence and proposed harness improvements. `internal/llm` and `internal/agentcli` provide optional model-backed evaluation; offline heuristics remain a supported fallback.
- `internal/graph` and `internal/retrieve` provide repository graph and corpus retrieval. Query `.so/graph/graph.json` through `so graph query` before broad source searches.
- `internal/guardrails`, `internal/audit`, `internal/redact`, and `internal/retention` enforce policy, preserve an audit trail, remove sensitive material, and manage stored data.
- `web/` is the local Next.js UI. Routes and file-backed APIs live under `web/src/app`, reusable UI under `web/src/components`, data access under `web/src/lib/so`, and session replay under `web/src/map`.
- `plugins/` contains vendor integration packages; `sdk/go` contains hook helpers and semantic conventions used by integrations. `templates/` is the source for generated harness content. `npm/` and release scripts package distributable integrations.

## Data flow

Coding-agent hooks emit vendor-specific events, normalization converts them to shared session records, and the trace/session stores persist them under `.so/`. Evaluation consumes those records and can create recommendations; approved changes update harness files and future injected context. The CLI and web UI operate on this shared file-backed model, so schema or path changes must be kept compatible across both surfaces.

## Where to start

Read `AGENTS.md`, `.so/guardrails.yaml`, and `docs/so-schema.md`. For behavior, start at the relevant `cmd/so` command, follow its `internal` package, then inspect matching web API or plugin code only when the change crosses those boundaries.

## Conventions

- Query the Superopen graph and read repository guidance before broad code search.
- Keep command handlers in `cmd/so` small; place reusable logic in focused `internal` packages.
- Preserve the local-first, file-backed contract between the CLI, `.so/` schema, vendor hooks, and web UI.
- Follow existing Go and TypeScript naming and package patterns; format Go with `gofmt` and use the repository's configured web linting.
- Add tests for behavior changes and update user-facing documentation when commands, schemas, configuration, or workflows change.
- Run focused checks while iterating, then run `go test -race -count=1 ./...` and `go vet ./...` for Go or CLI changes.
- For web changes, run `cd web && npm test && npm run typecheck && npm run lint`; include a Next build when routing or production bundling is affected.
- After plugin changes, run `bash scripts/sync-plugins.sh` and commit intentional marketplace or generated-file drift.
- Keep changes focused; exclude unrelated formatting, refactors, and generated artifacts.
- Preserve cross-platform behavior for filesystem paths, process handling, and hooks on Linux, macOS, and Windows.
- Redact sensitive values from logs, telemetry, fixtures, and checked-in harness data. Never commit credentials or secrets.
- Use Conventional Commit pull-request titles such as `feat: add project prune` or `fix(web): restore file search`.
- Explain the problem and solution in pull requests, link related issues when available, and complete the pull-request template accurately.

<!-- superopen:learned:start -->
## Superopen learned

<!-- superopen:learned:end -->
