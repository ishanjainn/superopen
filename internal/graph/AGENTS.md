# Graph engine

Native code graph: Tree-sitter extract → SQLite (`engine/`) → CLI (`so graph *`).

Parent index: [../../AGENTS.md](../../AGENTS.md)

## Goals: effective, cheap, accurate

| Goal | Implementation |
|------|----------------|
| **Effective** | `so graph query` returns ranked NODE/EDGE lines with `src=` paths; path-shaped File labels (`parent/file.go`). |
| **Cheap** | Default stdout is compact text under `--budget`; agent JSON omits full node arrays (`format/compact.go`). |
| **Accurate** | Full-index FTS/BM25 seeds; structural boosts; term-scoped File seeds on query only. |

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

1. **Seeds:** FTS via `Search` (limit ~50), not `ORDER BY id LIMIT N` over callables. Plus capped path-token **File** SQL in `queryFilePathSeeds`.
2. **Search vs query:** `so graph search` filters File/Folder/Variable noise. **File seeds belong on query only.**
3. **Expand:** Bounded BFS; hub skip for non-seed transit nodes.
4. **Render:** File NODE names use `dir/basename.go` when useful.
5. **Budget:** `TRUNCATED` → **raise `--budget` or narrow** — then `so graph snippet` for a known symbol.

## Build tags

- `go test ./internal/graph/engine/` — portable stub path.
- `make test-native` — `tsnative,sqlite_fts5` for full parser + FTS.

## Tests

```bash
go test ./internal/graph/engine/ -count=1 -timeout 120s
```

Fixtures: `engine/agent_ux_test.go`.

## Change checklist

- [ ] Query seeds use FTS + path File SQL — no `ORDER BY id LIMIT` sampling.
- [ ] File nodes: seeds on **query** only; filtered from **`so graph search`**.
- [ ] Default output stays compact text; slim agent JSON (no full `nodes[]`).
- [ ] TRUNCATED allows `--budget` or narrow; snippet for known symbols.
- [ ] Test added/updated in `agent_ux_test.go` or focused `*_test.go`.
- [ ] Run `make test-native` if FTS/parser touched.

**Anti-patterns:** RAM-load all nodes for seeds; JSON-first query default; hook-style hit lists in query output.

Rules: [.agents/rules/graph-engine.mdc](../../.agents/rules/graph-engine.mdc)
