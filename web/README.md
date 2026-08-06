# Superopen Web (Next.js)

Local UI for sessions, Graphify HTML, and Map. **No Go API process.**

```bash
# from a repo with `.so/` (after `so init`)
so dev          # Next on :4444, SUPEROPEN_ROOT set
# or:
SUPEROPEN_ROOT=/path/to/repo npm run dev -- -p 4444 -H 127.0.0.1
```

Data is read/written under `$SUPEROPEN_ROOT/.so/` via App Router handlers in `src/app/api/`.
