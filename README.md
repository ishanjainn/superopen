
<p align="center">
  <a href="https://github.com/ishanjainn/superopen"><img src="./assets/brand-wordmark.png" width="50%" height="50%" alt="Superopen"/></a>

  <a href="https://github.com/ishanjainn/superopen"><img src="./assets/superopen-banner.svg" width="100%" height="100%" alt="Superopen"/></a>
</p>

Superopen is not another coding agent. It builds the open source harness around Claude Code, Cursor, Codex, and similar agents so every coding session improves the next with less token waste and lower cost.

- It builds a Tree-sitter graph into SQLite. Agents query a scoped subgraph instead of grepping files. No embeddings, no similarity search, no index to keep warm. The graph is a local so.db your agent reads via so graph query.
- Session hooks run in the background so every turn is recorded to .so/sessions/. Follow-ups skip re-exploration because the context is already there.
- Distill compresses what mattered into a short index (at most about 350 tokens) so the next session starts with signal, not noise (90.8% / 82% recall@10).
- Harvest proposes playbook patches from the session with reusable guidance you apply only when you want it.

**The numbers speak for themselves**. In our benchmarks, Claude Code with Superopen solves **4 times more tasks correctly** while cutting tokens, cost, and latency roughly in half:

| Metric | Cold Claude Code | Claude Code with Superopen | Improvement |
|---|---|---|---|
| Correctness | 1 / 5 (20%) | **4 / 5 (80%)** | **+60 pts** |
| Tokens | 19.1M | **9.6M** | **+50%** |
| Cost | $1.52 | **$0.87** | **+43%** |
| Tool calls | 81 | **44** | **+46%** |
| API requests | 82 | **45** | **+45%** |
| Wall-clock | 504s | **312s** | **+38%** |

Source: [BENCHMARKS.md](BENCHMARKS.md)

## Getting Started

Full walkthrough: [docs/installation.md](docs/installation.md).

```bash
brew install ishanjainn/superopen/so
so install
```

`so install` is user-global. It wires the `/so` skill, hooks, and graph-first guidance into every supported agent. It does not write files inside a repo. Add `--vendor=cursor` (or `claude-code`, `codex`, `gemini`, `opencode`, `copilot-cli`, `pi`) to install one agent only.

Then, in your repository:

```bash
so init         # or /so init in the agent chat
```

That is the whole setup. You get a `.so/` in that tree.

```text
.so/
  sessions/     # session events, transcripts, checkpoints
  db/so.db      # SQLite store: Graph + Memory
  .gitignore
```

## The Problem

Every session, your coding agent starts from zero. It greps, opens files, follows imports, backtracks, tries again, rebuilding a mental map of the codebase it already navigated yesterday and threw away. That rediscovery burns most of a run's tokens, tool calls, and latency, and it is pure overhead:

- **Repeated**: Every task pays the exploration cost again, from scratch.
- **Discarded**: Whatever the agent figured out dies with the session.
- **Unshared**: The next teammate (or your own next session) starts cold too.

Humans onboard to a codebase once. Agents onboard every single time.

## See it in action

After `so init`, agents ask these four surfaces instead of grepping and re-reading transcripts:

```
$ so graph query "how do session hooks steer Cursor?"
Traversal: BFS depth=2 | Start: [emitSteerContext HookReminder] | 8 nodes
NODE emitSteerContext [qn=internal.agent.hook.emitSteerContext src=internal/agent/hook/steer_context.go loc=L27-82]
EDGE emitSteerContext --CALLS --> steerDecisionFor at=internal/agent/hook/steer_context.go:L28
help[1]:
  so graph snippet internal.agent.hook.emitSteerContext

$ so memory recall "login timeout"
hits: 1  anti_hits: 0  budget: 1500
memories[1]{id,kind,title,tokens}:
  42,knowledge,login timeout is 30s,18
count: 1 of 1
#42  medium  2026-09-07  login timeout is 30s
Login timeout is 30s. Check the gateway before raising it.
help[2]:
  so memory get 42 --full
  so memory timeline --around 42

$ so memory distill --apply sess_abc
applied sess_abc via live written=1 → #42

$ so harvest list
proposals[1]{id,status,kind,target,title,plus,minus}:
  7,open,improve,AGENTS.md,prefer graph query before grep,12,0
count: 1 of 1
help[3]:
  so harvest show <id>
  so harvest apply <id>
  so harvest decline <id>

$ so harvest apply 7
applied #7 improve AGENTS.md
```

`query` is the code map. `recall` is the project diary (cite `#id`). Distill compresses a finished session into knowledge (`--brief` then `--apply`, or `[]` if nothing durable). Harvest stages a playbook diff until you `apply`. More: [graph](docs/graph.md), [memory](docs/memory.md), [harvest](docs/harvest.md).

## Prerequisites

| Requirement | Minimum | Check | Install |
|-------------|---------|-------|---------|
| Node.js (for `so dev` only) | 20+ | `node --version` | [nodejs.org](https://nodejs.org) |

## Uninstall

Works from any directory. No source checkout required.

```bash
so uninstall                 # agent wiring + project index + marketplace + caches + .so data
# --keep-data                # leave per-repo .so/ in place
# --vendor=cursor            # drop one vendor's hooks only
```

Then remove the **binary** the same way you installed it:

| How you installed `so` | Remove the binary |
|------------------------|-------------------|
| Homebrew (macOS / Linux) | `brew uninstall so` |
| Windows `install.ps1` / curl installer | already gone (`so uninstall` deletes `~/.superopen`) |

Restart the coding agent so it drops in-memory hooks.

## Learn more

- [Installation](docs/installation.md) - binary, `so install`, `so init`, upgrade, uninstall
- [Architecture](docs/architecture.md) - components, data flow, storage map
- [Commands](docs/commands.md) - full `so` CLI reference
- [Configuration](docs/configuration.md) - config file, env vars, flags, paths
- [Graph](docs/graph.md) - build, query, trace, impact
- [Sessions](docs/sessions.md) - how agent sessions are recorded
- [Memory](docs/memory.md) - project diary over sessions
- [Harvest](docs/harvest.md) - playbook patches gated on human apply
- [Troubleshooting](docs/troubleshooting.md) - install, PATH, hooks, UI, stale graph
- [Contributing](CONTRIBUTING.md) - local build from source
