# Local build (developers)

End users never clone this repo. They get a built `so` via Homebrew or the
release installer in the [README](../README.md).

Local development uses **those same scripts**. The checkout is only the source
to compile; the installed CLI lives in `~/.superopen/bin`, same as production
curl / `install.ps1`.

Requires [Go](https://go.dev/dl/) (see `go.mod`).

## Install (production layout)

```bash
sh scripts/install.sh                 # macOS / Linux
# powershell -File scripts/install.ps1  # Windows
```

That:

1. Builds into `~/.superopen/bin/so` (`so.exe` on Windows)
2. Installs the Sessions/Memory/Graph UI into `~/.superopen/share/superopen/web` (npm install --ignore-scripts + production build)
3. Runs `so install` (hooks, skill, MCP, guidance for every supported agent)
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

## Uninstall (production command)

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

## Compile/test without touching the install

```bash
make build          # writes ./bin/so for local smoke tests
make test
```

`./bin/so` is a compile artifact. Agents use `~/.superopen/bin/so` from the
install script.
