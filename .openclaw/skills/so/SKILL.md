---
name: so
description: "Superopen agent-harness operations. Use for explicit /so or $so commands, Superopen install/init/sync/diagnostics, and codebase architecture, dependency, impact, or data-flow questions when a Superopen graph is present."
---

# /so

Superopen is **not** another coding agent. It is open source agent harness engineering around Claude Code, Cursor, Codex, Gemini, OpenCode, Copilot CLI, Pi, and similar tools: repository graph, context, rules, skills, guardrails, session evals, and recommendations - so each coding session wastes fewer tokens.

The `/so` skill is registered with **`so install`** (after installing the CLI). Agents can then run **`/so init`** to bootstrap a project - no prior `.so/` required.

## Usage

`/so ...` below is **chat skill syntax**. Inside Bash, a shell, or a terminal, always use `so ...` with no leading slash. Never execute `/so ...` as a filesystem path.

```
/so                                              # show this help (skill); CLI bare `so` = status snapshot
/so init                                         # bootstrap .so/ + full Graphify graph with YOUR model
/so init --force                                 # also overwrite existing heuristic templates
/so init --code-only                             # skip Graphify semantic/docs pass
/so graph                                        # rebuild repository graph into .so/graph/
/so graph query "<question>"                     # ask the local knowledge graph
/so graph path "<A>" "<B>"                       # connection between two graph nodes
/so graph explain "<node>"                       # focused neighborhood and evidence
/so graph affected "<node>"                      # downstream impact before an edit
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
/so review-brief [session-id]                    # print pending session-review prompt (no API key)
/so apply-review [session-id]                    # apply reviewer JSON you produced (file OR stdin, not both)
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
2. Start the full graph build with the host-agent continuation protocol:

```bash
so --json init --agent --no-llm
```

   (Add `--force` / `--code-only` if the user passed those flags.) Exit code 4 is a normal `continuation_required` result, not a failed initialization. Read the run id from its hint.

3. When semantic work is required, inspect status and obtain the exact prompts shipped by the pinned Graphify runtime:

```bash
so --json graph semantic status --run RUN_ID
so --json graph semantic briefs --next --run RUN_ID
```

   Process exactly the numbered brief returned, reading only its listed files and returning only Graphify extraction JSON. Apply it with `so graph semantic apply --run RUN_ID --chunk N`, then request `briefs --next` again until none remain; retry an invalid chunk at most twice. Then run `so graph semantic finalize --run RUN_ID`. The run is durable: after interruption, use `semantic status` and continue with only the next unfinished chunk. Never fabricate a partial graph, bypass a failed chunk, discard a pending run, or retry with `--code-only` unless the user explicitly requested the matching discard option. If finalize/labels/publish fails, report the resumable run id instead of claiming initialization completed.

4. Run `so graph labels brief --run RUN_ID`, produce the requested JSON map with one concise label per community, apply it with `so graph labels apply --run RUN_ID`, then atomically publish with `so graph publish --run RUN_ID`. Rerun `so init --agent --no-llm`; it will reuse the ready graph and continue harness setup. A `--code-only` request skips this semantic continuation but still produces a real Graphify AST graph.

5. Run `so upgrade-brief` to print the system instructions and repository profile. Using your own model, produce the JSON object described in that brief (`architecture_md`, `conventions_md`, `guardrails`, `evals`, `brief`, optional `mcp`, optional `skills`). Stay faithful to the profile - invent no secrets, env blocks, or fake paths. Pick the top 1–2 MCP from the candidates list; pin package versions (never `@latest`); never recommend Memory MCP. Include skills only for concrete repo-specific workflows.
6. Apply it with exactly one input method:

```bash
so apply-upgrade <<'EOF'
{ ... your JSON ... }
EOF
```

   Or write a temp file and `so apply-upgrade /tmp/so-upgrade.json` (no heredoc on that command).

7. Tell the user briefly: Superopen at `.so/`, graph node/edge counts, and that **AGENTS.md**, guardrails, evals, optional MCP, and any repo-specific project skills were upgraded with the assistant model. Suggest `so doctor`, `so sync`, or `so dev` next.

If `.so/` already exists and the user only wanted a refresh of context/guardrails/automations, skip step 2’s full rebuild when possible: run `so upgrade-brief`, then steps 4-5 (apply merges MCP servers additively).

### Review - live-agent session review (no API key)

Only review a **completed prior session with repository source edits and sufficient evidence**. Bootstrap, graph-only, sync-only, harness-only, and no-source-edit sessions are ineligible. A pending review never takes precedence over the user's current task merely because a tool-count threshold was crossed.

1. Run `so review-brief <session-id>` (default: previous pending same-vendor session).
2. If it says status is `complete` or `running`, skip apply-review and continue the user's task.
3. Using your own model, produce the JSON object from that brief (dimensions, findings with `proposed_body`, memory). Prefer empty findings over weak advice. Do **not** ask for an API key.
4. Apply with **exactly one** input method (never both):

```bash
so apply-review <session-id> <<'EOF'
{ ... your JSON ... }
EOF
```

   Or `so apply-review <session-id> /tmp/so-review.json` (no heredoc on that command).

5. Apply eligible review work after completing the user's requested task. Soft recs may auto-apply; guardrails, eval-policy, removals, and restructures stay pending for `so recommend apply`.

Skip if review status is already complete or running. Cursor often cannot consume hook `additional_context` same-turn — this skill is the reliable path.

### Bootstrap - no `.so/` yet (any `/so` task)

If `.so/` does **not** exist and the user did not say `init`, still run the **`/so init`** flow above before continuing their task.

### Fast path - existing Superopen + codebase question

Before broad exploration:

1. If `.so/graph/graph.json` exists and the user asked a natural-language question about the codebase (how X works, what calls Y, where is Z) **and** did not ask to rebuild: run:

```bash
so graph query "<question>" --budget 1000 --depth 1
```

Use the original natural-language question first. If it returns no match or ambiguous seeds, search the compact token list in `.so/graph/cache/vocab.txt` and retry with at most 12 exact `--term` values; those terms augment rather than replace the question and are emitted for auditability. Use BFS query for neighborhood/orientation questions, DFS or `path` for chains, `explain` for one concept, and `affected` before changing a dependency. Start at depth 1 with an 800–1200 token budget; widen only when output reports unresolved truncation. Prefer that answer over broad Grep/Glob, then open the surfaced source locations and verify them against source before editing.

When delegating codebase exploration to subagents, include the same graph-first orientation requirement and graph hash in their prompt; do not make each worker rediscover the repository broadly.

If graph state reports `pending_semantic_run_id`, resume only unfinished chunks with `so graph semantic status/briefs`; the previously published graph remains usable until atomic publication.

After a materially useful, dead-end, or corrected graph result, record the outcome explicitly with `so graph result save`; ordinary queries are not automatically useful. Run `so graph update` after edits that change source relationships. `so graph reflect` derives graph lessons without mutating structural graph nodes or edges.

At a true SessionEnd, Superopen's detached finalizer refreshes the graph before evaluating the completed session. Vendors without a reliable end event get the same idempotent maintenance at the next SessionStart. If semantic files changed, continue the injected semantic run; do not start a competing rebuild. Evaluation/review work remains after the user's current task.

2. For conventions / review rules, read `AGENTS.md` (and nested `*/AGENTS.md`), the active vendor's rules dir, and `.so/guardrails.yaml` before inventing process. When updating guidance, prune obsolete lines — do not only append.

3. Search memory compactly with `so memory search "<task>" --vendor <vendor>` when prior project experience may matter. Expand only a selected result with `--id <fingerprint> --vendor <vendor>`. Cursor cannot receive same-turn prompt context from its hook, so Cursor sessions should use this guided search for task-specific recall; inspect the full Session view only when compact evidence is insufficient.

### Commands → shell

Map `/so …` arguments to the `so` CLI on PATH (or `$(go env GOPATH)/bin/so` if needed):

| User says | Run |
|---|---|
| `/so init …` | Follow **`/so init`** section above (not bare `so init` alone) |
| `/so install …` | `so install …` |
| `/so apply-upgrade` | `so apply-upgrade` (with JSON you produced — file **or** heredoc, not both) |
| `/so review-brief …` | `so review-brief …` |
| `/so apply-review …` | `so apply-review …` (JSON you produced — file **or** heredoc, not both) |
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

If `so sync` returns exit code 4, treat it as durable semantic continuation work: use the run id from its hint and complete briefs/apply/finalize/labels/publish. Never replace that continuation with a direct AST-only rebuild.

### Headless / CI only

API keys (`OPENAI_API_KEY`, etc.) are **optional and never selected from ambient environment alone**. Session evals default to **`evals.backend: auto`**: the **live coding agent** reviews first (`so review-brief` / `so apply-review` on the next same-vendor SessionStart). Sealed `claude`/`codex`/`opencode`/`pi` CLI runs only after a true chat close (or idle) if still pending. Never finish a review with heuristics while a live agent or CLI can review. Set `evals.backend: heuristics` only for explicit offline judging.

```bash
so init --llm
```

Never prefer that path when you are an in-IDE coding agent.
