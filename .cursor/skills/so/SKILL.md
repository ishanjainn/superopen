---
name: so
description: "Superopen - Open source Agent Harness Engineering. Use when the user types /so or $so, wants to bootstrap a repo with so init, asks about architecture, or when .so/ exists - prefer graph query, context, rules, skills, and guardrails before broad Grep/Glob."
---

# /so

Superopen is **not** another coding agent. It is open source agent harness engineering around Claude Code, Cursor, Codex, Gemini, OpenCode, Copilot CLI, Pi, and similar tools: repository graph, context, rules, skills, guardrails, session evals, and recommendations - so each coding session wastes fewer tokens.

The `/so` skill is registered with **`so install`** (after installing the CLI). Agents can then run **`/so init`** to bootstrap a project - no prior `.so/` required.

## Usage

```
/so                                              # show this help (skill); CLI bare `so` = status snapshot
/so init                                         # bootstrap .so/ + upgrade with YOUR model (no API key)
/so init --force                                 # also overwrite existing heuristic templates
/so init --code-only                             # skip Graphify semantic/docs pass
/so graph                                        # rebuild repository graph into .so/graph/
/so graph query "<question>"                     # ask the local knowledge graph
/so query "<question>"                           # alias for graph query
/so knowledge                                     # show AGENTS.md + discovered rules/skills roots
/so rules                                        # point at this repo's rules dir (Cursor or .agents)
/so guardrails                                   # show .so/guardrails.yaml
/so skills                                       # list discovered non-/so skills tree
/so doctor                                       # health check
/so sync                                         # refresh injectors + graph after Superopen edits
/so install                                      # (re)register this /so skill with coding agents
/so uninstall                                    # remove skills, hooks, injectors, .so/, CLI
/so apply-upgrade                                # apply JSON you produced (usually automatic after /so init)
/so harvest idle                                 # harvest long-idle sessions into memory + native docs
/so dev                                          # open the file-backed Sessions UI
/so sessions                                     # list sessions
/so memory search "<task>"                       # compact local recall; add --id to expand selected evidence
/so eval <session-id>                            # score a session
/so recommend list                               # pending Superopen recommendations
/so recommend apply <id> --reason "…"          # resolve with an agent explanation
/so recommend dismiss <id> --reason "…"        # dismiss with durable feedback
```

Codex users: prefer `$so …` instead of `/so …`.
Gemini / Copilot / OpenCode / Pi: use `/so …` (or the vendor’s skill invoke if `/so` is not bound).

CLI flags for agents: `so --json …`, `so --full …` (or `SO_JSON=1` / `SO_FULL=1`). Prefer `--json` when parsing. Applying, dismissing, or reverting a recommendation requires `--reason`; explain what changed or why the recommendation is wrong. Empty results print `0 <kind>`. Guardrails are a **single** file: `.so/guardrails.yaml`.

## What You Must Do When Invoked

If the user invoked `/so`, `/so --help`, `/so -h`, or `$so` with no other arguments: print the **Usage** section above and stop.

### `/so init` - Graphify-style (assistant model = LLM)

**Do not ask the user for an API key.** Code/graph mapping is local; harness doc upgrade uses **your** model (same pattern as `/graphify`).

1. Ensure `so` is on PATH (`which so` or `$(go env GOPATH)/bin/so`).
2. Run heuristic bootstrap (no headless API):

```bash
so init --no-llm
```

   (Add `--force` / `--code-only` if the user passed those flags.)

3. Run `so upgrade-brief` to print the system instructions and repository profile without creating a persistent artifact. The profile includes **Automation candidates** (MCP servers, skills, guardrails) derived from stack signals after the graph build.
4. **Using your own model**, produce the JSON object described in that brief (`architecture_md`, `conventions_md`, `guardrails`, `evals`, `brief`, optional `mcp`, optional `skills`). Stay faithful to the profile - invent no secrets, env blocks, or fake paths. Pick the top 1–2 MCP and 1–2 skills from the candidates list; pin package versions (never `@latest`); never recommend Memory MCP (Superopen memory already covers recall). Do **not** reuse upgrade JSON from another project.
5. Apply it with **exactly one** input method (never both — the CLI errors if a path and a heredoc/stdin redirect are combined):

```bash
so apply-upgrade <<'EOF'
{ ... your JSON ... }
EOF
```

   Or write a temp file and `so apply-upgrade /tmp/so-upgrade.json` (no heredoc on that command).

6. Tell the user briefly: Superopen at `.so/`, graph node/edge counts, and that **AGENTS.md**, guardrails, evals, optional **mcp** (`.so/config.yaml` + projected `.mcp.json` / `.cursor/mcp.json`), and project skills were upgraded with the assistant model. Suggest `so doctor`, `so sync`, or `so dev` next. Leftover high-signal skills/guardrails may appear under `so recommend list` for HITL.

If `.so/` already exists and the user only wanted a refresh of context/guardrails/automations, skip step 2’s full rebuild when possible: run `so upgrade-brief`, then steps 4-5 (apply merges MCP servers additively).

### Bootstrap - no `.so/` yet (any `/so` task)

If `.so/` does **not** exist and the user did not say `init`, still run the **`/so init`** flow above before continuing their task.

### Fast path - existing Superopen + codebase question

Before broad exploration:

1. If `.so/graph/graph.json` exists and the user asked a natural-language question about the codebase (how X works, what calls Y, where is Z) **and** did not ask to rebuild: run:

```bash
so graph query "<question>"
```

Prefer that answer over Grep/Glob when it is useful. You may still open a few specific files the query surfaces.

2. For conventions / review rules, read `AGENTS.md` (and nested `*/AGENTS.md`), the active vendor's rules dir, and `.so/guardrails.yaml` before inventing process. When updating guidance, prune obsolete lines — do not only append.

3. Search memory compactly with `so memory search "<task>" --vendor <vendor>` when prior project experience may matter. Expand only a selected result with `--id <fingerprint> --vendor <vendor>`. Cursor cannot receive same-turn prompt context from its hook, so Cursor sessions should use this guided search for task-specific recall; inspect the full Session view only when compact evidence is insufficient.

### Commands → shell

Map `/so …` arguments to the `so` CLI on PATH (or `$(go env GOPATH)/bin/so` if needed):

| User says | Run |
|---|---|
| `/so init …` | Follow **`/so init`** section above (not bare `so init` alone) |
| `/so install …` | `so install …` |
| `/so apply-upgrade` | `so apply-upgrade` (with JSON you produced — file **or** heredoc, not both) |
| `/so graph` / `/so graph rebuild` | `so graph rebuild` |
| `/so graph query "…"` / `/so query "…"` | `so graph query "…"` |
| `/so doctor` | `so doctor` |
| `/so sync` | `so sync` |
| `/so dev` | `so dev` (background OK) |
| `/so guardrails` | `so guard` or read `.so/guardrails.yaml` |
| `/so skills` | list the discovered skills tree (not the reserved `/so` skill) |
| `/so knowledge` | read `AGENTS.md` (root + nested) relevant to the task |
| `/so rules` | summarize coding rules in the discovered rules dir |
| `/so sessions` | `so sessions` |
| `/so memory search …` | `so memory search …` |
| `/so eval …` | `so eval …` |
| `/so recommend …` | `so recommend …` |

### Headless / CI only

API keys (`OPENAI_API_KEY`, etc.) are **optional and never selected from ambient environment alone**. Session evals default to **`evals.backend: auto`**: the same vendor's sealed CLI when supported, another configured Claude/Codex CLI, an explicitly configured `llm:` backend, then offline **heuristics**.

```bash
so init --llm
```

Never prefer that path when you are an in-IDE coding agent.
