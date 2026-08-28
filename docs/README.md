# Superopen documentation

Superopen is an open source harness around coding agents (Claude Code, Cursor,
Codex, …). It is not an agent itself: a single local binary (`so`) gives every
session a code graph, a project diary, and observability, so the next session
starts smarter and cheaper.

## Docs

| Doc | Contents |
|-----|----------|
| [architecture.md](architecture.md) | System components, data flow, storage map |
| [commands.md](commands.md) | Full `so` CLI reference |
| [graph.md](graph.md) | Native code graph: build, query, trace, impact |
| [sessions.md](sessions.md) | Observability: how agent sessions are recorded and materialized |
| [memory.md](memory.md) | Project diary over sessions: capture, search, distill |
| [harvest.md](harvest.md) | Playbook patches staged for human approval |

## Quick start

```bash
brew install ishanjainn/superopen/so   # or the curl/release installer
so install                             # /so skill + hooks + guidance (user-global)

cd your-repo
so init                                # creates .so/ in that repo
```

## The loop

1. **Harness** (`so install`) wires hooks into your agent config — user-global,
   never inside a repo.
2. A repo becomes managed when `so init` creates `.so/`.
3. **Graph**: structural questions go to `so graph query` instead of grep.
4. **Sessions** (`so sessions`) record what happened while you worked.
5. **Memory** (`so memory`) distills sessions into searchable prior work.
6. **Harvest** proposes instruction-file patches; you approve before anything
   becomes always-on.
