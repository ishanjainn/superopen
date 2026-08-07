<h1 align="center">Superopen</h1>

<p align="center">
  <img src="web/public/brand-wordmark.png" alt="SUPEROPEN" width="420" />
</p>

<h3 align="center">Open-source Agent Harness Engineering</h3>

Superopen is not another coding agent. Type `/so init` in Claude Code, Cursor, Codex, or a similar agent to build an open-source harness for shared memory and governance and make every coding session improve the next one with less token waste and lower cost.

## Get started in 30 seconds

### Install with Homebrew

```bash
brew install ishanjainn/superopen/so
```

### Other install methods

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/ishanjainn/superopen/main/scripts/install.sh | sh

# Windows PowerShell
iwr -useb https://raw.githubusercontent.com/ishanjainn/superopen/main/scripts/install.ps1 | iex
```

### Register with your coding agents

```bash
so install
```

This registers the `/so` skill with your installed coding agents.

### Initialize a repository

```bash
# In your coding agent, with your target repository open
/so init
```

Now ask a real question about the repository instead of making the next session rediscover it:

```text
/so graph query "how does auth work?"
/so memory write "Prefer the existing auth middleware pattern"
/so doctor
```

Run `so dev` from your terminal when you want the local session UI at `http://localhost:4444`.

## What it does

| Capability | What you get |
| --- | --- |
| Agent skills | A single `/so` skill across Claude, Cursor, Codex, Gemini, OpenCode, Copilot, Pi, and Agent Skills-compatible tools |
| Repository understanding | A graph built before the harness is seeded, so context starts from the codebase’s real structure |
| Shared memory | Preferences, lessons, and project context injected at session start across supported agents in the same repo |
| Session replay | OTLP telemetry materialized into sessions and replayable as a city map in the local UI |
| Evals and recommendations | Post-session scoring that proposes targeted improvements; sensitive changes can require approval |
| Local-first workflow | `.so/` is ordinary repository data that both the CLI and UI read and write directly |
| Agent-first CLI | Compact output, JSON envelopes, clear empty states, and useful next-step hints |

## See the loop

```text
agent session
    ↓ OTLP traces
.so/sessions/<id>/
    ↓ evaluate
recommendations
    ↓ approve or auto-apply safe changes
harness updates
    ↓
next agent session starts with better context
```

Superopen can enrich evaluations with a signed-in Claude Code or Codex CLI, an API key or gateway, or offline heuristics. It falls back to heuristics when no model backend is available.

---

## Install

Install the CLI, then run `so install`. The binary alone is not enough: agents need the `/so` skill file in a discovery location.

```bash
brew install ishanjainn/superopen/so
so install
```

`so install` registers the skill in the supported global and project discovery paths:

| Agent | Invocation |
| --- | --- |
| Claude Code | `/so` |
| Cursor | `/so` |
| Codex | `$so` |
| Gemini, OpenCode, Copilot CLI, Pi | `/so` |
| Generic Agent Skills | `/so` |

It writes skills only; it does not create a `.so/` harness. Run `/so init` (or `$so init` in Codex) in a repository to bootstrap one.

## Initialize a project

```bash
# In a coding agent—the preferred path, using that agent’s model
/so init

# Codex
$so init

# Shell or CI: heuristic seed and an assistant-ready upgrade brief
so init --no-llm

# Headless API-key upgrade, intended for CI/automation
so init --llm
```

Initialization:

1. Creates `.so/` and its configuration.
2. Builds the repository graph first.
3. Reads existing instruction files such as `AGENTS.md`, `CLAUDE.md`, and Cursor rules.
4. Seeds context, skills, rules, guardrails, and evals heuristically—offline and without an API key.
5. Writes `.so/upgrade-brief.md` for an assistant-driven improvement pass.
6. Builds the city map, enables configured OTLP hooks, and injects always-on project instructions.

For unattended automation, configure `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `OPENROUTER_API_KEY`, or `OPENAI_BASE_URL`, then run `so init --llm`. You can also set `llm.provider`, `llm.model`, and `llm.base_url` in `.so/config.yaml`.

---

## How agents use it

After `so install`, use the skill directly:

```text
/so
/so init
/so graph query "how does auth work?"
/so graph
/so doctor
```

The CLI is designed for agents as well as humans:

- Compact text output by default, with counts and next-step hints.
- `--json` / `SO_JSON=1` for structured `ok`, `kind`, `items`, and `data` envelopes.
- `--full` / `SO_FULL=1` to disable field truncation.
- Exit codes: `0` success, `1` failure, `2` usage, `3` not found.

Guardrails live in one inspectable file: `.so/guardrails/guardrails.yaml`.

## Evals and recommendations

When a session ends (`so eval` or finalization), Superopen scores it and may propose harness updates.

| `evals.backend` | Behavior |
| --- | --- |
| `auto` (default) | Claude/Codex CLI on `PATH`, then API key, then heuristics |
| `agent_cli` | Claude Code or Codex only (`evals.agent_cli`: `auto`, `claude`, or `codex`) |
| `llm_api` | API key or gateway only |
| `heuristics` | Offline scoring only |

The coding-agent CLI login is reused when available, so a normal interactive setup does not require a separate API key. Cursor has no sealed headless judging CLI; use it for interactive `/so init` upgrades and Claude Code or Codex for backend judging.

---

## CLI

| Command | Purpose |
| --- | --- |
| `so` | Harness status snapshot |
| `so install` | Register `/so` with coding agents |
| `so init` | Bootstrap the harness and install observability hooks |
| `so sync` | Refresh injectors, graph, and city map |
| `so dev` | Run the lightweight Next.js UI on `:4444` (`-d` to detach, `stop` to stop) |
| `so graph query` | Query the repository graph |
| `so sessions` | List, finalize, and demo sessions |
| `so sessions port` | Port chats across Claude, Codex, OpenCode, Cursor, and `.so` |
| `so eval` | Score a session |
| `so recommend` | List, apply, or dismiss recommendations |
| `so memory` | Write, store, search, and inject workspace memory |
| `so retrieve` | Search the harness corpus index |
| `so guard show/check` | Inspect or check guardrail policy |
| `so audit` | View the SEL-style audit trail |
| `so open` | Open a UI deep link when `so dev` is running |
| `so doctor` | Run health checks |

For the full data model and CLI/UI parity, see [the `.so/` schema](docs/so-schema.md).

## Uninstall

Run `so uninstall` before removing the binary. Package managers remove the executable but do not know about the skills, hooks, and injected instructions that Superopen registered.

```bash
so uninstall --dry-run     # preview changes
so uninstall               # remove skills, injectors, hooks, .so/, and the binary

# If Homebrew installed the binary
so uninstall --keep-binary
brew uninstall so
```

| Flag | Effect |
| --- | --- |
| `--dry-run` | Print the plan without changing anything |
| `--keep-binary` | Leave the `so` executable installed |
| `--keep-harness` | Leave the current repository’s `.so/` directory |

To remove hooks only: `so coding uninstall --vendor=all`. Add `--purge` to also remove shared Superopen configuration and the session-state cache. Shared agent configuration is edited surgically, so other tools’ entries are preserved.

If the binary is already gone, rebuild from a checkout and run `make build && ./bin/so uninstall`.

## Project layout

| Path | Role |
| --- | --- |
| `cmd/so/` | CLI entrypoint |
| `web/` | Local Next.js UI |
| `internal/` | Harness, OTLP, evals, coding hooks, and shared Go packages |
| `templates/` | Seed content for project knowledge, skills, rules, guardrails, and evals |
| `plugins/` + `sdk/` | Coding-agent hooks and their OTLP bootstrap—not a general LLM SDK |

Session 3D replay lives in `web/src/map`.

## Attribution, contributing, and security

Repository graphs are powered by [Graphify](https://github.com/Graphify-Labs/graphify).

Superopen is Apache-2.0 licensed; see [LICENSE](LICENSE) and [NOTICE](NOTICE). Contributions are welcome—start with [CONTRIBUTING.md](CONTRIBUTING.md), then read [SECURITY.md](SECURITY.md) and the [Code of Conduct](CODE_OF_CONDUCT.md) before opening an issue or pull request.

| Workflow | What it runs |
| --- | --- |
| `ci-cli.yml` | `go vet`, `go test -race`, cross-compile, plugin-sync drift checks |
| `ci-web.yml` | TypeScript, ESLint, Vitest, Next build |
| `release-cli.yml` | Tagged `cli-X.Y.Z` binary release |
