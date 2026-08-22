# Scripts

Installers and packaging helpers. Parent index: [../AGENTS.md](../AGENTS.md).

Benchmarks live in [../benchmarks/](../benchmarks/).

| Script | Purpose |
|--------|---------|
| `install.sh` / `install.ps1` | User-global `so` install |
| `uninstall.sh` / `uninstall.ps1` | Remove hooks and shared config |
| `pack-web.sh` | Bundle UI for release |
| `sync-plugins.sh` | Embed `.claude-plugin/` + `plugins/` into `internal/agent/install/marketplace/` |
| `so.rb` | Homebrew formula helper |

## Go tests

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

## Fixtures

- `internal/graph/engine/agent_ux_test.go`
- `internal/agent/hook/steer_context_test.go`
- `internal/agent/install/install_patch_test.go`

## CI

`.github/workflows/ci-cli.yml`, `ci-web.yml`

Rules: [.agents/rules/tests.mdc](../.agents/rules/tests.mdc)
