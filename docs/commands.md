# Commands

`so` is a single binary. Help is authoritative: `so <command> --help`.

Global flags, the config file, and environment variables are listed in
[configuration.md](configuration.md). Install and uninstall steps are in
[installation.md](installation.md).

## Global flags

| Flag | Effect |
|------|--------|
| `--json` | Machine-readable JSON envelope `{ok, kind, data\|items, count, next}` (env: `SO_JSON`, `SUPEROPEN_JSON`) |
| `--full` | Do not truncate fields (env: `SO_FULL`, `SUPEROPEN_FULL`) |
| `--root` | Repository root override (env: `SUPEROPEN_ROOT`; otherwise walk up to git top-level or nearest `.so/`) |

Exit codes are stable: `0` ok, `1` error, `2` usage, `3` not found,
`4` continuation. Errors print `error:` / `hint:` on stdout.

## Top-level commands

| Command | Purpose |
|---------|---------|
| `so init` | Create `.so/` and build the native graph for this repository (`--force`, `--dev`, `--cursor-rules`) |
| `so install` | Install `/so` skill + hooks into user agent configs (user-global; `--vendor`, `--strict`) |
| `so uninstall` | Remove agent wiring, project index, caches, `.so/` data (`--keep-data`, `--vendor=<name>`, `--dry-run`) |
| `so dev` | Start the web UI (`-d` detaches, `status` / `stop`; `--ui-port`, `--hot`, `--no-open`) |
| `so projects` | List repositories registered with Superopen (`projects prune` drops missing entries) |
| `so status` | Show active observability sessions |
| `so gc` | Apply retention (`--show`, `--sessions-hours`, `--memory-hours`) |
| `so scan` | Run detection rules over recorded tool calls. See [scan.md](scan.md). |
| `so shrink` | Shrink stdin or one command's stdout. Original bytes stay in `.so/db/ccr.db`. |
| `so retrieve` | Print the exact original bytes for a recovery handle. |
| `so blocks` | Stage reusable scripts and run the built-ins. Apply is the only write to `AGENTS.md`. |
| `so version` | Print CLI version |

## so graph ([graph.md](graph.md))

```text
build      refresh     status     builds
query      search      snippet    trace
impact     code-search cypher     schema
architecture           coverage   diagnostics
artifact   layout      projects
```

Defaults: compact `NODE`/`EDGE` text under a token budget. `TRUNCATED`
means run `snippet` or ask a narrower question. `so graph impact` prints
compact file rollups (`--files`, `--base main`). `--json` / `--full` are
escape hatches. Cypher support is a read-only subset (`so graph cypher --help`).

`so graph --no-refresh` skips the query-path freshness check (useful in CI).
`SUPEROPEN_NO_REFRESH=1` does the same. A clipped snippet prints
`omitted:` and a follow-up `so graph snippet <qn> --from <line>`.

## so memory ([memory.md](memory.md))

No args opens a live dashboard. Subcommands:

```text
search  get  last  timeline  recall  temporal-recall
capture teach contradict forget fade rescue pin
ingest distill sleep status layout
upload watch observe
```

Search results are TOON index rows (`memories[n]{id,kind,title,tokens}:`).
Bodies stay behind `so memory get`. `so memory recall` prints clipped bodies.

## so sessions ([sessions.md](sessions.md))

```text
list  show  finalize  refresh  tokens  checkpoint  demo
```

`finalize` materializes raw observability spans into the session document.
The hidden `so sessions hook` subcommand is host protocol (vendor JSON).
Plugin manifests invoke it. Do not run it by hand.

## so harvest ([harvest.md](harvest.md))

```text
brief  propose  skip  scan  list  show  review  apply  decline  inventory  jev  jev-settings
```

Proposals are staged diffs on instruction files. Nothing goes live without
`so harvest apply`. Kind `memory` can store diary text on apply without a diff.
With Jev harvest enabled in Settings, the UI evaluates the latest finished
session and the agent is not asked to propose.

## so scan ([scan.md](scan.md))

```bash
so scan
so scan --session <id> --min-severity high --fail-on high
so scan --json
so scan --rules <dir>
```

Reads `.so/sessions/`. Does not block the agent. Empty stored rules fall back
to a small built-in baseline. `--json` is the shared `{ok, kind, data}` envelope.
The UI loads this when you open Scan or a session.

## so shrink / so retrieve

```bash
so shrink < noise.json
so shrink -- pnpm test
so shrink --type json < rows.json
so retrieve ccr_…
```

`so shrink` reads stdin, or the stdout of the command after `--`. Above 64 MiB
it fails with `cave_input_too_large` and does not truncate. If the transform
is not smaller, or the original cannot be stored, the original bytes are what
print. `so graph`, `so memory`, and `so sessions` output is unchanged.

The recovery file is `.so/db/ccr.db` (gitignored with `db/`). The default cap
is 512 MiB (`SUPEROPEN_CCR_MAX_BYTES`). At the cap, new transforms pass the
original through. Existing handles are not evicted. A bad handle fails.

Token lines on stderr are `inferred` local estimates. They are not a provider
bill.

`so install --voice` adds an opt-in short-reply skill. A normal install does
not. It stays off until you ask for it.

## so blocks

```bash
so blocks scan
so blocks init
so blocks apply
so blocks decline
so blocks run test-summary
so blocks run json-peek <file> [path]
so blocks run grep-defs <file>
```

`scan` ranks shell commands retyped in local session history. Script text stays
on the machine. `init` stages an index and does not edit `AGENTS.md`. `apply`
writes that index. A hook may add one line telling the agent to use a block.
It does not deny or rewrite the command. `grep-defs` can be larger than a
plain grep. That note stays in the index.
