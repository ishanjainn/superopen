# UI (`so dev`)

Next.js app — Sessions, Memory, Graph map. Release bundle via `scripts/pack-web.sh`.

Parent index: [../AGENTS.md](../AGENTS.md). Memory API: [../internal/memory/AGENTS.md](../internal/memory/AGENTS.md).

## Run

```bash
so dev
cd web && npm ci --ignore-scripts && npm run dev   # hot reload
```

## Layout

| Path | Role |
|------|------|
| `src/app/` | Pages |
| `src/app/api/` | BFF → `so` / SQLite |
| `src/lib/so/` | Exec helpers |
| `src/map/` | Graph viz (Three.js) |

Same `.so/db/so.db` as CLI — keep API shapes stable.

## Tests

```bash
make test-web
make lint
```

## Constraints

- Match existing Radix + Tailwind patterns.
- No secrets in client bundle.
- Release uses `npm ci --ignore-scripts`.

## Change checklist

- [ ] Prefer `src/lib/so/` exec helpers for new routes.
- [ ] `npm run typecheck` + `npm test`
- [ ] Breaking API changes documented in PR.

Rules: [.agents/rules/ui.mdc](../.agents/rules/ui.mdc)
