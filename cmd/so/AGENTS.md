# CLI

Single binary: `cmd/so/` + presentation in `internal/cli/`.

**Never MCP.** Superopen is this binary only. Do not add an MCP server, MCP tool definitions, or a host plugin tool named `so`. Coding agents invoke it with their shell tool (`so graph query`, `so memory recall`, …).

Parent index: [../../AGENTS.md](../../AGENTS.md). Graph output: [../graph/AGENTS.md](../graph/AGENTS.md).

## AXI alignment

| Surface | Format |
|---------|--------|
| `so graph *` success | **axi.md** TOON: `count: N of M`, `nodes[N]{qn,kind,file,lines}:`, numbered `code[]`, `shared[]`/`uses[]` for values with multiple consumers, `tests[]`/`test_run[]` for related checks, `help[]` (`internal/graph/format`) |
| `so memory`, `so sessions` (user) | **axi.md**: TOON lists, content-first dashboards, `help[]`, structured errors on stdout |
| `so sessions hook` | **host JSON** (`additionalContext` / permission). Not AXI. Always exit 0 on telemetry failure. Hidden from `so sessions --help`; invoked by vendor plugin manifests |
| `so shrink`, `so blocks run *` | TOON records show the complete result content so a formatter cannot hide failures after a safe transform passes through. `--json` is machine-readable; `--raw` prints bytes for a pipe. Compressed originals have `so retrieve --raw` recovery. |
| `so retrieve` | Bounded TOON by default, with `--full` or `--raw` for complete content. |

| AXI | Where |
|-----|--------|
| Stable exit codes | `internal/cli/cli.go` (0/1/2 + 3 not-found / 4 continuation) |
| `--json` / `--full` | Root persistent flags; env `SO_JSON`, `SUPEROPEN_JSON` |
| TOON output | `cli.Rows()` (tables, `count:` first), `cli.Toon()` (any record: sorted keys, nested maps, clipped lists with `--full`), `internal/graph/format` (graph views) |
| Content-first home | Every command group shows live state or a table of its subcommands with no arguments, never help text (`group_home.go` wires the ones with no data; a test fails if a group has no bare behavior) |
| Definitive empty states | `count: 0` plus `<kind>: none` (memories, sessions, proposals, findings) and `help[]` |
| Structured errors | stdout `error:` plus `help[1]:` (JSON `{ok:false,code,error,hint}`); unknown flag exits 2 |
| Next-step `help[]` | `cli.Next` flushed after primary output |

`--json` envelope stays `{ok, kind, data|items, count, next}` for the web BFF.

## Documented divergences (intentional)

| Topic | Superopen |
|-------|-----------|
| Every other command | **axi.md TOON** by default (`cli.Toon` renders any result the same way); `--json` opt-in envelope |
| Hook stdout | Host control JSON, not AXI |
| `so install` / `so uninstall` | Structured TOON on terminal and in pipes; install defaults to all supported agents unless `--vendor` narrows it |
| `so dev` | Long-running server: prints `dev: running` and `url:`, then serves |
| `so version` | One TOON record by default; `--json` gives the shared envelope |
| Not a Superopen repo | Exits 0 with `status: not a Superopen repo`, so an agent in another checkout is not told to `so init` |

`cmd/so/axi_contract_test.go` enforces the flag contract on every command: an unknown flag exits 2 with `error:` and a `help[1]:` line naming usage and valid flags.

Do not revert to JSON-first without explicit product decision.

## Commands

```text
so init | install | uninstall | dev | projects | gc | status
so graph build | refresh | query | search | snippet | trace | …
so memory | search | get | capture | forget | …
so harvest inventory | propose | scan | list | show | apply | decline | review | jev | jev-settings
so scan
so shrink | retrieve
so blocks scan | init | apply | decline | run
so sessions | show | finalize | …

Hidden (host argv, not user-facing): so sessions hook
```

Wiring: `graph_cmd.go`, `graph_native_cli.go` → `internal/graph/client`.

`so blocks run` is a content-first group of named scripts. `blocks scan` counts
distinct sessions, and `blocks init` stages only scripts matched by repeated
commands. A staged index is never deleted by an empty rescan.

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
