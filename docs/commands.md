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
brief  propose  skip  scan  list  show  review  apply  decline  inventory
```

Proposals are staged diffs on instruction files. Nothing goes live without
`so harvest apply`.
