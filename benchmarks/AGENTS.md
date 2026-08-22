# Benchmarks

Agent eval harness and init timing scripts. Parent index: [../AGENTS.md](../AGENTS.md).

## Layout

```
benchmarks/
├── AGENTS.md              # this file
├── bench-init.sh          # so init wall-time + graph counts for a repo
└── agent-graph-eval/      # isolated Claude Code eval (vanilla vs superopen)
    ├── run_eval.py        # main harness
    ├── test_harness.py    # unit tests (no Claude)
    ├── questions/         # task + grading fixtures
    ├── work/              # gitignored eval workspaces
    ├── cache/             # gitignored shallow repo mirrors
    └── results/           # gitignored run outputs (use new stamp dirs)
```

| Path | Purpose |
|------|---------|
| [agent-graph-eval/](agent-graph-eval/) | Isolated Claude Code eval (vanilla vs Superopen) |
| [bench-init.sh](bench-init.sh) | Record `so init` wall time and graph counts for a repo |

## Quick commands

```bash
python3 benchmarks/agent-graph-eval/test_harness.py

go build -o /tmp/so ./cmd/so
python3 benchmarks/agent-graph-eval/run_eval.py \
  --repo grafana \
  --so-bin /tmp/so \
  --arms vanilla,superopen
```

Retrieval microbenches (no Claude): `go test -bench=. -benchmem ./internal/memory/`

Do not overwrite `benchmarks/agent-graph-eval/results/*` — use new stamp dirs.
