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
| Benchmarks | [benchmarks/AGENTS.md](benchmarks/AGENTS.md) |

## Shared rules (all agents)

Scoped rules live in **[`.agents/rules/`](.agents/rules/)** — agent-agnostic (Cursor, Claude Code, Codex, etc.).

- Always-on: [`.agents/rules/repo.mdc`](.agents/rules/repo.mdc)

There is no repo-local `.cursor/rules` tree. Cursor still loads user-level `~/.cursor/rules` from `so install`. See [`.agents/README.md`](.agents/README.md) for layout.

## Principles

1. **Effective, cheap, accurate** graph agent UX — [internal/graph/AGENTS.md](internal/graph/AGENTS.md)
2. **AXI CLI** with text-first / query-first divergences — [cmd/so/AGENTS.md](cmd/so/AGENTS.md)
3. **Minimal diffs** — match local style
4. **No vendor names** in product or contributor copy

## Quick commands

```bash
make build test test-native test-web lint sync-plugins
sh scripts/install.sh   # dogfood in another repo with so init
```

## Repository layout

| Path | Purpose |
|------|---------|
| `cmd/so/` | CLI entrypoint |
| `internal/` | Graph engine, memory, agent harness, sessions |
| `web/` | Next.js UI (`so dev`) |
| `plugins/` | Vendor hook payloads (source of truth; run `make sync-plugins`) |
| `sdk/go/` | OTel span helpers + semconv |
| `scripts/` | Installers, packaging, Homebrew dev formula |
| `benchmarks/` | Agent eval harness + `bench-init.sh` |
| `tools/` | Maintainer CLIs for graph WASM/spec/model assets |
