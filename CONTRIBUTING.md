# Contributing to Superopen

Thanks for helping improve Superopen. Contributions of code, documentation, bug
reports, and integration feedback are all welcome.

## Before you start

- Read the [README](README.md) for the Homebrew install and product overview.
- Working from a git checkout? See [docs/local-build.md](docs/local-build.md) to build `so`, wire hooks, and wipe install state for a clean retest.
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md).
- For a substantial change, open or discuss an issue first so maintainers can
  confirm the scope. Small documentation fixes can go straight to a pull
  request.

## Development setup

```bash
git clone https://github.com/YOUR_USERNAME/so.git
cd so

# CLI — production-shaped install (see docs/local-build.md)
go test ./...
sh scripts/install.sh          # Windows: powershell -File scripts/install.ps1

# Web UI
cd web
npm ci --ignore-scripts
npm run typecheck
npm test
npm run lint
```

Optional smoke (writes a local `.so/` under the current directory):

```bash
make smoke
```

## Development workflow

1. Fork the repository and clone your fork.
2. Create a focused branch: `git switch -c fix/short-description`
3. Make the change, including tests and documentation when they are relevant.
   Keep unrelated formatting and refactors out of the pull request.
4. Run the focused checks for what you changed:

   | Area | Checks |
   |---|---|
   | Go / CLI | `go test -race -timeout 30m -count=1 ./...` · `go vet ./...` |
   | Web UI | `cd web && npm test && npm run typecheck && npm run lint` |
   | Plugins | `bash scripts/sync-plugins.sh` then commit marketplace drift |

5. Commit with a clear subject, push, and open a pull request against `main`.

## Pull requests

- Explain the problem and the solution; link the related issue when one exists.
- Prefer Conventional Commit titles, for example `feat: add project prune` or
  `fix(web): restore file search`.
- Include tests for behavior changes and update user-facing documentation.
- Complete the pull-request template honestly.

## Issues and questions

Search existing [issues](https://github.com/ishanjainn/superopen/issues) before opening
a new one. Bug reports should include reproducible steps, expected and actual
behavior, and relevant non-sensitive logs.

Do not report security vulnerabilities in a public issue; follow
[SECURITY.md](SECURITY.md) instead.
