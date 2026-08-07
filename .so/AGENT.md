Superopen agent harness: prefer AGENTS.md, so graph query, and vendor rules/skills before broad search.

- Graph: .so/graph/ — `so graph query "…"`
- Knowledge: .so/knowledge/ (architecture.md, conventions.md, decisions.md)
- Rules: .claude/rules/, .codex/rules/, .cursor/rules/, AGENTS.md, CLAUDE.md
- Skills: .agents/skills/, .cursor/skills/, .pi/skills/, and other vendor skill dirs
- Guardrails: .so/guardrails/guardrails.yaml (single file)
- Active context: .so/memory/active-context.md (SessionStart inject)

CLI: cmd/so · core: internal/ · UI: web/ · seeds: templates/
