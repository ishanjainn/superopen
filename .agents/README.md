# `.agents/` — shared agent config (this repo only)

Contributor-facing rules and pointers. **Not** installed by `so install`.

## Layout

```text
.agents/
  README.md           # this file
  rules/              # scoped rules (Cursor .mdc format; usable as plain markdown elsewhere)
    repo.mdc          # always apply in this checkout
    graph-engine.mdc
    cli.mdc
    memory.mdc
    agent-harness.mdc
    ui.mdc
    tests.mdc
```

Detailed instructions live in **nested `AGENTS.md`** next to the code (`internal/graph/AGENTS.md`, `web/AGENTS.md`, …). Rules point agents at those files when matching paths are touched.

## Cursor

`.cursor/rules` is a symlink to `.agents/rules` so Cursor and other tools share one source of truth.

## Other agents

- **Claude Code / Codex:** read root `AGENTS.md` and the nested `AGENTS.md` for your working directory; optional: merge `.agents/rules/*.mdc` into project instructions.
- **CI:** no automatic load — cite `AGENTS.md` in workflow docs if needed.

## End-user product

Customer repos get `/so` skill + hooks from `so install`, defined under `internal/agent/`. Do not confuse with this tree.
