# Superopen repository — agent instructions

**Scope:** Contributors and CI agents editing **this repo**. Not the end-user `/so` skill (`internal/agent/skills/so/SKILL.md`), which `so install` writes into customer projects when `.so/` exists there.

| Context | Read |
|---------|------|
| Editing Superopen source | This file + nested `AGENTS.md` below |
| Using Superopen in another repo | Installed `/so` skill (source: `internal/agent/skills/so/SKILL.md`) |

## Nested docs (colocated)

| Area | `AGENTS.md` |
|------|-------------|
| Graph engine | [internal/graph/AGENTS.md](internal/graph/AGENTS.md) |
| CLI | [cmd/so/AGENTS.md](cmd/so/AGENTS.md) (+ `internal/cli/`) |
| Memory | [internal/memory/AGENTS.md](internal/memory/AGENTS.md) |
| Agent harness | [internal/agent/AGENTS.md](internal/agent/AGENTS.md) |
| UI | [web/AGENTS.md](web/AGENTS.md) |
| Tests & eval | [scripts/AGENTS.md](scripts/AGENTS.md) |
| Benchmarks | [scripts/agent-graph-eval/AGENTS.md](scripts/agent-graph-eval/AGENTS.md) |

## Shared rules (all agents)

Scoped rules live in **[`.agents/rules/`](.agents/rules/)** — agent-agnostic (Cursor, Claude Code, Codex, etc.).

- Always-on: [`.agents/rules/repo.mdc`](.agents/rules/repo.mdc)
- Cursor loads the same files via [`.cursor/rules`](.cursor/rules) → symlink to `.agents/rules`

See [`.agents/README.md`](.agents/README.md) for layout.

## Principles

1. **Effective, cheap, accurate** graph agent UX — [internal/graph/AGENTS.md](internal/graph/AGENTS.md)
2. **AXI CLI** with text-first / query-first divergences — [cmd/so/AGENTS.md](cmd/so/AGENTS.md)
3. **Minimal diffs** — match local style
4. **No vendor names** in product or contributor copy

## Quick commands

```bash
make build test test-native test-web lint
sh scripts/install.sh   # dogfood in another repo with so init
```
