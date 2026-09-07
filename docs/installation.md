# Installation

Install Superopen in three steps:

1. Put the `so` binary on your machine.
2. Run `so install` once so your coding agent gets the `/so` skill and hooks.
3. Run `so init` once in each repository you want Superopen to manage.

There is no server and no API key. Graph builds stay on your machine.

Related: [troubleshooting](troubleshooting.md), [configuration](configuration.md), [commands](commands.md), [contributing](../CONTRIBUTING.md).

## Prerequisites

| Requirement | Minimum | Check | Notes |
|-------------|---------|-------|-------|
| Homebrew (recommended, macOS / Linux) | any | `brew --version` | [brew.sh](https://brew.sh) |
| curl + tar (Linux alternative) | any | `curl --version` | Usually already present |
| PowerShell (Windows) | 5+ | `$PSVersionTable` | Built-in |
| Node.js (for `so dev` only) | 20+ | `node --version` | Not needed for graph, sessions, or memory |
| Coding agent | Claude Code, Cursor, Codex, Gemini CLI, OpenCode, Copilot CLI, or Pi | - | Install the agent first |

Python 3 is optional. It is only used for denser memory embeddings. Go is only needed if you build from source.

Supported release binaries: macOS and Linux (`amd64`, `arm64`), Windows (`amd64`, `arm64`).

## 1. Install the `so` binary

Pick one method. Do not mix Homebrew and the curl installer on the same machine unless you know which `so` your `PATH` will run (`which -a so`).

### macOS (Homebrew)

```bash
brew install ishanjainn/superopen/so
so version
```

Homebrew puts `so` in `$(brew --prefix)/bin`. After the first Homebrew install, open a new terminal if `so` is not found.

### Linux (Homebrew or curl)

```bash
brew install ishanjainn/superopen/so
```

Or, without Homebrew:

```bash
curl -fsSL https://raw.githubusercontent.com/ishanjainn/superopen/main/scripts/install.sh | sh
```

The curl installer downloads the latest GitHub Release (`so-linux-amd64.tar.gz` or `so-linux-arm64.tar.gz`, plus `so-web.tar.gz`), writes `so` to `~/.superopen/bin`, installs the prebuilt UI under `~/.superopen/share/superopen/web`, runs `so install`, and appends `~/.superopen/bin` to `.zprofile`, `.zshrc`, `.bash_profile`, and `.bashrc`.

That process cannot change the terminal you already have open:

```bash
export PATH="$HOME/.superopen/bin:$PATH"
so --help
```

Or open a new terminal.

Pin a release (tag without the `cli-` prefix):

```bash
SUPEROPEN_VERSION=1.2.0 curl -fsSL https://raw.githubusercontent.com/ishanjainn/superopen/main/scripts/install.sh | sh
```

### Windows (PowerShell)

```powershell
iwr -useb https://raw.githubusercontent.com/ishanjainn/superopen/main/scripts/install.ps1 | iex
```

From a git checkout:

```powershell
powershell -File scripts/install.ps1
```

`so.exe` lands in `%USERPROFILE%\.superopen\bin` and is added to your user PATH. The prebuilt UI goes under `%USERPROFILE%\.superopen\share\superopen\web`. Open a new PowerShell window after install.

In a terminal, run `so init` with no leading slash. PowerShell treats `/so` as a path. In agent chat, `/so init` is still the skill command.

### Installer environment variables

These apply to `scripts/install.sh` and `scripts/install.ps1`:

| Variable | Default | Meaning |
|----------|---------|---------|
| `SUPEROPEN_INSTALL_DIR` | `~/.superopen/bin` | Where the binary is written |
| `SUPEROPEN_VERSION` | `latest` | Release tag without the `cli-` prefix |
| `SUPEROPEN_REPO` | `ishanjainn/superopen` | GitHub `owner/repo` for the download |

The curl installer needs `curl`, `tar`, and `uname`. It verifies sha256 when a sidecar file is present. Exit code `1` means unsupported OS/arch, a network failure, or a missing command.

## 2. Wire your coding agent (`so install`)

The Homebrew formula does not run this for you. The curl / `install.ps1` path already ran it once. Run it again after you upgrade the binary, or if you installed with Homebrew.

```bash
so install
```

This is user-global. It does not write files inside a repository. It installs:

- the `/so` skill
- observability hooks
- durable graph-first guidance

Default: every supported agent. Limit to one:

```bash
so install --vendor=cursor
# also: claude-code, codex, gemini, opencode, copilot-cli, pi
```

Stricter graph-first gate (denies the first in-repo source Read once per session):

```bash
so install --strict
```

Same effect as `SUPEROPEN_HOOK_STRICT=1`. See [configuration](configuration.md).

Restart the agent so it loads the new hooks. After you replace the `so` binary, run `so install` again so hook scripts keep the current path.

| Agent | Typical user-global location |
|-------|------------------------------|
| Claude Code | `~/.claude` (or `CLAUDE_CONFIG_DIR`) |
| Cursor | `~/.cursor/hooks.json` and `~/.cursor/rules/superopen.mdc` |
| Codex | `~/.codex` (or `CODEX_HOME`) |
| Gemini CLI | Gemini user config |
| OpenCode | `~/.config/opencode` |
| Copilot CLI | `~/.copilot` (or `COPILOT_HOME`) |
| Pi | Pi user config |

Codex Desktop rejects PreToolUse `additionalContext`. On Codex, always-on guidance lives in `AGENTS.md` and the skill.

## 3. Init a repository (`so init`)

From the repo root:

```bash
so init
```

Or `/so init` in the agent chat.

This creates `.so/`, writes `.so/.gitignore`, builds the graph, and registers the project in the machine-wide index.

```text
.so/
  .gitignore    # sessions/, db/, harvest/ (do not commit)
  sessions/     # session events, transcripts, checkpoints
  db/so.db      # SQLite: graph + memory
```

| Flag | Meaning |
|------|---------|
| `--force` | Rebuild even if `.so/db/so.db` already exists |
| `--dev` | After init, start `so dev -d` |
| `--cursor-rules` | Also write `.cursor/rules/superopen.mdc` in this repo |
| `--root` | Nested package graph (otherwise nearest `.so/` or git top-level) |

A repo without `.so/` stays unmanaged. Hooks stay quiet there. `so graph query` prints `not a Superopen repo; run so init`.

Linked git worktrees of an inited parent can seed on the first graph query. If that still prints the unmanaged message, run `so init` in the worktree.

## Verify

```bash
so version
so install          # if you have not already
cd your-repo && so init
so graph status
so dev              # optional UI; needs Node.js 20+
```

Ask the agent a codebase question. It should run `so graph query` before grepping.

## Upgrade

**Homebrew**

```bash
brew update
brew upgrade so
so install
```

**curl / install.ps1**

Re-run the same installer. It overwrites `~/.superopen/bin/so` (or `so.exe`) and the web bundle, then runs `so install`.

Always run `so install` after the binary changes so agent hooks keep the new path. Restart the agent.

## Uninstall

Works from any directory. No source checkout required.

```bash
so uninstall                 # agent wiring + project index + marketplace + caches + .so data
# --keep-data                # leave per-repo .so/ in place
# --vendor=cursor            # drop one vendor's hooks only
# --dry-run                  # print what would be removed
```

Then remove the binary the same way you installed it:

| How you installed `so` | Remove the binary |
|------------------------|-------------------|
| Homebrew (macOS / Linux) | `brew uninstall so` |
| Windows `install.ps1` / curl installer | already gone (`so uninstall` deletes `~/.superopen`) |

Restart the coding agent so it drops in-memory hooks.

`so uninstall` does not remove a Homebrew / Scoop / WinGet binary. Use that package manager after the command.

From a source checkout, `sh scripts/uninstall.sh` (or `powershell -File scripts/uninstall.ps1`, or `make uninstall`) runs the same `so uninstall`.

## Build from source

End users should not need this. Contributors: see [CONTRIBUTING.md](../CONTRIBUTING.md).

Short version: clone the repo, install Go (see `go.mod`), then:

```bash
sh scripts/install.sh                 # macOS / Linux
# powershell -File scripts/install.ps1  # Windows
```

That builds into `~/.superopen/bin/so` (the same prefix as the curl installer), packs the UI, runs `so install`, and updates PATH. `make install` is the same command.

Do not `go install` or pin `./bin/so` into hooks. Agents must use `~/.superopen/bin/so`.

## Next

- [configuration.md](configuration.md) for `config.env`, flags, and paths
- [troubleshooting.md](troubleshooting.md) if `so` is not on PATH or hooks do not fire
- [graph.md](graph.md) after `so init`
