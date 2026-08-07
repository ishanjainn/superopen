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
/so knowledge                                     # point at .so/knowledge to read for the current task
/so rules                                        # point at .so/rules for coding guidance
/so guardrails                                   # show .so/guardrails/guardrails.yaml
/so skills                                       # list .so/skills/
/so doctor                                       # health check (warns OK; otlp needs `so dev`)
/so sync                                         # refresh injectors + graph after Superopen edits
/so install                                      # (re)register this /so skill with coding agents
/so uninstall                                    # remove skills, hooks, injectors, .so/, CLI
/so apply-upgrade                                # apply JSON you produced (usually automatic after /so init)
/so harvest idle                                 # harvest long-idle sessions into memory/knowledge
/so dev                                          # open Sessions UI + OTLP receiver
/so sessions                                     # list sessions
/so eval <session-id>                            # score a session
/so recommend list                               # pending Superopen recommendations
/so recommend apply <id> --reason "…"          # resolve with an agent explanation
/so recommend dismiss <id> --reason "…"        # dismiss with durable feedback
```

Codex users: prefer `$so …` instead of `/so …`.
Gemini / Copilot / OpenCode / Pi: use `/so …` (or the vendor’s skill invoke if `/so` is not bound).

CLI flags for agents: `so --json …`, `so --full …` (or `SO_JSON=1` / `SO_FULL=1`). Prefer `--json` when parsing. Applying, dismissing, or reverting a recommendation requires `--reason`; explain what changed or why the recommendation is wrong. Empty results print `0 <kind>`. Guardrails are a **single** file: `.so/guardrails/guardrails.yaml`.

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

3. Read `.so/upgrade-brief.md` (written by init). It contains the system instructions + repo profile.
4. **Using your own model**, produce the JSON object described in that brief (architecture_md, conventions_md, guardrails, evals, brief). Stay faithful to the profile - invent no secrets or fake paths.
5. Apply it:

```bash
so apply-upgrade <<'EOF'
{ ... your JSON ... }
EOF
```

   Or write a temp file and `so apply-upgrade /tmp/so-upgrade.json`.

6. Tell the user briefly: Superopen at `.so/`, graph node/edge counts, and that context/guardrails were upgraded with the assistant model. Suggest `so doctor` or `so dev` next. Doctor may **warn** if OTLP is down (`so dev` starts it); that is not a failed init.

If `.so/` already exists and the user only wanted a refresh of context/guardrails, skip step 2’s full rebuild when possible: read `.so/upgrade-brief.md` (re-run `so init --no-llm` if missing), then steps 4-5.

### Bootstrap - no `.so/` yet (any `/so` task)

If `.so/` does **not** exist and the user did not say `init`, still run the **`/so init`** flow above before continuing their task.

### Fast path - existing Superopen + codebase question

Before broad exploration:

1. If `.so/graph/graph.json` exists and the user asked a natural-language question about the codebase (how X works, what calls Y, where is Z) **and** did not ask to rebuild: run:

```bash
so graph query "<question>"
```

Prefer that answer over Grep/Glob when it is useful. You may still open a few specific files the query surfaces.

2. For conventions / review rules, read `.so/knowledge/conventions.md`, `.so/rules/`, and `.so/guardrails/guardrails.yaml` before inventing process.

### Commands → shell

Map `/so …` arguments to the `so` CLI on PATH (or `$(go env GOPATH)/bin/so` if needed):

| User says | Run |
|---|---|
| `/so init …` | Follow **`/so init`** section above (not bare `so init` alone) |
| `/so install …` | `so install …` |
| `/so apply-upgrade` | `so apply-upgrade` (with JSON you produced) |
| `/so graph` / `/so graph rebuild` | `so graph rebuild` |
| `/so graph query "…"` / `/so query "…"` | `so graph query "…"` |
| `/so doctor` | `so doctor` |
| `/so sync` | `so sync` |
| `/so dev` | `so dev` (background OK) |
| `/so guardrails` | `so guard` or read `.so/guardrails/guardrails.yaml` |
| `/so skills` | `so skill list` or list `.so/skills/` |
| `/so knowledge` | summarize paths under `.so/knowledge/` relevant to the task |
| `/so rules` | summarize coding rules under `.so/rules/` |
| `/so sessions` | `so sessions` |
| `/so eval …` | `so eval …` |
| `/so recommend …` | `so recommend …` |

### Headless / CI only

API keys (`OPENAI_API_KEY`, etc.) are **optional**. Backend `so eval` / recommendations prefer a sealed **Claude Code or Codex CLI** on PATH (`evals.backend: auto`) - same login as interactive coding. API keys are a fallback for CI without those CLIs:

```bash
so init --llm
```

Never prefer that path when you are an in-IDE coding agent.
