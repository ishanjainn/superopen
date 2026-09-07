# Troubleshooting

Use this page when install, hooks, the graph, the UI, or memory do not behave as expected. Help text on the CLI is also authoritative: `so <command> --help`.

Related: [installation](installation.md), [configuration](configuration.md), [commands](commands.md), [graph](graph.md).

## `so: command not found`

The binary is installed, but your shell cannot find it.

**Homebrew (macOS / Linux)**

1. Confirm the formula: `brew list so`
2. Confirm the path: `brew --prefix so`
3. Confirm `$(brew --prefix)/bin` is on `PATH`
4. Open a new terminal after the first Homebrew install

**curl installer (macOS / Linux)**

`so` is written to `~/.superopen/bin`. The installer appends that directory to `.zprofile`, `.zshrc`, `.bash_profile`, and `.bashrc`. It cannot change the terminal you already have open.

```bash
export PATH="$HOME/.superopen/bin:$PATH"
so --help
```

Or open a new terminal.

**Windows `install.ps1`**

`so.exe` is written to `%USERPROFILE%\.superopen\bin` and added to your user PATH. Open a new PowerShell window.

If you built from a checkout with Go, run `sh scripts/install.sh` (or `powershell -File scripts/install.ps1`) so agents pin `~/.superopen/bin/so`, not `./bin/so`.

## PowerShell treats `/so init` as a path

PowerShell uses `/` as a path separator. In a terminal, run `so init` with no leading slash.

In agent chat, `/so init` is still the skill command. That is not a PowerShell path.

## `not a Superopen repo; run so init`

`so install` only wires coding agents on this machine. It does not create project data.

Each repository still needs `so init` once, from that repo's root. That command creates `.so/` and builds the graph.

After init, restart the agent so it sees the new directory.

Linked git worktrees of an inited parent can seed on the first `so graph query`. If that still prints the unmanaged message, run `so init` in the worktree.

## `so install` vs `so init`

| Command | How often | What it does |
|---------|-----------|--------------|
| `so install` | Once per machine (or after you replace the binary) | Writes `/so` skill, hooks, and guidance into user agent config. No files inside a repo. |
| `so init` | Once per repository | Creates `.so/`, writes `.so/.gitignore`, builds the graph, registers the project. |
| `so uninstall` | When you want Superopen gone | Reverse of install. Does not need a source checkout. |

`so init --force` rebuilds an existing graph. `so init --dev` starts `so dev -d` after init. `so init --cursor-rules` also writes `.cursor/rules/superopen.mdc` in that repo.

## Hooks do not fire

1. Run `so install` (add `--vendor=...` if you only want one agent).
2. Restart the coding agent. Claude Code, Cursor, Codex, Gemini CLI, OpenCode, Copilot CLI, and Pi load hooks on startup.
3. After you upgrade or move the `so` binary, run `so install` again so hook scripts keep the current path.
4. Confirm you are in an inited repo. Hooks stay quiet in unmanaged trees (no `.so/`).
5. Codex Desktop rejects PreToolUse `additionalContext`. On Codex, graph-first guidance lives in `AGENTS.md` and the skill, not in a PreToolUse nudge.

`so install --strict` (or `SUPEROPEN_HOOK_STRICT=1`) denies the first in-repo source Read once per session so the agent must query the graph first. Default install is a soft nudge, not a block.

## `so dev` fails (`node` or `server.js` missing)

The UI is a prebuilt Next.js standalone bundle. Runtime needs Node.js 20 or newer (`node --version`).

The web tree must exist at:

- curl / `install.ps1`: `~/.superopen/share/superopen/web`
- Homebrew: `$(brew --prefix so)/share/superopen/web`

If that directory is missing, reinstall Superopen. Graph query, sessions, and memory work without the UI.

`so dev` binds the current inited repo, or the last managed project. Override with `--root` or `SUPEROPEN_ROOT`. Default port is `4444`. Use `so dev -d` to detach, `so dev status` to check, `so dev stop` to stop.

`so dev --hot` needs UI sources (`SUPEROPEN_WEB_DIR`). Release users should not need that flag.

## Graph looks stale

Session hooks refresh the graph in the background on SessionStart and SessionEnd. On a large repo that can lag a few seconds.

```bash
so graph refresh          # incremental
so graph build --force    # full rebuild
so graph status           # build state
```

If refresh prints `build pool full`, another repo is building. Default pool size is 2. Set `SUPEROPEN_BUILD_SLOTS` (use `0` for unlimited). See [configuration](configuration.md).

After `git pull` or a large edit, run `so graph refresh` before asking the agent again.

`so graph query` skips unmanaged trees. It does not create `.so/` for you.

## Query output says `TRUNCATED`

The answer is under a token budget and a hard row cap (16 NODE rows and 16 EDGE rows by default). Superopen never drops the answer silently.

Narrow the question, or run `so graph snippet <qn>` for a NODE already listed. Raise the cap with `SUPEROPEN_GRAPH_QUERY_MAX_ROWS` only if you need a larger dump.

Do not pipe `so` through `head` or `tail`. That hides `TRUNCATED` and `help[]`.

## Graph skipped files you care about

Discovery skips `vendor/`, `node_modules/`, and similar generated trees, plus binary suffixes such as `.wasm`. Extra excludes come from `.soignore` in the repo root (gitignore syntax, including `!` negation).

`.so/` itself is listed in `.so/.gitignore` so it is not committed.

Symlinks that escape the repository are not indexed.

## Memory embeddings look weak or print a hash fallback warning

Keyword search always works (FTS). Optional BGE embeddings need Python 3 (`python3`, then `python`, then `py -3` on Windows) and a model under `~/.superopen/models/`. `so install` tries to fetch that model.

If Python is missing, Superopen falls back to hash embeddings and warns once. Graph builds do not use this path.

## Distill or harvest wants a network call

Graph builds never call a model.

`so memory distill` is the only memory path that may send a session digest off the machine, and only when that session's own agent CLI is logged in. Live agents use `so memory distill --brief <id>` then `so memory distill --apply <id>` (empty JSON array if nothing durable).

Harvest SessionEnd uses the same vendor CLI. If it is not authenticated, work stays pending for the next SessionStart (`HARVEST pending`). There is no cross-vendor fallback. Headless workers set `SUPEROPEN_HEADLESS=1` and are not recorded as sessions.

## Two copies of `so` on PATH

Homebrew puts `so` in the brew prefix. The curl installer puts it in `~/.superopen/bin`. If both exist, your shell may run the older one.

```bash
which -a so
so version
```

Uninstall the extra copy. After a binary change, run `so install` again.

## Agent still greps the whole tree

1. Confirm `.so/` exists in the workspace.
2. Restart the agent after `so install`.
3. Ask a codebase question and check that the agent ran `so graph query` first.
4. For a stronger gate: `so install --strict`.

Do not grep `.so/`. Session files and `so.db` are machine-local.

## Uninstall leftovers

```bash
so uninstall                 # hooks, skill, index, caches, registered .so data
# --keep-data                # leave per-repo .so/ in place
# --vendor=cursor            # drop one vendor's hooks only
```

Then remove the binary the same way you installed it (`brew uninstall so`, or nothing extra for curl / `install.ps1` because `so uninstall` deletes `~/.superopen`).

Restart the agent so it drops in-memory hooks.

Homebrew / Scoop / WinGet binaries are not deleted by `so uninstall`. Use that package manager after the command.

## Still stuck

```bash
so version
so status
so graph status
so graph diagnostics
so memory status
so harvest list
```

Include those outputs (no secrets, no session transcripts) when you open an issue.
