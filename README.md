# Superopen

One CLI to rule them all.

## Install (user-global, any directory)

Works on **Linux, macOS, and Windows** across supported coding agents
(Claude Code, Cursor, Codex, Gemini CLI, OpenCode, Copilot CLI, Pi).

```bash
brew install ishanjainn/superopen/so   # or the curl/release installer
so install                            # /so skill + hooks + guidance
```

The curl/`install.ps1` installer puts `so` in `~/.superopen/bin`, the prebuilt Sessions/Memory/Graph UI in `~/.superopen/share/superopen/web` (`so-web.tar.gz` from the GitHub Release, same as the CLI binary), and adds `bin` to PATH (new terminals; the current shell needs `export PATH="$HOME/.superopen/bin:$PATH"` or a new tab). Homebrew installs that same UI bundle under its prefix `share/superopen/web`. Neither path runs `npm install` / `next build` on your machine. `so dev` uses that prefix from any repo; it does not look up a Superopen git clone. Running the UI needs [Node.js](https://nodejs.org/) on PATH (`node server.js`); Homebrew installs Node as a dependency.

`so install` writes into each agent’s **user** skill/plugin/config directories
(OS-agnostic home / XDG / `%APPDATA%` / `%LOCALAPPDATA%`). It is not tied to
the Superopen source repo (users never need that) and not tied to the current
working tree. It installs:

- the `/so` skill
- observability hooks
- durable graph-first guidance (user-level instruction surfaces)

That wiring is **capability on this machine**. A repository is managed only after
`so init` creates `.so/` in that tree. Opening other clones in a coding agent
does not initialize them, does not write `.so/`, and does not expose Superopen
hook context (so agents do not spend tokens on Superopen there).

A teammate who never ran `so install` is unaffected: nothing Superopen-specific
is required in git besides an optional `.so/.gitignore`. There are no git hooks.

## Initialize a repository

In a coding agent (after `so install`), or from a shell inside the repo:

```bash
so init          # or /so init in the agent (only when the user asks)
```

Defaults to the **repository root** (nearest existing `.so` or git top-level).
Use `--root` / `SUPEROPEN_ROOT` for an explicit nested package graph.

Agents must **not** run `so init` just because `.so/` is missing.

Creates:

```text
.so/
  sessions/      # observability sessions (gitignored)
  db/so.db       # shared Superopen SQLite store (gitignored)
  .gitignore
```

Registers the repo in the user-wide project index under the Superopen config
dir (`~/.config/superopen` / `%APPDATA%\superopen`).

## Native graph (automatic for agents in inited repos)

After install + init **in that repository**, coding agents are steered to use the graph for structural
questions without the user saying `/so`. Repositories without `.so/` stay unmanaged.

```bash
so graph build
so graph refresh              # skip when unchanged, or when .so/ is missing; --force for full rebuild
so graph search DataFlowingGate
so graph query "How does DataFlowingGate gate the UI?"
so graph architecture
so graph impact DataFlowingGate
```

Session hooks refresh the graph in the background on SessionStart / SessionEnd
(detached, fail-open) **only if the workspace already has `.so/`**. Builds are **local** (Tree-sitter + SQLite) — they do not
invoke an LLM or the live coding agent.

Default `so graph query` stdout is compact NODE/EDGE text plus AXI `help[]` next steps. `--json` and `--full` are script escape hatches.

## Sessions, memory, and UI

```bash
so sessions list
so sessions show <id>
so sessions finalize <id>
so memory search "login bug"
so memory get 12
so memory capture --request "…" --learned "…" --next "…"
so projects                   # repos where Superopen has been used
so dev                        # UI from any directory; binds the current inited repo or last managed project
so dev -d                     # detached UI
```

`so dev` does not require cwd to be an inited repo: if this folder has no `.so/`, it uses the active
(or most recently seen) Superopen-managed project. `so init` is still required once per repo you want managed.

## Layout summary

| Location | Purpose |
|----------|---------|
| User skill dirs | `/so` skill from `so install` |
| User instruction surfaces | Graph-first durable guidance |
| `<repo>/.so/sessions` | Session documents |
| `<repo>/.so/db/so.db` | Shared DB (graph + memory) |
| Config dir `projects.json` | Cross-repo index of Superopen usage |

One `so` binary includes the native graph engine. There is no separate graph binary.

Contributors: read [`AGENTS.md`](AGENTS.md) and the nested `AGENTS.md` in the area you edit; shared rules in [`.agents/rules/`](.agents/rules/). Repo-only — not what `so install` writes to customer projects.

## Uninstall

Works from any directory. No source checkout required.

```bash
so uninstall                 # agent wiring + project index + marketplace + caches + .so data
# --keep-data                # leave per-repo .so/ in place
```

Then remove the **binary** the same way you installed it:

| How you installed `so` | Remove the binary |
|------------------------|-------------------|
| Homebrew (macOS / Linux) | `brew uninstall so` |
| Windows `install.ps1` / curl installer | already gone (`so uninstall` deletes `~/.superopen`) |
| Scoop / WinGet / Chocolatey | `scoop uninstall so` / `winget uninstall so` / `choco uninstall so` |

Restart the coding agent so it drops in-memory hooks.

Building from source (developers only): [docs/local-build.md](docs/local-build.md).
