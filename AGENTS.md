# Agent instructions

Prefer `so graph query` and this file before broad code search.

## Architecture

Superopen is open-source **Agent Harness Engineering**: a Go CLI (`so`) plus local Sessions UI that builds repository graph, context, rules, skills, guardrails, session evals, and recommendations around Claude Code, Cursor, Codex, Gemini, OpenCode, Copilot CLI, Pi, and similar tools.

## Where to look first

| Path | Role |
|---|---|
| `cmd/so/` | CLI entrypoint and command wiring |
| `internal/` | Core packages (install, hooks, harness seed, graph, memory, OTLP, eval, recommend) |
| `web/` | Next.js Sessions UI (localhost:4444 via `so dev`) |
| `templates/` | Seeded rules/skills/memory/gitignore content |
| `plugins/`, `sdk/`, `npm/` | Agent plugin packaging and distribution |
| `docs/` | Schema and product docs |
| `.so/` | Per-repo harness data (graph, knowledge, guardrails, memory, sessions) |

## Package map (`internal/`)

- **Agent integration**: `coding/` (detect/install/hooks), `inject/`, `nativedocs/`, `harness/`, `harnessvalid/`
- **Bootstrap**: `initcmd/`, `seed/`, `sync/`, `discover/`
- **Knowledge**: `graph/`, `retrieve/`, `memory/`, `learn/`, `harvest/`
- **Governance**: `guardrails/`, `config/`, `redact/`, `audit/`
- **Sessions & evals**: `codingotlp/`, `otlp/`, `tracestore/`, `session/`, `eval/`, `recommend/`, `port/`
- **Ops**: `doctor/`, `githooks/`, `gitruntime/`, `userpaths/`, `version/`

## Runtime loop

1. `so install` registers `/so` (or `$so`) skills and hooks with coding agents.
2. `/so init` / `so init` builds `.so/graph/` and seeds knowledge, guardrails, evals, memory.
3. Sessions emit OTLP → materialized under `.so/sessions/` when `so dev` is running.
4. `so eval` / `so recommend` score sessions and propose harness improvements.

Prefer `so graph query "…"` and `.so/knowledge/` before broad Grep/Glob. Guardrails live in a single file: `.so/guardrails/guardrails.yaml`.

## Conventions

## Scope and PRs

- Keep diffs scoped to the requested task; no drive-by refactors or unrelated formatting.
- Prefer Conventional Commit titles: `feat:`, `fix:`, `docs:` (example: `feat: add project prune`).
- Open or discuss an issue for substantial changes; small docs can go straight to PR.
- Link the related issue; summarize problem + solution in the PR body.
- Do not invent secrets or fake paths in examples or docs.

## Go / CLI

- Match neighboring style under `internal/` and `cmd/so/`.
- While iterating, run package-scoped checks: `go test -race -count=1 ./path` and `go vet ./path`; broaden to `./...` before merge.
- Prefer channels for ownership transfer; mutexes for shared state.
- Never start a goroutine without a clear shutdown or WaitGroup story.
- Always race-test packages that share state across goroutines.

## Web UI

- From `web/`: `npm test`, `npm run typecheck`, `npm run lint` for UI changes.
- Sanitize untrusted HTML/SVG before render; build URLs with `URL` / `URLSearchParams`, not string concat.

## Plugins and secrets

- After plugin marketplace edits: `bash scripts/sync-plugins.sh` and commit any drift.
- Never commit secrets, tokens, or real `.env` values; never log credentials.

<!-- superopen:learned:start -->
## Superopen learned

<!-- superopen:learned:end -->

<!-- superopen:start -->
## Superopen

This project is managed by Superopen (`.so/`). Prefer `.so/` before raw exploration to save tokens.

Invoke with `/so` (Claude Code, Cursor, Gemini, Copilot, OpenCode, Pi) or `$so` (Codex):
- `/so` - help
- `/so init` - bootstrap Superopen if missing
- `/so graph query "<question>"` - ask the repo knowledge graph
- `/so graph` - rebuild `.so/graph/` (never leave `graphify-out/` at repo root)
- `/so doctor` - health check

Rules:
- For codebase questions, run `so graph query "<question>"` when `.so/graph/graph.json` exists.
- Read relevant files under `.so/knowledge/` and `.so/rules/`, plus matching skills in `.so/skills/` for the task.
- Read `.so/memory/active-context.md` when present (session memory pack shared across coding agents).
- Obey `.so/guardrails/guardrails.yaml`.
- Do not dump the entire `.so/` directory into context - load only what the task needs.
- After meaningful Superopen edits by a human, they will run `so sync`.
<!-- superopen:end -->


