# CLI

Single binary: `cmd/so/` + presentation in `internal/cli/`.

Parent index: [../../AGENTS.md](../../AGENTS.md). Graph output: [../graph/AGENTS.md](../graph/AGENTS.md).

## AXI alignment

| AXI | Where |
|-----|--------|
| Stable exit codes | `internal/cli/cli.go` |
| `--json` / `--full` | Root persistent flags; env `SO_JSON`, `SUPEROPEN_JSON` |
| Definitive empty states | Clear “not found” / “no graph” text |
| Structured errors | `cli.Err`, JSON envelope when `--json` |
| Next-step `help[]` | `internal/graph/format/help.go` |

## Documented divergences (intentional)

| Topic | Superopen |
|-------|-----------|
| Default output | **Compact text**; `--json` opt-in |
| Primary discovery | **`graph query`** first; search secondary |
| MCP | Not in default install |

Do not revert to JSON-first without explicit product decision.

## Commands

```text
so init | install | uninstall | dev | projects
so graph build | refresh | query | search | snippet | trace | …
so memory search | get | capture | …
so sessions list | show | finalize | …
so coding hook …
```

Wiring: `graph_cmd.go`, `graph_native_cli.go` → `internal/graph/client`.

## Contributor workflow

1. Change command in `cmd/so/`.
2. Update `internal/cli` and `format/*` if output shape changes.
3. `go test ./internal/cli/ ./cmd/so/...`

Dogfood: `sh scripts/install.sh`, then `so init` in **another** repo — [docs/local-build.md](../../docs/local-build.md).

## Change checklist

- [ ] Exit codes unchanged or documented if new cases added.
- [ ] Default remains compact text for graph commands.
- [ ] `help[]` updated when adding graph subcommands.
- [ ] Graph UX changes covered by `internal/graph/engine/` tests.

Rules: [.agents/rules/cli.mdc](../../.agents/rules/cli.mdc)
