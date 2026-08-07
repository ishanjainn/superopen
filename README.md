# Superopen

<p align="center">
  <img src="web/public/brand-wordmark.png" alt="SUPEROPEN" width="420" />
</p>

**Open source Agent Harness Engineering**

Superopen is not another coding agent. It builds the open source harness around Claude Code, Cursor, Codex, and similar tools so every coding session improves the next - with less token waste and lower cost.

```bash
# macOS / Linux
brew install ishanjainn/superopen/so
so install                 # enables /so (or $so) in Cursor, Claude, Codex, Gemini, OpenCode, Copilot, Pi

# Bootstrap (shell or agent):
so init                    # or /so init
so sync
so dev                     # light Next.js UI (Turbopack) on :4444 + OTLP
so dev -d                  # same, detached (background)
so dev stop                # stop detached / tracked UI
```

Other install methods: `curl -fsSL https://raw.githubusercontent.com/ishanjainn/superopen/main/scripts/install.sh | sh`, `go install ./cmd/so`, or on Windows `iwr -useb https://raw.githubusercontent.com/ishanjainn/superopen/main/scripts/install.ps1 | iex`.

The UI reads/writes `.so/` directly (Next App Router). Advanced Memory search, Port, Retrieve, and checkpoint create/restore shell out to the `so` CLI - keep it on PATH (or set `SUPEROPEN_SO_BIN`). Graph view needs Graphify HTML (`so graph` / Graphify); a stub `graph.json` alone is not enough for the Graph page.

## What `so install` does

Same idea as Graphify’s `graphify install`: the CLI binary alone is not enough - agents need a skill/command file.

`so install` writes the `/so` skill into every supported agent’s discovery path:

- `~/.agents/skills/so/` + project `.agents/skills/so/` (shared Agent Skills)
- `~/.cursor/skills/so/` + project `.cursor/skills/so/` (Cursor - this is `/so`; do not also add `.cursor/commands/so.md`)
- `~/.claude/skills/so/` + project `.claude/skills/so/` (skills only - no duplicate `.claude/commands/so.md`)
- `~/.codex/skills/so/` + project `.codex/skills/so/` (use `$so` in Codex)
- `~/.gemini/skills/so/` + project `.gemini/skills/so/`
- `~/.config/opencode/skills/so/` + project `.opencode/skills/so/`
- `~/.copilot/skills/so/` + project `.github/skills/so/` (Copilot CLI)
- `~/.pi/agent/skills/so/` + project `.pi/skills/so/`

No `.so/` harness is created by install (skills only). After `so install`, any of those agents can run **`/so init`** (or `$so init` in Codex) to bootstrap `.so/` + graph.

## What `so init` does

1. Creates `.so/` harness directories + config
2. **Builds the repository graph first** (understand structure before seeding)
3. **Reads existing agent instruction files** (`AGENTS.md`, `CLAUDE.md`, `.cursor/rules/*.mdc`, …)
4. Seeds context/skills/rules/guardrails/evals **heuristically** (works offline, no API key)
5. Writes `.so/upgrade-brief.md` for **assistant-driven upgrade** (Graphify-style - no API key). Headless `so init --llm` is optional for CI.
6. Builds a citymap for session Map replay
7. Enables OTLP observability hooks for configured vendors (Claude Code, Cursor, Codex, Gemini, OpenCode, Copilot CLI, Pi)
8. Injects always-on instructions into `AGENTS.md`, `CLAUDE.md`, `.cursor/rules/superopen.mdc`

```bash
# In any supported coding agent (preferred - uses the assistant model):
/so init                         # so init --no-llm → agent upgrades via .so/upgrade-brief.md → so apply-upgrade
# Codex: $so init

# Shell / CI:
so init --no-llm                 # heuristic seed + upgrade-brief only
so init --llm                    # headless API-key upgrade (fails without a key/gateway)
so init --force                  # also overwrite existing context/guardrails/evals templates
so init --code-only              # skip Graphify semantic/docs pass
```

### Headless LLM (CI / automation only)

Coding agents should **not** ask for API keys - `/so init` upgrades with the assistant model.
For scripts without an assistant:

```bash
export OPENAI_API_KEY=sk-...     # or ANTHROPIC_API_KEY / OPENROUTER_API_KEY / OPENAI_BASE_URL
so init --llm
```

You can also set `llm.provider`, `llm.model`, and `llm.base_url` in `.so/config.yaml`.

### Backend evals & recommendations

After a session ends (`so eval` / finalize), Superopen scores the session and may propose harness updates. Model enrichment reuses **coding-agent CLIs** when available (sealed `claude -p` / `codex exec` - your logged-in subscription, no extra API key):

| `evals.backend` | Behavior |
|---|---|
| `auto` (default) | Prefer Claude/Codex CLI on PATH → else API key → else heuristics |
| `agent_cli` | Claude Code / Codex only (`evals.agent_cli`: `auto` \| `claude` \| `codex`) |
| `llm_api` | API key / gateway only |
| `heuristics` | Offline scores only |

Cursor has no sealed headless CLI for judging; use it for interactive `/so init` upgrades. Backend judging uses Claude Code or Codex.

## Uninstall

**Run `so uninstall` before removing the binary.** Package managers only delete the executable - they know nothing about the skills, hooks, and injectors `so install` / `so init` wrote into your agent config directories. Remove the binary first and those are orphaned, pointing at a CLI that no longer exists.

```bash
so uninstall --dry-run     # preview everything that would be removed
so uninstall               # skills + injectors + hooks + .so/ + the binary itself
```

`so uninstall` removes the `so` binary too. Flags to keep parts of it:

| Flag | Effect |
|---|---|
| `--dry-run` | Print the plan, change nothing |
| `--keep-binary` | Leave the `so` executable in place |
| `--keep-harness` | Leave this repo's `.so/` directory in place |

If you installed via a package manager, keep the binary and let that manager remove it:

```bash
so uninstall --keep-binary
brew uninstall so
brew untap ishanjainn/superopen     # optional
```

### Hooks only

To strip coding-agent hooks without touching skills, `.so/`, or the binary:

```bash
so coding uninstall --vendor=all             # all vendors
so coding uninstall --vendor=cursor          # one vendor
so coding uninstall --vendor=all --purge     # also drop ~/.config/superopen + session-state cache
```

Leave `--purge` off if you plan to reinstall and want to keep your endpoint and API key. Shared files (`~/.cursor/hooks.json`, `~/.codex/config.toml`) are edited surgically - other tools' entries are preserved.

### Already removed the binary?

Rebuild from a checkout and run the uninstaller with it:

```bash
make build && ./bin/so uninstall
```

## Closed loop

Agent session → OTLP traces → post-session materialization (`.so/sessions/<id>/`) → Map replay from telemetry → auto eval → recommendations → soft tier may auto-apply; guardrails/evals need approve (or `require_approval: false`) → harness updates → next session is cheaper.

Backend enrichment uses sealed Claude Code / Codex CLI on PATH (models: `evals.models.claude` / `evals.models.codex`, defaults `claude-sonnet-5` / `gpt-5.6-luna`) even when the coding session was Cursor, OpenCode, Pi, or Gemini. Heuristics run if no CLI/API key.

After `git pull`, Superopen refreshes via `post-merge` / `post-checkout` hooks or `so refresh` (also while `so dev` watches). Full rebuild remains `so sync`.

Shared **memory** (`.so/memory/`) injects on every vendor SessionStart and via `so sessions start --vendor=…`, so Claude / Cursor / Codex (and other installed vendors) continue from the same prefs, lessons, and project context **within one repo**. Each clone has its own `.so/`; the projects registry lets one UI browse sessions across local clones. Consolidation prefers your coding-agent CLI login; API keys are optional.

CLI ↔ UI parity: both surfaces read/write `.so/` (see `docs/so-schema.md`). Recommendations Approve in the UI runs `so recommend apply` (writes proposed body, adds a lesson, `so sync --skip-graph`).

## AXI (Agent eXperience Interface)

CLI output is built for agents first:

- **Compact text** by default (rows + `count:` + `next:` hints)
- **`--json`** / `SO_JSON=1` - structured envelopes (`ok`, `kind`, `items`/`data`, `next`)
- **`--full`** / `SO_FULL=1` - disable field truncation
- **Definitive empty states** - `0 sessions` (never silent)
- **Exit codes** - `0` ok · `1` fail · `2` usage · `3` not found
- **Content-first root** - bare `so` shows a harness status snapshot (`so --help` for the full command list)

Guardrails live in **one file**: `.so/guardrails/guardrails.yaml` (advisory rules + denied commands / sensitive paths).

## Agent slash command (`/so`)

After **`so install`** (not only after `so init`):

```
/so
/so init
/so graph query "how does auth work?"
/so graph
/so doctor
```

## Layout

| Path | Role |
|---|---|
| `cmd/so/` | CLI entrypoint (`so`) |
| `web/` | Local Next.js UI (`so dev` → :4444) |
| `internal/` | Shared Go libraries (harness, OTLP, evals, coding hooks, …) |
| `templates/` | Seed content for `.so/knowledge`, skills, rules, guardrails |
| `plugins/` + `sdk/` | Coding-agent hooks + OTLP bootstrap for those hooks (not a general LLM SDK) |

Session 3D replay lives in `web/src/map`.

## CLI

| Command | Purpose |
|---|---|
| `so` | Status snapshot (AXI content-first) |
| `so uninstall` | Remove skills, hooks, injectors, `.so/`, and the CLI ([see Uninstall](#uninstall)) |
| `so install` | Register `/so` skill with coding agents (required after CLI install) |
| `so init` | Bootstrap harness + install coding-agent o11y hooks |
| `so coding install --vendor=all` | Install/refresh Claude/Cursor/Codex OTLP hooks |
| `so coding uninstall --vendor=all` | Strip OTLP hooks only (`--purge` also drops shared config) |
| `so coding hook` | Hot path invoked by agent hooks (do not run by hand) |
| `so sync` | Refresh injectors/graph/citymap |
| `so dev` | Light Next.js UI (:4444, Turbopack); `so dev -d` / `so dev stop` |
| `so graph query` | Query repository graph |
| `so sessions` | List/finalize/demo sessions |
| `so sessions port` | Port chats across Claude/Codex/OpenCode/Cursor/.so |
| `so sessions detect` / `verify` | Detect vendor stores / sample IR integrity |
| `so eval` | Score a session |
| `so recommend` | List/apply/dismiss recommendations |
| `so memory` | Workspace memory: Write→Store→Inject (`active-context.md` on SessionStart) |
| `so learn` | Capture corrections as lessons |
| `so sessions start` | Start a coding agent from shared `.so/memory` |
| `so retrieve` | Search harness corpus index |
| `so guard show/check` | Guardrails policy inspect / deny check |
| `so audit` | SEL-style audit trail |
| `so open` | Open UI deep-link when `so dev` is running |
| `so knowledge` | Knowledge helpers (`.so/knowledge`) |
| `so doctor` | Health checks |

## Attribution

Repository graphs powered by [Graphify](https://github.com/Graphify-Labs/graphify).

Apache-2.0 - see `LICENSE` and `NOTICE`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Please read [SECURITY.md](SECURITY.md) and the [Code of Conduct](CODE_OF_CONDUCT.md) before opening issues or PRs.

CI:

| Workflow | What it runs |
|---|---|
| `ci-cli.yml` | `go vet` · `go test -race` · cross-compile · plugin sync drift |
| `ci-web.yml` | `tsc` · `eslint` · Vitest · `next build` |
| `release-cli.yml` | Tagged `cli-X.Y.Z` binary release |
