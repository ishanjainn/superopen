<!-- superopen:start -->
## Superopen

This project is managed by Superopen (`.so/`). Prefer `.so/` before raw exploration to save tokens.

Invoke with `/so` (Claude Code, Cursor, Gemini, Copilot, OpenCode, Pi) or `$so` (Codex):
- `/so` - help
- `/so init` - bootstrap Superopen if missing
- `/so graph query "<question>"` - ask the repo knowledge graph
- `/so graph` - rebuild `.so/graph/` (never leave `graphify-out/` at repo root)
- `/so doctor` - health check

Rules:
- For codebase questions, run `so graph query "<question>"` when `.so/graph/graph.json` exists.
- Read relevant files under `.so/knowledge/` and `.so/rules/`, plus matching skills in `.so/skills/` for the task.
- Read `.so/memory/active-context.md` when present (session memory pack shared across coding agents).
- Obey `.so/guardrails/guardrails.yaml`.
- Do not dump the entire `.so/` directory into context - load only what the task needs.
- After meaningful Superopen edits by a human, they will run `so sync`.
<!-- superopen:end -->












