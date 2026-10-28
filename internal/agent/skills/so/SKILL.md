---
name: so
description: "Use for any question about a codebase, its architecture, file relationships, or project content. Start with graph code-search for an exact flag, key, error or value; graph query for unknown behavior and relationships. Also use for prior-work facts (so memory recall) and when the user wants a fact stored (so memory capture). Superopen is a CLI binary invoked with your shell tool (Bash, PowerShell, or equivalent), not an MCP tool. If .so/ is missing, run one graph query anyway (a linked worktree may seed from the parent). If that prints the unmanaged message, stop and do not so init unless the user explicitly asked."
---

# Superopen (`/so`)

Superopen is a **CLI binary**, not an MCP tool and not a host plugin tool. There is no `so` tool schema. Use `so` (`so.exe` on Windows) when it is on `PATH`; otherwise copy-paste the install-time absolute path below into your shell tool. On Windows use PowerShell or cmd, not `bash -c`.

```
__SO_BIN__
```

The examples below use `so`; substitute the absolute path above if `so` is unavailable. Do not retype or shorten the path.

Never Grep `.so/` or installed skill/rule directories. Never pipe `so` through `head`/`tail`. If you spawn a Task/subagent, its prompt must say to run the same command line first (Explore children never see SessionStart).

If `.so/` is missing, still run **one** `graph query`. If stdout says `not a Superopen repo`, stop. Do not `so init` unless the user explicitly asked. A linked git worktree of a managed parent seeds on that call.

## Scenario table

| Situation | Command | Then |
|-----------|---------|------|
| Unknown behavior or relationship | `so graph query "<question>"` | Ranked `nodes[]` with file and lines; `bodies[]` hold source with line numbers. Answer from them |
| Known symbol, need body | `so graph snippet "<qn>"` | A qn from `nodes[]` or `candidates[]` |
| Callers | `so graph trace "<qn>"` | `--direction incoming` when you need callers |
| Rename / signature / multi-file edit | `so graph impact --files <path>` or `--base main` | Check siblings before you finish; do not skip dependents |
| Cold repo / architecture | `so graph architecture` | Stop; do not dump every hub |
| Prior work / diary | `so memory recall "<question>"` | Quote `#id`; not graph query |
| Persist a fact for later | `so memory capture --kind knowledge --horizon medium --title "…" --text "…"` | Intent, not a phrase; then stop |
| Literal flag, option, key, error or value | `so graph code-search -- "<literal>" [path...]` | Use the most distinctive exact string. The result ranks definitions, shows bare-name registrations for flags and namespaced directives, names enclosing `qn`s and related graph users, and attaches bounded source bodies. Follow a related `qn` with `graph snippet` if needed. Grep only if the file is unindexed |
| Several literal variants or a regex | `so graph code-search --regex -- "<pattern>" [path...]` | Use a bounded path such as `tests` when looking for test coverage; hits retain qualified names and source contexts |

Use `graph code-search -- "--verbose"` first for an exact flag, or search an exact key or error message. Keep punctuation and prefer the longest distinctive literal (`--verbose` over `verbose`). The `--` before the pattern is required for patterns beginning with `-`; put options such as `--limit` before it. A broad natural-language query over common words can return thousands of unrelated symbols and source lines. For relationships, ask `graph query` in a few specific words: the symbols and behavior you are after (`QuerySet.count unused annotations`), not pasted issue text, code or tracebacks. Long pasted text ranks worse. A traceback's frames are already symbols: `graph snippet` them directly.

Output is TOON: `count: N of M`, then `name[N]{cols}:` rows. Query `edges[]` connects listed nodes; `linked[]` names relevant call targets just outside the page, with full qns for `graph snippet`; `bodies[]` gives numbered source. Code-search `matches[]` names exact literal hits, `name_matches[]` suggests bare-name registrations for flags and namespaced directives, `related[]` names indexed users or registered handlers, and `contexts[]` gives bounded numbered implementation and test source. `tests[]` names relevant test symbols and `test_run[]` gives a module-level command; use it after adding a regression at the shared producer, so all consumers run together. `uses[]` shows the actual expression at each shared-value consumer. Follow a relevant qn with `graph snippet <qn>` or `graph trace <qn> --direction outgoing` before reading source elsewhere. Use `graph snippet <qn> --context 20` for surrounding source and the printed `--from` help for a clipped body. After `code-search`, use its exact `help[]` source-window command only if `contexts[]` lacks the needed lines. When `count` shows more than you need and the answer is not listed, run the `help[]` line it printed (for example `--offset 15`), not Grep. Offset pages also attach source bodies. `count: 0` means no indexed exact match in the printed scope; check freshness and scope before claiming absence. Do not Read lines already in `bodies[]`, `contexts[]`, or `code[]`; read an indexed file directly only when editing needs surrounding context unavailable from snippets. Do not list the tree to confirm Superopen.

`shared[]` in code-search names a value and its consumers. Reproduce the issue at the shared producer, not by patching one consumer's imported alias. Check every listed consumer, including behavior after cached state changes. If more than one consumer needs the same normalization, apply it at the producer. When an edit emits an option, directive, event name, or other string consumed elsewhere, `graph code-search` that exact string without a path filter to find its parser or handler.

Before editing a value or its formatting at one use site, check `related[]` and `edges[]` for other consumers. If several paths share the same producer, inspect its source and the other consumers in `contexts[]` or with `graph snippet`; choose the fix location that covers the observed behavior across those paths. Check the relevant tests with a bounded `code-search` or snippet.

Once the graph has shown the cause, the edit site, and a relevant test, make the change and run that test. Expand discovery only when a concrete question remains or a test fails. Broad searches for possible nearby behavior after those points add cost and usually repeat source already shown in the graph.

Use Git history for historical questions, not to discover the current implementation; the graph and snippets describe the current checkout.

If a hook line says `HARVEST pending`, you are the live agent: run `__SO_BIN__ harvest brief <id> --full` then `__SO_BIN__ harvest propose` (JSON on stdin, include `session_id`) **before answering**. If that session has nothing to change, run `__SO_BIN__ harvest skip <id>`. The current user message is a memory capture, not a playbook diff for that session. Do not write a file. Same for `DISTILL pending` (`memory distill --brief <id> --full`, then `memory distill --apply <id>`; empty array if nothing durable). See `references/harvest.md`.

Recipes (dead code, fan-in/out, routes): `references/query.md`. Prior-work memory: `references/memory.md`. Playbook harvest (on demand): `references/harvest.md`.
