# Scripts, tests, eval

Parent index: [../AGENTS.md](../AGENTS.md).

## Go

```bash
make test              # go test -race ./...
make test-native       # tsnative + sqlite_fts5
make lint
```

| Area changed | Package tests |
|--------------|---------------|
| Graph | `./internal/graph/engine/` (+ `test-native`) |
| Harness | `./internal/agent/hook/ ./internal/agent/steer/ ./internal/agent/skills/` |
| CLI | `./cmd/so/... ./internal/cli/` |
| Memory | `./internal/memory/` |

## Web

```bash
make test-web
```

## Benchmarks

Protocol: [agent-graph-eval/AGENTS.md](agent-graph-eval/AGENTS.md) (isolation, Claude Code host, natural product install — no trimmed skills/hooks/MCP). Invoke: [agent-graph-eval/README.md](agent-graph-eval/README.md).

```bash
python3 scripts/agent-graph-eval/test_harness.py
```

Do not overwrite `agent-graph-eval/results/*` — use new stamp dirs.

## Fixtures

- `internal/graph/engine/agent_ux_test.go`
- `internal/agent/hook/steer_context_test.go`
- `internal/agent/install/install_patch_test.go`

## CI

`.github/workflows/ci-cli.yml`, `ci-web.yml`

Rules: [.agents/rules/tests.mdc](../.agents/rules/tests.mdc)
