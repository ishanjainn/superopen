# pr-check

Pre-submit checklist for Superopen pull requests.

## Before opening a PR

1. **Scope** — Diff is focused; no unrelated refactors or formatting sweeps.
2. **Tests** — Behavior changes include tests; run the relevant suite:
   - Go: `go test -race -count=1 ./...` and `go vet ./...`
   - Web: `cd web && npm test && npm run typecheck && npm run lint`
   - Plugins: `bash scripts/sync-plugins.sh` if marketplace files changed
3. **Docs** — Update README, AGENTS.md, or user-facing docs when behavior changes.
4. **Title** — Conventional Commit format: `feat:`, `fix(web):`, `docs:`, etc.
5. **Description** — Explain problem, solution, and link related issues.
6. **Secrets** — No credentials, tokens, or `.env` contents in the diff.
7. **Lockfiles** — Only changed via package manager, not hand-edited.
8. **Template** — Complete the GitHub PR template honestly.

## Superopen-specific

- If `.so/` harness files changed, note whether `so sync` was run.
- Prune obsolete rule/skill lines instead of only appending.
