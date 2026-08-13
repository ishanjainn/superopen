# PR check

Use when reviewing a pull request or preparing one for review.

1. Diff: run `gh pr diff` (or `git diff main...HEAD`).
2. Checklist:
   - Tests added for new behavior
   - No secrets or credentials in the diff
   - Scope stays on the requested task
   - Conventional Commit PR title
3. Mark each item pass/fail with a one-line reason.

## Superopen-specific

- Run `so doctor` if harness files changed
- Sync plugins with `bash scripts/sync-plugins.sh` when `plugins/` drift
