# Commands

`so` is a single binary. Help is authoritative: `so <command> --help`.

## Global flags

| Flag | Effect |
|------|--------|
| `--json` | Machine-readable JSON envelope `{ok, kind, data\|items, count, next}` (env: `SO_JSON`, `SUPEROPEN_JSON`) |
| `--full` | Do not truncate fields |
| `--root` | Repository root override (env: `SUPEROPEN_ROOT`; otherwise walks up to git top-level) |

Exit codes are stable: `0` ok, `1` error, `2` usage, `3` not found,
`4` continuation. Errors print `error:` / `hint:` on stdout.

## Top-level commands

| Command | Purpose |
|---------|---------|
| `so init` | Create `.so/` and build the native graph for this repository |
| `so install` | Install `/so` skill + hooks into user agent configs (user-global) |
| `so uninstall` | Remove agent wiring, project index, caches, `.so/` data (`--keep-data`, `--vendor=<name>`) |
| `so dev` | Start the web UI (`-d` detaches); binds current inited repo or last managed project |
| `so projects` | List repositories registered with Superopen |
| `so status` | Show active observability sessions |
| `so gc` | Apply retention: delete old sessions, harvest history, and unpinned memories |
| `so version` | Print CLI version |

## so graph — [docs](graph.md)

```text
build      refresh     status     builds
query      search      snippet    trace
impact     code-search cypher     schema
architecture           coverage   diagnostics
artifact   layout      projects
```

Defaults: compact `NODE`/`EDGE` text under a token budget; `TRUNCATED`
suggests `snippet` or a narrower question. `so graph impact` prints compact
file rollups (`--files`, `--base main`). `--json` / `--full` are escape
hatches. Cypher support is a read-only subset (`so graph cypher --help`).

## so memory — [docs](memory.md)

No args opens a live dashboard. Subcommands:

```text
search  get  last  timeline  recall  temporal-recall
capture teach contradict forget fade rescue pin
ingest distill sleep status layout
```

Search results are TOON index rows (`memories[n]{id,kind,title,tokens}:`);
bodies stay behind `so memory get`.

## so sessions — [docs](sessions.md)

```text
list  show  finalize  refresh  tokens  checkpoint  demo
```

`finalize` materializes raw observability spans into the session document.
The hidden `so sessions hook` subcommand is host protocol (vendor JSON),
invoked by installed plugin manifests — not for direct use.

## so harvest — [docs](harvest.md)

```text
inventory  propose  scan  list  show  review  apply  decline
```

Proposals are staged diffs on instruction files; nothing goes live without
`so harvest apply`.
