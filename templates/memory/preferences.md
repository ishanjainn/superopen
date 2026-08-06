# Preferences

How agents should work in this workspace.

- Prefer `so graph query` and `.so/knowledge` before broad code search.
- Run tests for packages you change (`go test ./…` or the package under edit).
- No drive-by refactors - keep diffs focused on the task.
- Never commit secrets, API keys, or `.env` files with real credentials.
- PR titles in this monorepo: `Plugin (<category>): <description>` (or include `[skip-changelog]`).
- Prefer parameterized SQL and typed errors; never log credentials.
- Ask before force-push, amend of pushed commits, or making a repo public.
