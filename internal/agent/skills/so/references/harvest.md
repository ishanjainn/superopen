# Playbook harvest (on demand)

Load this only when the user asks to harvest, review playbook patches, or
when a SessionStart line named `HARVEST`. Skip on a cold clone.

Harvest stages small patches to instruction files (`AGENTS.md`, vendor rules,
skills) in `.so/` until a human applies them. It does **not** rewrite live
playbooks from SessionEnd or headless generate.

`__SO_BIN__` is the binary from `SKILL.md`. Harvest list/review default to compact
text; `--json` is the envelope.

## Propose (wrap-up only)

If this skill is already loaded at wrap-up, you may write JSON — not a second
full review:

```bash
__SO_BIN__ harvest propose <<'EOF'
{
  "kind": "simplify",
  "target": "AGENTS.md",
  "title": "drop unused always-on rule",
  "reason": "the session never used this rule and it costs every turn",
  "diff": "--- a/AGENTS.md\n+++ b/AGENTS.md\n@@ ...",
  "evidence": [{"kind":"session","id":"<session_id>","label":"user correction"}]
}
EOF
```

`reason` is required. `evidence` is required when `session_id` is set.
Prefer **simplify** (delete/shrink unused or duplicate rules). Max 3 proposals.
Do not restate graph-first, Superopen sentinel, or memory contracts.
Do not apply live playbooks unless the user asked in this session.

## Review (only when asked)

```bash
__SO_BIN__ harvest review
__SO_BIN__ harvest show <id>
__SO_BIN__ harvest apply <id>          # additive improve
__SO_BIN__ harvest apply <id> --force  # simplify / create / non-additive
__SO_BIN__ harvest decline <id>
```

Empty: `0 proposals` — nothing to do.
