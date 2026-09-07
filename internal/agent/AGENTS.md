# Agent harness

What `so install` writes into **user** agent config, not this repo's contributor `AGENTS.md` tree.

Parent index: [../../AGENTS.md](../../AGENTS.md).

## Installed surfaces

| Surface | Source |
|---------|--------|
| `/so` skill | `skills/so/SKILL.md` |
| References | `skills/so/references/` |
| Durable block | `steer/block.go` → user `CLAUDE.md` (`$HOME/.claude` and `$CLAUDE_CONFIG_DIR` when set) |
| Cursor rule (install) | `steer.CursorRule()` → user `~/.cursor/rules` |
| Cursor rule (opt-in) | `so init --cursor-rules` → `<repo>/.cursor/rules/superopen.mdc` |
| Hooks | `plugins/*/hooks/hooks.json`, `hook/` |

Install: `install/`, `steer/install.go`.

## Product harness contract

When `.so/` exists in a **customer repo**, every supported coding vendor gets the same three outcomes (host-legal apply may differ):

| Outcome | What happens |
|---------|----------------|
| **Graph** | Agent is told to run `so graph query` before grep/read |
| **Observability** | `so sessions hook` records session start/end, user prompts, tool calls, assistant turns into `.so/sessions` |
| **Silent lifecycle** | SessionEnd / sessionEnd detach `so sessions finalize` (no extra model text). Finalize is single-flight per session: duplicate sessionEnd or both `sessions hook` + `sessions finalize --detach` must not spawn a second harvest/distill worker. Prompt-submit is silent unless the prompt is prior-work/personal (or the workspace is a memory store with little source), **or** harvest/distill is pending. Then inject the live `brief`/`propose`/`skip` (or distill `--brief`/`--apply`) line once. Stop stays silent. PostToolUse / postToolUse may inject a blast-radius line after an edit when unedited dependents exist (fail-open, at most 3 per session). SessionStart is one route only: graph one-liner when the repo has source, or N-memories + recall command when live diary rows exist and there is little/no source. Harvest OPEN review stays on-demand (`references/harvest.md`). A pending harvest/distill one-liner may append after the graph or memory line. SessionEnd uses the session vendor's own one-shot CLI when authenticated; otherwise work stays pending for the next live agent. There is no cross-vendor fallback. Headless distill/harvest workers set `SUPEROPEN_HEADLESS=1` and must not be recorded as sessions. Only explore-tool nudges (and SubagentStart where the host has subagents). Skill / `AGENTS.md` / `SKILL.md` Reads are not “skipped graph”. |

`so sessions hook` is a **host-protocol exception**: stdout is vendor control JSON (`additionalContext` / `permissionDecision`), never AXI TOON/`help[]`/dashboards. Telemetry logs go to stderr. Always exit 0 on telemetry-path failure. The command is Hidden under `so sessions` so the user AXI catalog stays list/show/finalize. Users install with `so install`.

| Vendor | Graph-first channel | Notes |
|--------|---------------------|-------|
| Claude Code | PreToolUse JSON (`SearchNudge` / `ReadNudge`) | Eval host |
| Cursor | `preToolUse` / `beforeReadFile` JSON | Same nudges |
| Gemini | `BeforeTool` `additionalContext` | Same nudges |
| Copilot CLI | bash + powershell hook commands | Same nudges |
| Codex | Durable `AGENTS.md` + skill only | Desktop **rejects** PreToolUse `additionalContext`. Do not emit it |
| OpenCode | Hook stdout → one-shot `echo "<nudge>" ; <command>` on bash | `;` not `&&` (Windows PowerShell 5.1). Non-bash tools: telemetry + AGENTS.md |
| Pi | Native `graph_*` tools + AGENTS.md; bash echo rewrite when `command` is mutable | Do not rewrite `graph_*` tools |

OS-neutral: spawn `so` / `so.exe` with argv (never `shell: true`). Windows install pins `so.exe` via `patchPluginSoBin`.

When `.so/` exists:

1. **Codebase questions (repo has source):** `so graph query "<question>"` via Bash. Superopen has no MCP and must not grow one.
2. **Explore-tool nudge:** once per session (`SearchNudge` / `ReadNudge`) before a graph query; after a successful query, Grep stays silent, source Read gets a once-per-session snippet overflow, and a second `graph query` gets a once-per-session snippet overflow (re-query only if TRUNCATED). Skip when the prompt is personal/memory (`MemoryNudge` instead). Codex PreToolUse stays empty.
3. **No ExploreAugment** on Grep/Read (`graphGate`)
4. **Follow-ups:** if NODE/EDGE lines or attached BODIES answer, stop. Need another body: `so graph snippet` of a listed NODE. Do not list the tree to confirm Superopen. Grep/Read only for a literal the graph does not index. After TRUNCATED, narrow first. No search spray.
5. **SubagentStart:** `HookReminder` or `MemoryHookReminder` (hosts that have subagents)
6. **SessionStart:** graph one-liner **or** N-memories + `so memory recall` (never both encyclopedias). **Prompt-submit** (UserPromptSubmit / beforeSubmitPrompt / BeforeAgent / before_agent_start): silent unless memory-shaped / personal cue (then matching recalled bodies). **PostToolUse / Stop / SessionEnd:** silent for steer text

Optional: `so install --strict` / `SUPEROPEN_HOOK_STRICT`

## Session contracts (S)

| ID | Contract |
|----|----------|
| **S1** | Read/Edit/Write emit `coding_agent.file_path` (repo-relative). Shell commands are never stamped. Materialize and ingest share that field and re-validate it. Tool-arg JSON is a fallback. |
| **S2** | `SessionCost` counts usage once (session-root, else latest `loop.stop`). No usage spans → tokens=0. Never invent tokens from character counts. Model family prefix-matching prices `grok-4.6*`. |
| **S3** | VCS revision attrs are not redacted. Prompts and tool args still are. |
| **S4** | Finalize must not replace a longer `events.jsonl` with a shorter Query. `sessionEnd` is Cursor close; `loop.stop` is not end-of-chat. Duplicate thoughts / extra `read_file`+Read are storage-only dedup. |

## Repo vs installed skill

| | Contributor `AGENTS.md` here | Installed `/so` skill |
|--|------------------------------|------------------------|
| Audience | Superopen developers | Customer repo agents |
| Trigger | Editing this checkout | `.so/` in cwd |

## Tests

```bash
go test ./internal/agent/hook/ ./internal/agent/steer/ ./internal/agent/skills/ ./internal/agent/install/ -count=1
```

Benchmarks: [../../benchmarks/agent-graph-eval/AGENTS.md](../../benchmarks/agent-graph-eval/AGENTS.md)

## Change checklist

- [ ] `skills/so/SKILL.md`: query-first tripwire unless intentional.
- [ ] Memory only in `references/memory.md`. Harvest only in `references/harvest.md`.
- [ ] Nudges: no MANDATORY; no `so graph search` in default hook text. CLI/Bash once in Block/skill.
- [ ] No ExploreAugment on live Grep/Read path.
- [ ] `Block()` / `CursorRule()`: no `--json` in always-on block.
- [ ] `plugins/*/hooks/hooks.json` synced with install tests.
- [ ] No MCP server, MCP tools, or host plugin tool named `so`.

Rules: [.agents/rules/agent-harness.mdc](../../.agents/rules/agent-harness.mdc)
