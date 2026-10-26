# Graph

The native code graph: Tree-sitter AST extraction into SQLite, then
`so graph *` queries. Builds are local and deterministic. No LLM, no network.
On Superopen's own source the full build indexes thousands of nodes and edges
in a few seconds.

## Lifecycle

```bash
so init            # create .so/ + first build (runs inside a repo)
so graph build     # full rebuild
so graph refresh   # incremental; skips when unchanged, or when .so/ is missing (--force for full)
so graph status    # build state of the current project
so graph projects  # every indexed project on this machine
```

Graph reads (`query`, `search`, `snippet`, …) probe for unindexed edits and
spawn an incremental `so graph refresh --probe` when the workspace already
has `.so/`. Session hooks do not spawn graph refresh. Skip the check with
`so graph --no-refresh` or `SUPEROPEN_NO_REFRESH=1` (CI). Set
`SUPEROPEN_REFRESH=hash` when size and mtime can match after a content change.
If the index is still behind, the answer starts with
`[!] graph stale: edit not indexed yet`. When 12 or fewer files are dirty, the
next lines name them and say they are absent from the answer.

At most `SUPEROPEN_BUILD_SLOTS` builds run at once (default 2; `0` is
unlimited). If refresh says `pool_full`, wait or raise the cap.
See [configuration.md](configuration.md).

## Ignoring files

Create `.soignore` in the repository root. Syntax matches `.gitignore`,
including `!` negation.

The walker also skips generated trees such as `vendor/` and `node_modules/`,
and binary suffixes such as `.wasm`. Symlinks that leave the repository are
not indexed.

`.so/.gitignore` ignores `sessions/`, `db/`, and `harvest/`.

## Asking questions

| Command | Use for |
|---------|---------|
| `so graph query "<question>"` | Plain-language question to a ranked subgraph. Default answer surface. |
| `so graph search <pattern>` | Find symbols by name (BM25 over FTS). |
| `so graph snippet <qn>` | Read the source of a known symbol. Symbols clip around 250 lines. Files and modules clip around 500. `--from <line>` continues a clipped read. |
| `so graph trace <symbol>` | Callers (`--direction incoming`), callees (`outgoing`), or both. |
| `so graph impact <symbol\|--files\|--base>` | Blast radius: callers, impls, siblings, co-change. Run this before you finish a multi-file edit. |
| `so graph architecture` | Languages, packages, routes, hotspots, clusters. |
| `so graph schema` | Node labels, relationship types, property patterns. Run this first when scripting. |
| `so graph cypher "<query>"` | Read-only Cypher subset. |
| `so graph code-search <pattern> [path]` | Text search over indexed files only. `[path]` (or `--path`) limits it to a repo-relative file or directory; `--glob` (or `--file-pattern`) filters by file glob. Matches carry line numbers. |
| `so graph coverage` | What was indexed, what was skipped and why. |
| `so graph diagnostics` | Database integrity checks. |
| `so graph artifact` | Export/import/verify a content-addressed graph snapshot for teammates. |

## Output contract

Success output is [AXI](https://axi.md) TOON, the same shape for every graph command:

```text
count: 15 of 101
nodes[15]{qn,kind,file,lines}:
  django.core.mail.utils.DNS_NAME,Variable,django/core/mail/utils.py,20
  django.core.mail.message.EmailMessage.message,Method,django/core/mail/message.py,239-264
  django.core.mail.utils.CachedDnsName.get_fqdn,Method,django/core/mail/utils.py,14-17
  ...
edges[1]{from,rel,to}:
  EmailMessage.message,CALLS,EmailMessage._create_message
bodies[1]:
  - qn: django.core.mail.message.EmailMessage.message
    file: django/core/mail/message.py
    code[26 lines]:
      239|     def message(self):
      ...
help[1]:
  86 more ranked: so graph query "<question>" --offset 15
```

- `count: N of M` is the true total. The page is ranked: listed seeds, then
  search order, then graph neighbors. `--limit` sets the page size (15) and the
  `help[]` line gives the exact `--offset` command for the next page.
- Project source comes first. Test files stay off the page unless the question
  names tests. Folder, Section and data-file Variable rows are dropped.
- `edges[]` lists only behavioral edges (CALLS, USAGE, INHERITS, ...) between
  listed rows. `bodies[]` attach source for the top callables with absolute
  line numbers, whole lines only.
- A clipped body or snippet prints `code[<shown> of <total> lines]` and a
  `help[]` line with `so graph snippet <qn> --from <line>`.
- `count: 0` plus one line stating what was searched is a definite answer.
  Errors print `error:` and `hint:` on stdout; unknown symbols exit 3, usage
  errors exit 2 and list the command's flags.
- Hints are omitted when the answer is self-contained (a complete snippet).
- `--json` / `--full` are script escape hatches. Agent piping relies on the
  default shape.
- Do not pipe `so` through `head` or `tail`.

## Data model

Labels include `Project`, `Package`, `Folder`, `File`, `Module`, `Class`,
`Function`, `Method`, `Interface`, `Enum`, `Type`, `Route`. Relationships
include `CALLS`, `USAGE`, `IMPORTS`, `DEFINES`, `IMPLEMENTS`, `HANDLES`,
`CONFIGURES`, `TESTS`. The live truth is `so graph schema`.

Identity rules: path-shaped names resolve File then Module. Symbol-shaped
names resolve to a single callable or report `ambiguous` rather than guessing.

## For contributors

- Engine layout and contracts (G1-G6): [`internal/graph/AGENTS.md`](../internal/graph/AGENTS.md).
- Native build: `make build` (`-tags tsnative,sqlite_fts5`); tests:
  `make test test-native`.
- Grammar/langspec assets are regenerated by maintainer CLIs under `tools/`.
