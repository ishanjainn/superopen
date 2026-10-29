# Graph engine

Native code graph: Tree-sitter extract → SQLite (`engine/`) → CLI (`so graph *`).

Parent index: [../../AGENTS.md](../../AGENTS.md)

## Goals: effective, cheap, accurate

| Goal | Implementation |
|------|----------------|
| **Effective** | `so graph query` prints a ranked TOON page (`count: N of M`, `nodes[]`, `edges[]`, numbered `bodies[]`) with `--offset` paging. |
| **Cheap** | Default stdout is one TOON page (15 rows, at most 3 bodies and 120 source lines); agent JSON carries the same page, not full node arrays (`format/compact.go`). |
| **Accurate** | Full-index FTS/BM25 seeds; structural boosts. File/Variable stay out of query seeds. |

## Key files

| Concern | Files |
|---------|--------|
| Store + FTS search | `engine/store.go`, `engine/search_pattern.go` |
| Query ranking (BM25F over name, module, path, docstring, body) | `engine/query_rank.go` |
| Query seeds + scoring (exact names, dotted qualifiers) | `engine/query_seed.go`, `engine/analysis.go` |
| BFS neighbors | `engine/query_expand.go` |
| Ranked rows + paging | `engine/analysis.go` (`queryAnswerRows`, `pageQueryRows`) |
| Index pipeline | `engine/index_*.go`, `engine/store_dump.go` |
| Agent output | `format/compact.go`, `format/help.go` |
| Protocol types | `api/` |

## Query contract (do not regress)

1. **Seeds:** FTS via `Search` (limit ~50), not `ORDER BY id LIMIT N` over callables. Do not File-prepend; BFS `DEFINES` from File would spray Variables.
2. **Search vs query:** `so graph search` filters File/Folder/Module/Section and data-language Variables. Exported source consts stay searchable. Query uses the same FTS pool.
3. **Expand:** Bounded BFS; hub skip for non-seed transit nodes; per-hop neighbor cap (same-file / same-package / question-name overlap / CALLS first). Header counts listed nodes, not the raw walk.
4. **Render:** the CLI renders `api.QueryResult.Rows/Bodies/Edges` as TOON (`format/compact.go`, `format/toon.go`). There is no second text rendering.
5. **Page + bodies:** 15 rows (`--limit`), `--offset` pages; `count` is the true total. The first four ranked rows stay in place; remaining first-page slots favor distinct implementation files before documentation examples, while a few direct callees are promoted beside their callers. `linked[]` gives at most six relevant behavioral targets outside the page. Every page attaches up to 3 Method/Function/Constructor or source Variable bodies, with a 120-line total budget (`queryBodyLineBudget`); connected implementation gets priority over tests and examples. A long body is clipped to the lines left (80 at most, `queryBodyMaxLines`), and the help line names `--from` for the rest; `so graph snippet` itself clips at 250 (`snippetMaxLines`). `code-search` attaches enclosing qualified names, up to two bare-name registrations for a long flag or namespaced directive, complete bounded source symbols for up to three implementation sites (90 lines), and up to two test bodies for distinct shared-value consumers (30 lines total). A registry key can link to its handler class. `shared[]` names other consumers and `uses[]` shows their source expressions; `tests[]` and `test_run[]` provide related tests and a module suite command. Literal patterns keep whitespace and leading dashes; user CLI help puts `--` before a dash-leading pattern. File snippets support `--from` and `--lines`. Do not lead with Cypher.

## Graph contracts (G)

| ID | Contract |
|----|----------|
| **G1** | Compact `trace` prints the requested direction. Incoming uses `callers:`; outgoing uses `callees:`; both uses `neighbors:`. AXI snippet help suggests `--direction incoming` and `outgoing`, not `both`. |
| **G2** | Syntax File/Module nodes store `start_line=1` and `end_line=file_line_count`. Query rows print `1-N` for that span, not start-only `1`. Snippet of File/Module clips at ~500 lines, sets `clipped: true`, and does not dump whole files. One-line symbols are not marked clipped just because the file continues. |
| **G3** | Path-shaped identity (`foo.ts`, `src/foo.ts`) prefers File then Module. Symbol-shaped identity prefers a single Function/Method/Class; two callables of equal rank stay `ambiguous`. |
| **G4** | FTS includes exported source Variables (JS/TS consts). JSON/YAML/TOML/INI/HCL Variables stay out of FTS. No path denylist. |
| **G5** | Query expand/render skips data-language Variable neighbors. They remain in the index. |
| **G6** | Compact `both`/`incoming` defaults to CALLS/USAGE/CONFIGURES. Data-language Variables are not listed as callees unless they were the start node. |

## Build tags

- `go test ./internal/graph/engine/`: portable stub path.
- `make test-native`: `tsnative,sqlite_fts5` for full parser + FTS.

## Tests

```bash
go test ./internal/graph/engine/ -count=1 -timeout 120s
```

Fixtures: `engine/agent_ux_test.go`.

## Asset maintenance (`tools/`)

Maintainer-only CLIs (not in CI). Run from repo root with `SUPEROPEN_GRAPH_SOURCE` set when hacking engine sources.

| Tool | Output |
|------|--------|
| `tools/graph-assets` | Tree-sitter WASM → `engine/assets/grammars/*.wasm.gz` |
| `tools/graph-specs` | `langspec/assets/lang_specs.json` |
| `tools/graph-model-assets` | Semantic model → `engine/assets/model/` |

## Change checklist

- [ ] Query seeds use FTS + path File SQL, not `ORDER BY id LIMIT` sampling.
- [ ] File nodes: seeds on **query** only; filtered from **`so graph search`**.
- [ ] Default output stays compact text; slim agent JSON (no full `nodes[]`).
- [ ] `count: N of M` is the true total; `help[]` gives the `--offset` page; snippet for known symbols.
- [ ] Test added/updated in `agent_ux_test.go` or focused `*_test.go`.
- [ ] Run `make test-native` if FTS/parser touched.

**Anti-patterns:** RAM-load all nodes for seeds; JSON-first query default; hook-style hit lists in query output.

