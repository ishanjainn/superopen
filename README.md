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
| Session replay | Hook telemetry stored with each session and replayable as a session map in the local UI |
| Evals and recommendations | Post-session scoring that proposes targeted improvements; sensitive changes can require approval |
| Local-first workflow | `.so/` is ordinary repository data that both the CLI and UI read and write directly |
| Agent-first CLI | Compact output, JSON envelopes, clear empty states, and useful next-step hints |

## See the loop

```text
agent session
    ↓ local hook events
.so/sessions/<id>/
    ↓ evaluate
recommendations
    ↓ approve or auto-apply safe changes
harness updates
    ↓
next agent session starts with better context
```

Superopen reviews sessions with the **live coding agent** first (`so review-brief` / `so apply-review`). A signed-in `claude`, `codex`, `opencode`, or `pi` CLI is a fallback after a true SessionEnd or idle. Heuristics do not complete reviews under `evals.backend: auto`.

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

# Shell or CI: compact heuristic seed
so init --no-llm

# Headless API-key upgrade, intended for CI/automation
so init --llm
```

Initialization:

1. Creates `.so/` and its configuration.
2. Builds the repository graph first.
3. Reads existing instruction files such as `AGENTS.md`, `CLAUDE.md`, and Cursor rules.
4. Seeds shared `AGENTS.md`, `.so/guardrails.yaml`, and `.so/evals.yaml` heuristically—offline and without an API key.
5. Enables only detected or explicitly requested vendor integrations; `.agents` requires `--shared-agents`.
6. Prints an assistant upgrade prompt on demand with `so upgrade-brief` without persisting an artifact.

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

Guardrails live in one inspectable file: `.so/guardrails.yaml`.

## Evals and recommendations

When a session closes, Superopen materializes it and leaves review **pending** until a model reviewer finishes. The next same-vendor SessionStart live agent runs `so review-brief` / `so apply-review`. A sealed CLI (`claude`, `codex`, `opencode`, or `pi`) may run on a true SessionEnd or idle if `evals.cli_fallback` is true.

| `evals.backend` | Behavior |
| --- | --- |
| `auto` (default) | Live agent (next SessionStart) → sealed CLI on true close/idle. Heuristics do **not** complete reviews. |
| `agent_cli` | Sealed CLI only (`evals.agent_cli`: `auto`, `claude`, `codex`, `opencode`, or `pi`) |
| `llm_api` | Explicit API key or gateway only |
| `heuristics` | Offline scoring only (no model tokens) |

| Setting | Default | Meaning |
| --- | --- | --- |
| `evals.live_agent` | `true` | Next SessionStart reviews the previous pending same-vendor session |
| `evals.cli_fallback` | `true` | Spawn sealed CLI after true SessionEnd or idle (never Stop) |
| `evals.mid_session` | `false` | Once in the same open chat after considerable work (`mid_session_min_edits` 3 or `mid_session_min_tools` 10) |

Set `heuristics` only when you want zero judging cost. `live_agent: false` is CLI-only (or pending if no CLI). `cli_fallback: false` waits for a live agent.

---

## CLI

| Command | Purpose |
| --- | --- |
| `so` | Harness status snapshot |
| `so install` | Register `/so` with coding agents |
| `so init` | Bootstrap the harness and install observability hooks |
| `so sync` | Refresh injectors, graph, and session map; exit 4 resumes agent-semantic work when required |
| `so dev` | Run the lightweight Next.js UI on `:4444` (`-d` to detach, `stop` to stop) |
| `so graph query` | Query the repository graph |
| `so sessions` | List, finalize, and demo sessions |
| `so sessions port` | Port chats across Claude, Codex, OpenCode, Cursor, and `.so` |
| `so review-brief` | Print the live-agent session-review prompt |
| `so apply-review` | Apply live-agent reviewer JSON |
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
| `internal/` | Harness, session telemetry, evals, coding hooks, and shared Go packages |
| `templates/` | Seed content for project knowledge, skills, rules, guardrails, and evals |
| `plugins/` + `sdk/` | Coding-agent hooks and their local telemetry bootstrap—not a general LLM SDK |

Session 3D replay lives in `web/src/map`.

## Attribution, contributing, and security

Repository graphs are powered by the exact pinned [Graphify 0.9.45](https://github.com/Graphify-Labs/graphify/releases/tag/v0.9.45) runtime. `so install` creates an isolated Python 3.12 environment with every platform-compatible `graphifyy` extra (`all` on Windows; the equivalent set excluding Windows-only DreamMaker elsewhere); `so graph …` is the supported façade. Superopen never substitutes a stub graph when the runtime or extraction fails.

Graph builds exclude `.so/**` completely. Code plus Markdown-family document structure is extracted deterministically without model tokens. Incremental code and structural-document edits refresh locally; opaque documents and media queue a resumable coding-agent semantic continuation while the last valid graph stays queryable. Agents start with the original question and a focused 800–1200 token graph query; compact vocabulary terms are an audited fallback for ambiguous matches.

Release cost/effectiveness claims use the opt-in [model-neutral paired lifecycle gate](docs/haiku-release-gate.md). It reports initialization separately, measures cost per successful result and break-even over repeated sessions, and does not label Graphify's ERPNext question benchmark as SWE-bench.

Superopen is Apache-2.0 licensed; see [LICENSE](LICENSE) and [NOTICE](NOTICE). Contributions are welcome—start with [CONTRIBUTING.md](CONTRIBUTING.md), then read [SECURITY.md](SECURITY.md) and the [Code of Conduct](CODE_OF_CONDUCT.md) before opening an issue or pull request.

| Workflow | What it runs |
| --- | --- |
| `ci-cli.yml` | `go vet`, `go test -race`, cross-compile, plugin-sync drift checks |
| `ci-web.yml` | TypeScript, ESLint, Vitest, Next build |
| `ci-cross-platform.yml` | Native CLI and file-backed UI tests on Linux, macOS, and Windows |
| `ci-automation.yml` | Workflow linting and release-automation validation |
| `release-cli.yml` | Tagged `cli-X.Y.Z` binary release |
| `release-packages.yml` | Coordinated package release automation |
