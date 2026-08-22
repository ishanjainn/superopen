# Contributing to Superopen

Thanks for helping improve Superopen. Contributions of code, documentation, bug
reports, and integration feedback are all welcome.

## Before you start

- Read the [README](README.md) for the Homebrew install and product overview.
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md).
- For a substantial change, open or discuss an issue first so maintainers can
  confirm the scope. Small documentation fixes can go straight to a pull
  request.

## Local build

End users never clone this repo. They get a built `so` via Homebrew or the
release installer in the [README](README.md).

Local development uses **those same scripts**. The checkout is only the source
to compile; the installed CLI lives in `~/.superopen/bin`, same as production
curl / `install.ps1`.

Requires [Go](https://go.dev/dl/) (see `go.mod`).

```bash
git clone https://github.com/ishanjainn/superopen.git
cd superopen
```

Forks use the same repository name: `git clone https://github.com/<your-user>/superopen.git`.

### Install (production layout)

```bash
sh scripts/install.sh                 # macOS / Linux
# powershell -File scripts/install.ps1  # Windows
```

That:

1. Builds into `~/.superopen/bin/so` (`so.exe` on Windows)
2. Installs the Sessions/Memory/Graph UI into `~/.superopen/share/superopen/web` as the same Next standalone tree curl users get (`npm install --ignore-scripts` + production build, then `scripts/pack-web.sh`)
3. Runs `so install` (hooks, skill, guidance for every supported agent)
4. Puts `~/.superopen/bin` on PATH in shell rc files (Windows: user PATH)

`so dev` in any inited repo uses that prefix. It does not search for a Superopen git clone.

`sh scripts/install.sh` cannot change the terminal you already have open. After it finishes:

```bash
export PATH="$HOME/.superopen/bin:$PATH"
so --help
```

or open a new terminal.

Then, in a **test repo** (not this checkout):

```bash
so init          # or /so init in a coding agent
```

`make install` is the same as `sh scripts/install.sh`.

Re-run the install script after CLI changes so `~/.superopen/bin/so` and
agent-pinned paths stay in sync. Do not `go install` or pin `./bin/so` into
hooks — that is a different prefix than production.

### Uninstall (production command)

```bash
sh scripts/uninstall.sh               # macOS / Linux
# powershell -File scripts/uninstall.ps1
# make uninstall
```

That runs `so uninstall`: agent wiring, project index, marketplace, caches,
registered `.so/` data, and the `~/.superopen` prefix.

It does not remove a Homebrew / Scoop / WinGet binary. If you also have one of
those, uninstall it with that manager after the script.

Restart the coding agent after uninstall.

### Compile and test without touching the install

```bash
make build          # writes ./bin/so for local smoke tests
make test
make smoke          # optional; writes a local .so/ under the current directory
```

`./bin/so` is a compile artifact. Agents use `~/.superopen/bin/so` from the
install script.

### Web UI

```bash
cd web
npm ci --ignore-scripts
npm run typecheck
npm test
npm run lint
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
