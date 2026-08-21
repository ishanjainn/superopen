# Agent harness

What `so install` writes into **user** agent config — not this repo’s contributor `AGENTS.md` tree.

Parent index: [../../AGENTS.md](../../AGENTS.md).

## Installed surfaces

| Surface | Source |
|---------|--------|
| `/so` skill | `skills/so/SKILL.md` |
| References | `skills/so/references/` |
| Durable block | `steer/block.go` → user `CLAUDE.md` |
| Cursor rule (install) | `steer.CursorRule()` → user `~/.cursor/rules` |
| Cursor rule (opt-in) | `so init --cursor-rules` → `<repo>/.cursor/rules/superopen.mdc` |
| Hooks | `plugins/*/hooks/hooks.json`, `hook/` |

Install: `install/`, `steer/install.go`.

## Product harness contract

When `.so/` exists in a **customer repo**, every supported coding vendor gets the same three outcomes (host-legal apply may differ):

| Outcome | What happens |
|---------|----------------|
| **Graph** | Agent is told to run `so graph query` before grep/read |
| **Observability** | `so coding hook` records session start/end, user prompts, tool calls, assistant turns into `.so/sessions` |
| **Silent lifecycle** | SessionStart / prompt-submit / PostToolUse / Stop / SessionEnd do **not** inject model text. Only explore-tool nudges (and SubagentStart where the host has subagents) |

| Vendor | Graph-first channel | Notes |
|--------|---------------------|-------|
| Claude Code | PreToolUse JSON (`SearchNudge` / `ReadNudge`) | Eval host |
| Cursor | `preToolUse` / `beforeReadFile` JSON | Same nudges |
| Gemini | `BeforeTool` `additionalContext` | Same nudges |
| Copilot CLI | bash + powershell hook commands | Same nudges |
| Codex | Durable `AGENTS.md` + skill only | Desktop **rejects** PreToolUse `additionalContext` — do not emit it |
| OpenCode | Hook stdout → one-shot `echo "<nudge>" ; <command>` on bash | `;` not `&&` (Windows PowerShell 5.1). Non-bash tools: telemetry + AGENTS.md |
| Pi | Native `graph_*` tools + AGENTS.md; bash echo rewrite when `command` is mutable | Do not rewrite `graph_*` tools |

OS-neutral: spawn `so` / `so.exe` with argv (never `shell: true`). Windows install pins `so.exe` via `patchPluginSoBin`.

When `.so/` exists:

1. **First:** `so graph query "<question>"`
2. **Explore-tool nudge:** MANDATORY one-liner (`SearchNudge`, `ReadNudge`) on the host-legal channel above
3. **No ExploreAugment** on Grep/Read (`graphGate`)
4. **Follow-ups:** snippet/trace; no search spray after TRUNCATED
5. **SubagentStart:** `HookReminder` (hosts that have subagents)
6. **SessionStart / UserPromptSubmit / PostToolUse / Stop / SessionEnd:** silent for steer text (observability still records `.so/sessions`)

Optional: `so install --strict` / `SUPEROPEN_HOOK_STRICT`

## Repo vs installed skill

| | Contributor `AGENTS.md` here | Installed `/so` skill |
|--|------------------------------|------------------------|
| Audience | Superopen developers | Customer repo agents |
| Trigger | Editing this checkout | `.so/` in cwd |

## Tests

```bash
go test ./internal/agent/hook/ ./internal/agent/steer/ ./internal/agent/skills/ ./internal/agent/install/ -count=1
```

Benchmarks: [../../scripts/agent-graph-eval/AGENTS.md](../../scripts/agent-graph-eval/AGENTS.md)

## Change checklist

- [ ] `skills/so/SKILL.md`: query-first tripwire unless intentional.
- [ ] Memory only in `references/memory.md`.
- [ ] Nudges: MANDATORY; no `so graph search` in default hook text.
- [ ] No ExploreAugment on live Grep/Read path.
- [ ] `Block()` / `CursorRule()`: no MCP/--json in always-on block.
- [ ] `plugins/*/hooks/hooks.json` synced with install tests.

Rules: [.agents/rules/agent-harness.mdc](../../.agents/rules/agent-harness.mdc)
