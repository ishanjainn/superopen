<!-- superopen:start -->
## Superopen

This project uses Superopen. Prefer AGENTS.md (including nested dir/AGENTS.md), existing vendor rules & skills dirs when present, and `so graph query` before raw exploration.

Invoke with `/so` (Claude Code, Cursor, Gemini, Copilot, OpenCode, Pi) or `$so` (Codex):
Chat syntax and shell syntax are different: `/so ...` invokes the chat skill; inside Bash or a terminal, always run `so ...` with no leading slash.
- `/so` - help
- `/so init` - bootstrap Superopen if missing
- `/so graph query "<question>"` - ask the repo knowledge graph
- `/so graph` - rebuild `.so/graph/` (local, regenerable)
- `/so doctor` - health check

Rules:
- Never type `/so ...` into Bash; the leading slash is only for chat skill invocation.
- For codebase questions, run `so graph query "<question>"` when `.so/graph/graph.json` exists.
- Read `AGENTS.md` (and nested `*/AGENTS.md`), project rules, and matching skills for the task.
- When updating guidance: edit existing rule/skill files in the dirs this repo already uses; prune obsolete lines instead of only appending.
- Read `.so/memory/context.md` when present (generated session context shared across coding agents).
- Obey `.so/guardrails.yaml`.
- Do not dump the entire `.so/` directory into context - load only what the task needs.
- After meaningful Superopen edits by a human, they will run `so sync`.
<!-- superopen:end -->
