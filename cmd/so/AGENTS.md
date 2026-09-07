# CLI

Single binary: `cmd/so/` + presentation in `internal/cli/`.

**Never MCP.** Superopen is this binary only. Do not add an MCP server, MCP tool definitions, or a host plugin tool named `so`. Coding agents invoke it with their shell tool (`so graph query`, `so memory recall`, …).

Parent index: [../../AGENTS.md](../../AGENTS.md). Graph output: [../graph/AGENTS.md](../graph/AGENTS.md).

## AXI alignment

| Surface | Format |
|---------|--------|
| `so graph *` success | compact `NODE` / `EDGE` (not AXI TOON) |
| `so memory`, `so sessions` (user) | **axi.md**: TOON lists, content-first dashboards, `help[]`, structured errors on stdout |
| `so sessions hook` | **host JSON** (`additionalContext` / permission). Not AXI. Always exit 0 on telemetry failure. Hidden from `so sessions --help`; invoked by vendor plugin manifests |

| AXI | Where |
|-----|--------|
| Stable exit codes | `internal/cli/cli.go` (0/1/2 + 3 not-found / 4 continuation) |
| `--json` / `--full` | Root persistent flags; env `SO_JSON`, `SUPEROPEN_JSON` |
| TOON lists | `cli.Rows()`: `kind[n]{cols}:` plus `count:` |
| Content-first home | `so memory` (no-args dashboard); `so sessions` lists |
| Definitive empty states | `0 memories` / `0 sessions` plus `help[]` |
| Structured errors | stdout `error:` / `hint:` (JSON `{ok:false,code,error,hint}`) |
| Next-step `help[]` | `cli.Next` flushed after primary output |

`--json` envelope stays `{ok, kind, data|items, count, next}` for the web BFF.

## Documented divergences (intentional)

| Topic | Superopen |
|-------|-----------|
| Graph success output | compact `NODE`/`EDGE`; TOON would regress agent piping |
| Memory / sessions | **axi.md TOON** + dashboards; `--json` opt-in envelope |
| Hook stdout | Host control JSON, not AXI |

Do not revert to JSON-first without explicit product decision.

## Commands

```text
so init | install | uninstall | dev | projects | gc | status
so graph build | refresh | query | search | snippet | trace | …
so memory | search | get | capture | forget | …
so harvest inventory | propose | scan | list | show | apply | decline | review
so sessions | show | finalize | …

Hidden (host argv, not user-facing): so sessions hook
```

Wiring: `graph_cmd.go`, `graph_native_cli.go` → `internal/graph/client`.

## Contributor workflow

1. Change command in `cmd/so/`.
2. Update `internal/cli` and `format/*` if output shape changes.
3. `go test ./internal/cli/ ./cmd/so/...`

Dogfood: `sh scripts/install.sh`, then `so init` in **another** repo. See [CONTRIBUTING.md](../../CONTRIBUTING.md).

## Change checklist

- [ ] Exit codes unchanged or documented if new cases added.
- [ ] Default remains compact text for graph commands.
- [ ] `help[]` updated when adding graph subcommands.
- [ ] Graph UX changes covered by `internal/graph/engine/` tests.
- [ ] No MCP surface added.
