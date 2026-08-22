# Graph engine

Native code graph: Tree-sitter extract → SQLite (`engine/`) → CLI (`so graph *`).

Parent index: [../../AGENTS.md](../../AGENTS.md)

## Goals: effective, cheap, accurate

| Goal | Implementation |
|------|----------------|
| **Effective** | `so graph query` returns ranked NODE/EDGE lines with `src=` paths; path-shaped File labels (`parent/file.go`). |
| **Cheap** | Default stdout is compact text under `--budget`; agent JSON omits full node arrays (`format/compact.go`). |
| **Accurate** | Full-index FTS/BM25 seeds; structural boosts. File/Variable stay out of query seeds. |

## Key files

| Concern | Files |
|---------|--------|
| Store + FTS search | `engine/store.go`, `engine/search_pattern.go` |
| Query seeds + scoring | `engine/query_seed.go`, `engine/analysis.go` |
| BFS + NODE/EDGE text | `engine/query_expand.go` |
| Index pipeline | `engine/index_*.go`, `engine/store_dump.go` |
| Agent output | `format/compact.go`, `format/help.go` |
| Protocol types | `api/` |

## Query contract (do not regress)

1. **Seeds:** FTS via `Search` (limit ~50), not `ORDER BY id LIMIT N` over callables. Do not File-prepend; BFS `DEFINES` from File would spray Variables.
2. **Search vs query:** `so graph search` filters File/Folder/Module/Section and data-language Variables. Exported source consts stay searchable. Query uses the same FTS pool.
3. **Expand:** Bounded BFS; hub skip for non-seed transit nodes.
4. **Render:** NODE lines include `qn=` for callables; File NODE names use `dir/basename.go` when useful.
5. **Budget:** `TRUNCATED` → `so graph snippet <qn>` or narrow the question. Do not lead with `--budget` or Cypher.

## Graph contracts (G)

| ID | Contract |
|----|----------|
| **G1** | Compact `trace` prints the requested direction. Incoming uses `callers:`; outgoing uses `callees:`; both uses `neighbors:`. AXI snippet help suggests `--direction incoming` and `outgoing`, not `both`. |
| **G2** | Syntax File/Module nodes store `start_line=1` and `end_line=file_line_count`. Compact query NODE `loc=` prints `L1-N` for that span, not start-only `L1`. Snippet of File/Module clips at ~500 lines, sets `clipped: true`, and does not dump whole files. One-line symbols are not marked clipped just because the file continues. |
| **G3** | Path-shaped identity (`foo.ts`, `src/foo.ts`) prefers File then Module. Symbol-shaped identity prefers a single Function/Method/Class; two callables of equal rank stay `ambiguous`. |
| **G4** | FTS includes exported source Variables (JS/TS consts). JSON/YAML/TOML/INI/HCL Variables stay out of FTS. No path denylist. |
| **G5** | Query expand/render skips data-language Variable neighbors. They remain in the index. |
| **G6** | Compact `both`/`incoming` defaults to CALLS/USAGE/CONFIGURES. Data-language Variables are not listed as callees unless they were the start node. |

## Build tags

- `go test ./internal/graph/engine/` — portable stub path.
- `make test-native` — `tsnative,sqlite_fts5` for full parser + FTS.

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

- [ ] Query seeds use FTS + path File SQL — no `ORDER BY id LIMIT` sampling.
- [ ] File nodes: seeds on **query** only; filtered from **`so graph search`**.
- [ ] Default output stays compact text; slim agent JSON (no full `nodes[]`).
- [ ] TRUNCATED suggests snippet or narrow; snippet for known symbols.
- [ ] Test added/updated in `agent_ux_test.go` or focused `*_test.go`.
- [ ] Run `make test-native` if FTS/parser touched.

**Anti-patterns:** RAM-load all nodes for seeds; JSON-first query default; hook-style hit lists in query output.

Rules: [.agents/rules/graph-engine.mdc](../../.agents/rules/graph-engine.mdc)
