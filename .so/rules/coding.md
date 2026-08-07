# Coding rules

Project-specific rules for AI coding agents. Prefer these over inventing process.

Guardrails (`.so/guardrails/`) are hard stop/warn policies. **Rules** here are guidance the agent should follow while coding (style, PR conventions, concurrency, etc.).

## Sources

- Seeded from repo agent files (`AGENTS.md`, `CLAUDE.md`, `.cursor/rules/*`) during `so init` / `/so init`.
- Edit freely; `so sync` will not overwrite your changes.

## Checklist

- [ ] Keep diffs scoped to the task
- [ ] Prefer `so graph query` and `.so/knowledge/` before broad Grep
- [ ] Follow PR / commit conventions for this repo
- [ ] Never commit secrets; use parameterized SQL; respect rate limits


- [ ] Check `.so/skills/` for a matching skill before broad exploration
