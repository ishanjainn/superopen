# Playbook harvest (on demand)

Load this when the user asks to harvest, review playbook patches, or a hook
line named `HARVEST pending`. Skip on a cold clone.

Harvest stages small patches to instruction files (`AGENTS.md`, vendor rules,
skills) in `.so/` until a human applies them. It does **not** rewrite live
playbooks from SessionEnd or headless generate.

`__SO_BIN__` is the binary from `SKILL.md`. Harvest list/review default to compact
text; `--json` is the envelope.

## Live agent

You are the live agent. On `HARVEST pending session <id>`, **before answering**:

```
__SO_BIN__ harvest brief <id>
__SO_BIN__ harvest propose
```

Pipe JSON to stdin (PowerShell: `'{"session_id":"<id>",...}' | __SO_BIN__ harvest propose`). Do not use a bash heredoc on Windows.

```json
{
  "session_id": "<id>",
  "kind": "simplify",
  "target": "AGENTS.md",
  "title": "drop unused always-on rule",
  "reason": "the session never used this rule and it costs every turn",
  "diff": "--- a/AGENTS.md\n+++ b/AGENTS.md\n@@ ...",
  "evidence": [{"kind":"session","id":"<id>","label":"user correction"}]
}
```

If the brief shows nothing worth changing:

```
__SO_BIN__ harvest skip <id>
```

Do not run `harvest scan`. That is SessionEnd for one-shot CLIs (claude-code,
codex, opencode, pi), not a live-agent fallback. Do not skip both propose and skip.

`reason` is required. `evidence` is required when `session_id` is set.
Prefer **simplify** (delete/shrink unused or duplicate rules). Max 3 proposals.
Do not restate graph-first, Superopen sentinel, or memory contracts.
Do not apply live playbooks unless the user asked in this session.

## Review (only when asked)

```
__SO_BIN__ harvest review
__SO_BIN__ harvest show <id>
__SO_BIN__ harvest apply <id>          # additive improve
__SO_BIN__ harvest apply <id> --force  # simplify / create / non-additive
__SO_BIN__ harvest decline <id>
```

Empty: `0 proposals` - nothing to do.
