# Superopen session memory

Read this pack before exploring. Prefer `so graph query` / `.so/knowledge` for code structure.

## [Preferences]

# Preferences

How agents should work in this workspace.

- Prefer `so graph query` and `.so/knowledge` before broad code search.
- Run tests for packages you change (`go test ./…` or the package under edit).
- No drive-by refactors - keep diffs focused on the task.
- Never commit secrets, API keys, or `.env` files with real credentials.
- PR titles in this monorepo: `Plugin (<category>): <description>` (or include `[skip-changelog]`).
- Prefer parameterized SQL and typed errors; never log credentials.
- Ask before force-push, amend of pushed commits, or making a repo public.

## [Projects]

# Projects

## Current focus

- Updating README documentation in the `superopen` repo to match the "Graphify" project/branding.
- Deciding whether to add a UI screenshot vs. highlighting CLI strengths (e.g. recommendations) in the README.
- Clarifying `.so` directory gitignore policy in `superopen` repo (branch `ui-fixes`): whether the entire `.so` folder should be gitignored, or only ephemeral parts like traces, with the rest (e.g. knowledge, memory) tracked in git.

## Active areas

- (Packages / services under active change)

## Do not touch

- (Fragile areas, frozen APIs, or ongoing migrations)

## Notes

- Shared memory injects on every vendor SessionStart via `.so/memory/active-context.md`.
- Teach durable corrections with `so memory add` or Memory → Lessons.
- Port chats with `so sessions port` - that moves transcripts; this file stays workspace context.
- Multiple "Update README to match Graphify" sessions on 2026-08-07 in `superopen` (main) ended with poor eval and no visible transcript content - outcome unclear, likely incomplete or aborted.
- Open question raised repeatedly: attach a UI screenshot to the README, or instead emphasize CLI strengths like recommendations - still unresolved.
- A `.so` gitignore policy session ran 2026-08-07 on branch `ui-fixes` (~43min, 718k tokens, $0.44, eval "ok") but transcript content was truncated/not visible, so the actual decision (whole `.so` folder vs. only traces) is still not confirmed - re-check with the user or `git log`/`.gitignore` directly before assuming a resolution.

## [History]

#### 2026-08-07
### 2026-08-07T03:32:37Z
## Meta
{
  "id": "019fda2f-a371-7f00-9d70-03b028b2db0a",
  "vendor": "",
  "model": "gpt-5.6-terra",
  "title": "Update README to match Graphify",
  "status": "ended",
  "started_at": "2026-08-07T03:32:24.030308Z",
  "ended_at": "2026-08-07T03:32:32.754081Z",
  "duration_ms": 8723,
  "project_id": "2a249b6e996bf384",
  "repo_root": "/Users/ishanjain/work/superopen",
  "branch": "main"
}
## Transcript (truncated)

### 2026-08-07T03:32:39Z
## Meta
{
  "id": "019fda2f-a371-7f00-9d70-03b028b2db0a",
  "vendor": "",
  "model": "gpt-5.6-terra",
  "title": "Update README to match Graphify",
  "status": "ended",
  "started_at": "2026-08-07T03:32:24.030308Z",
  "ended_at": "2026-08-07T03:32:32.754081Z",
  "duration_ms": 8723,
  "eval_badge": "poor",
  "project_id": "2a249b6e996bf384",
  "repo_root": "/Users/ishanjain/work/superopen",
  "branch": "main"
}
## Transcript (truncated)

### 2026-08-07T03:38:14Z
## Meta
{
  "id": "019fda2f-a371-7f00-9d70-03b028b2db0a",
  "vendor": "",
  "model": "gpt-5.6-terra",
  "title": "Update README to match Graphify",
  "prompt_preview": "Should we attach a UI screenshot, or maybe CLI has some strenthts like recommendations or something",
  "status": "ended",
  "started_at": "2026-08-07T03:32:24.030308Z",
  "ended_at": "2026-08-07T03:38:05.655156Z",
  "duration_ms": 341624,
  "tokens": 61262,
  "cost_usd": 0.020695,
  "eval_badge": "poor",
  "project_id": "2a249b6e996bf384",
  "repo_root": "/Users/ishanjain/work/superopen",
  "branch": "main",
  "base_sha": "[REDACTED]",
  "head_sha": "[REDACTED]"
}
## Transcript (truncated)

### 2026-08-07T03:43:21Z
## Meta
{
  "id": "019fda2f-a371-7f00-9d70-03b028b2db0a",
  "vendor": "",
  "model": "gpt-5.6-terra",
  "title": "Update README to match Graphify",
  "prompt_preview": "Should we attach a UI screenshot, or maybe CLI has some strenthts like recommendations or something",
  "status": "ended",
  "started_at": "2026-08-07T03:32:24.030308Z",
  "ended_at": "2026-08-07T03:43:13.871847Z",
  "duration_ms": 649841,
  "tokens": 2585872,
  "cost_usd": 2.073072,
  "eval_badge": "poor",
  "project_id": "2a249b6e996bf384",
  "repo_root": "/Users/ishanjain/work/superopen",
  "branch": "main",
  "base_sha": "[REDACTED]",
  "head_sha": "[REDACTED]"
}
## Transcript (truncated)

### 2026-08-07T05:00:08Z
## Meta
{
  "id": "019fda2f-a371-7f00-9d70-03b028b2db0a",
  "vendor": "",
  "model": "gpt-5.6-terra",
  "title": "Update README to match Graphify",
  "prompt_preview": "Should we attach a UI screenshot, or maybe CLI has some strenthts like recommendations or someth
…[truncated]

## [Semantic Memory]

- repo: superopen repo at /Users/ishanjain/work/superopen, branch main
- task: Update README to match Graphify naming/branding
- session_2026-08-07_readme_graphify: Session titled 'Update README to match Graphify' in superopen repo (main branch) started and ended within ~9s with poor eval badge and no transcript content — task likely not completed.
- repo_superopen: Repo at /Users/ishanjain/work/superopen, default branch main.
- project_graphify: 'Graphify' appears to be a project/branding name referenced for README updates in superopen.
- superopen_readme_question: Open question raised: whether to attach a UI screenshot or emphasize CLI strengths (e.g. recommendations) in the README
- superopen_readme_session_status: Multiple sessions on this task ended quickly with poor eval and no visible transcript — task likely still incomplete
- superopen_repo_branch: Work on superopen repo README task occurs on the main branch
- superopen_readme_open_question: Open question raised across multiple sessions: whether README should include a UI screenshot or emphasize CLI strengths like recommendations.
- superopen-readme-rebrand: README in `superopen` repo is being updated to match the "Graphify" project/branding.
- superopen-readme-open-question: Undecided whether README should include a UI screenshot or instead highlight CLI strengths (e.g. recommendations feature).
- superopen-readme-sessions-status: Multiple README/Graphify sessions on 2026-08-07 ended quickly with poor eval and no visible transcript - likely incomplete/aborted, so no confirmed decisions yet.
- session_2026-08-07_ui-fixes: Session titled 'Clarify .so gitignore policy' on branch ui-fixes in superopen repo, 2026-08-07, ~29s duration, transcript truncated/no visible content
- repo_superopen_branches: superopen repo has at least two active branches in use: main and ui-fixes
- gitignore_so_policy: Open question in superopen repo: whether/how .so directory should be handled in gitignore
- user_email: ishan.jai
…[truncated]

## [Learned corrections]

- Add skill for recurring exploration pattern - Harness underused - agent skipped .so skills/context.

## [Skills]

- create-api.md
- debugging.md
- reduce-exploration.md
- testing.md

