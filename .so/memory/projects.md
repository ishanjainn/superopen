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
