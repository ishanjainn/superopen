# Configuration

Superopen is a local CLI. There is no server and no required API key. Most people never need this page: `so install` then `so init` is enough.

This document lists every supported switch: the config file, environment variables, CLI flags, paths, ignore files, and retention.

Related: [installation](installation.md), [troubleshooting](troubleshooting.md), [commands](commands.md), [architecture](architecture.md).

## Precedence

For values that exist in more than one place:

1. CLI flags for that command
2. Environment variables (`SUPEROPEN_*`, plus a few `SO_*` aliases)
3. `config.env` on disk
4. Built-in defaults

A real shell environment variable always wins over the same key in `config.env`.

## Config file

Path:

- macOS / Linux: `~/.config/superopen/config.env` (or `$XDG_CONFIG_HOME/superopen/config.env`)
- Windows: `%APPDATA%\superopen\config.env`

The file is `KEY=VALUE` lines, mode `0600`. Comments start with `#`. Unknown keys are ignored. Only this allow-list is read:

| Key | Default | Meaning |
|-----|---------|---------|
| `SUPEROPEN_SESSION_RETENTION_HOURS` | `168` (7 days) | Age after which session transcripts and closed harvest history are deleted. `0` keeps them forever. |
| `SUPEROPEN_MEMORY_RETENTION_HOURS` | `168` | Age after which unpinned prompts and session rollups are deleted. `0` keeps them forever. Teachings, pins, and `never_decay` rows are never deleted by age. |
| `SUPEROPEN_HOOK_STRICT` | unset | `1` / `true` / `on`: deny the first in-repo source Read once per session (same as `so install --strict`). |
| `SUPEROPEN_BUILD_SLOTS` | `2` | How many graph builds may run at once across repos. `0` means unlimited. |
| `SUPEROPEN_CODING_REPO_ALLOWLIST` | unset | Optional allow-list consumed by hook adapters. Empty means no extra filter. |
| `SUPEROPEN_ENVIRONMENT` | `default` | Local telemetry resource attribute. |
| `SUPEROPEN_APPLICATION_NAME` | `so-cli` | Local telemetry resource attribute. |

You can also persist retention with `so gc`:

```bash
so gc --show
so gc --sessions-hours 168 --memory-hours 168
so gc --sessions-hours 0 --memory-hours 0   # keep forever
```

`--sessions-hours` / `--memory-hours` write the config file, then `so gc` with no extra flags applies the policy to the current inited repo.

Open harvest proposals, pending harvest runs, teachings, pins, and the code graph are never aged out. Checkpoints live inside session folders and go with the session.

## Global CLI flags

These flags work on every `so` command:

| Flag | Env | Meaning |
|------|-----|---------|
| `--json` | `SO_JSON` or `SUPEROPEN_JSON` | JSON envelope `{ok, kind, data\|items, count, next}` |
| `--full` | `SO_FULL` or `SUPEROPEN_FULL` | Do not truncate string fields |
| `--root` | `SUPEROPEN_ROOT` | Repository root. Default: walk up to git top-level or nearest `.so/` |

Env values that count as true: `1`, `true`, `yes`, `on`, `json`.

Exit codes stay stable: `0` ok, `1` error, `2` usage, `3` not found, `4` continuation.

## Install and init flags

**`so install`** (user-global, any directory)

| Flag | Meaning |
|------|---------|
| `--vendor` | One or more of `claude-code`, `cursor`, `codex`, `gemini`, `opencode`, `copilot-cli`, `pi`. Default: all. |
| `--strict` | Deny the first in-repo source Read once per session. |

**`so init`** (per repository)

| Flag | Meaning |
|------|---------|
| `--force` | Rebuild the graph even if `.so/db/so.db` already exists. |
| `--dev` | After init, start `so dev -d`. |
| `--cursor-rules` | Write `.cursor/rules/superopen.mdc` in this repo. |
| `--root` | Nested package graph (otherwise nearest `.so/` or git top-level). |

**`so uninstall`**

| Flag | Meaning |
|------|---------|
| `--keep-data` | Leave per-repo `.so/` in place. |
| `--vendor` | Remove one vendor's hooks (`all` drops every vendor hook without a full wipe). |
| `--dry-run` | Print what would be removed. |

Does not remove a Homebrew / Scoop / WinGet binary.

## Other environment variables

These are not stored in `config.env` (except where noted above). Set them in the shell, a systemd unit, or your agent host.

| Variable | Used for |
|----------|----------|
| `SUPEROPEN_SO_BIN` | Absolute path to `so` when `PATH` does not have it. The installed skill also pins the path from `so install`. |
| `SUPEROPEN_WEB_DIR` | Override UI sources for `so dev --hot`. Release installs do not need this. |
| `SUPEROPEN_GRAPH_QUERY_MAX_ROWS` | NODE/EDGE row cap for `so graph query` (default `16`). |
| `SUPEROPEN_HEADLESS` | Set to `1` by distill/harvest workers so they are not recorded as sessions. |
| `SUPEROPEN_GRAPH_SOURCE` | Maintainer-only: engine source tree for `tools/` asset CLIs. |
| `SUPEROPEN_INSTALL_DIR` | curl / `install.ps1` target (default `~/.superopen/bin`). |
| `SUPEROPEN_VERSION` | Installer release tag without the `cli-` prefix. Default `latest`. |
| `SUPEROPEN_REPO` | GitHub `owner/repo` for the installer. Default `ishanjainn/superopen`. |
| `SO_DEV_NO_OPEN` | `1`: `so dev` does not open a browser. |
| `XDG_CONFIG_HOME` | Config root on Unix (and OpenCode / Superopen when set). |
| `XDG_DATA_HOME` | Data root on Unix (marketplace copy). |
| `CLAUDE_CONFIG_DIR` | Claude Code config root (default `~/.claude`). |
| `CODEX_HOME` | Codex config root (default `~/.codex`). |
| `COPILOT_HOME` | Copilot CLI config root (default `~/.copilot`). |

Installer notes: `scripts/install.sh` and `scripts/install.ps1` also honor those `SUPEROPEN_INSTALL_DIR` / `VERSION` / `REPO` values.

## Paths on disk

**Per repository** (created by `so init`):

```text
.so/
  .gitignore    # sessions/, db/, harvest/ (do not commit)
  sessions/     # raw spans, events.jsonl, checkpoints
  db/so.db      # SQLite: graph tables and memory tables
  db/so.db.key  # AES key for sealed memory episode bodies (mode 0600)
  harvest/      # staged playbook proposals
```

**Per machine**

| Location | Typical path | Contents |
|----------|--------------|----------|
| Config | `~/.config/superopen` or `%APPDATA%\superopen` | `config.env`, `projects.json` (cross-repo index) |
| Data | `~/.local/share/superopen` or `%LOCALAPPDATA%\superopen` | Marketplace copy used by `so install` |
| Prefix | `~/.superopen` (curl / `install.ps1`) or Homebrew prefix | `bin/so`, `share/superopen/web`, optional `models/` |
| Cache / runtime | OS cache dir `superopen/runtime/<hash>` | `so dev` pid/log files |

A repo without `.so/` stays unmanaged: no hook context and no graph spend there.

## Graph ignore rules

Put a `.soignore` in the repository root. Syntax matches `.gitignore`, including `!` negation. Patterns are applied after built-in skips (`vendor/`, `node_modules/`, binary suffixes such as `.wasm`, and a few package manifests that are not source).

`so graph build` / `refresh` also accept exclude patterns on the engine request path. Agents should prefer `.soignore` so every developer gets the same index.

`.gitignore` is not merged into `.soignore`. Files git already ignores are still walked unless they match a built-in skip or `.soignore`. If a generated tree must stay out of the graph, add it to `.soignore`.

## Graph query knobs

Defaults (see [graph](graph.md)):

- Compact `NODE` / `EDGE` text
- Token budget about 1200
- At most 16 NODE rows and 16 EDGE rows unless `SUPEROPEN_GRAPH_QUERY_MAX_ROWS` is set
- File/Module snippets clip at about 500 lines

`so graph --no-refresh` skips the query-path freshness check (CI).

Concurrent builds: `SUPEROPEN_BUILD_SLOTS` (default 2). Status `pool_full` means wait or raise the cap.

## Memory knobs

Ranking defaults live in Go. A store may hold explicit overrides in `memory_meta` (`stale_weight`, `supersede_window`, `pin_weight`, `recall_budget`, `recency_half_life`). Product ranking keys are not seeded on new stores.

SessionStart memory injection is capped at about 350 tokens. That is not configurable.

Optional dense embeddings use Python 3 and `~/.superopen/models/bge-small-en-v1.5-int8`. If Python never starts, search still works with hash embeddings.

The FTS index stores plaintext of episode bodies so keyword search works. `memory_episodes.text` is AES-GCM sealed. Anyone with filesystem access to `.so/db/` can read the FTS sidecar.

## Agent host config

`so install` writes user-global files. It does not write into a repo unless you pass `so init --cursor-rules`.

| Agent | Typical user-global location |
|-------|------------------------------|
| Claude Code | `~/.claude` (or `CLAUDE_CONFIG_DIR`) |
| Cursor | `~/.cursor/hooks.json` and `~/.cursor/rules/superopen.mdc` |
| Codex | `~/.codex` (or `CODEX_HOME`) |
| Gemini CLI | Gemini user config |
| OpenCode | `~/.config/opencode` |
| Copilot CLI | `~/.copilot` (or `COPILOT_HOME`) |
| Pi | Pi user config |

Restart the agent after install or uninstall.

## Privacy in one place

- Graph builds: Tree-sitter + SQLite. No LLM. No network.
- Session hooks: local files under `.so/sessions/`. Prompts and tool args are redacted. VCS revision metadata is not.
- Memory distill: may send a session digest to the session vendor's CLI when that CLI is logged in. Opt-in. No cloud memory by default.
- Query logging of graph questions is not sent anywhere.

See [memory](memory.md) and [sessions](sessions.md) for retention details.
