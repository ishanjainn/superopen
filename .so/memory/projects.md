# Projects

## Current focus

- Updating README documentation in the `superopen` repo to match the "Graphify" project/branding.
- Deciding whether to add a UI screenshot vs. highlighting CLI strengths (e.g. recommendations) in the README.
- Clarifying `.so` directory gitignore policy in `superopen` repo (branch `ui-fixes`): whether the entire `.so` folder should be gitignored, or only ephemeral parts like traces, with the rest (e.g. knowledge, memory) tracked in git.

## Active areas

- Branch `ui-fixes` in `superopen` - had a commit "fixsync" on 2026-08-07.

## Do not touch

- (Fragile areas, frozen APIs, or ongoing migrations)

## Notes

- Shared memory injects on every vendor SessionStart via `.so/memory/active-context.md`.
- Teach durable corrections with `so memory add` or Memory → Lessons.
- Port chats with `so sessions port` - that moves transcripts; this file stays workspace context.
- Multiple "Update README to match Graphify" sessions on 2026-08-07 in `superopen` (main) ended with poor eval and no visible transcript content - outcome unclear, likely incomplete or aborted.
- Open question raised repeatedly: attach a UI screenshot to the README, or instead emphasize CLI strengths like recommendations - still unresolved.
- A `.so` gitignore policy session ran 2026-08-07 on branch `ui-fixes` (~43min, 718k tokens, $0.44, eval "ok") but transcript content was truncated/not visible, so the actual decision (whole `.so` folder vs. only traces) is still not confirmed - re-check with the user or `git log`/`.gitignore` directly before assuming a resolution.
- A short session on 2026-08-07 (branch `ui-fixes`, prompt "yoyo") produced a commit "fixsync" (c950a0683b0e8b9e34e920732f696c21943e12dd) but transcript content was not visible - purpose of this commit is unconfirmed, check `git show c950a068` for details.
- Lesson logged 2026-08-07 (lesson_03038f3d49a2bb3e): agent skipped `.so` skills/context during a recurring exploration pattern - consider adding a dedicated skill for this pattern so it's not missed again.
