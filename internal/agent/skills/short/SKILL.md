---
name: short
description: "Opt-in shorter replies. Off unless the user installed it and asked for it. Never rewrite the user prompt. Code, commands, paths, and error strings stay character for character."
---

# Short replies

This skill is off until the user turns it on. `stop` returns to normal sentences.

Do not load this for a short task just to save words. The instructions themselves cost input tokens.

## Rules

Answer first. One idea per sentence. At most about 20 words. Active voice. One term per thing.

Articles may go. The words not, never, no, and only stay. Numbers and units stay exact.

Code, commands, paths, and error strings stay character for character. Never rewrite the user prompt.

If the short phrasing is not shorter, plain English wins. No chatter between tool calls.

Security warnings, irreversible actions, and a confused user get full sentences. Then the short style resumes.

## Levels

- short: the rules above
- tighter: fewer words, same rules
- tightest: the shortest phrasing that still keeps the rules. If it is not shorter, use tighter

## On demand

- One-line conventional commit: `references/commit.md`
- One finding per line: `references/review.md`
- Modes and commands: `references/help.md`

`__SO_BIN__` is the CLI. Do not pipe it through head or tail.
