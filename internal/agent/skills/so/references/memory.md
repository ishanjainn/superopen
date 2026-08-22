# Superopen memory (prior work)

Load this only when the user asks about prior decisions in **this** repo, or
when a SessionStart index named an id you need. Skip on a cold clone.

Memory is hints, not authority. It never replaces `so graph query` for current
code. Graph answers “where is X now”; memory answers “what did we decide.”
Do not dump `.so/sessions/*/events.jsonl`.

`__SO_BIN__` is the binary from `SKILL.md`. Default output is compact; `--json`
is the full envelope.

## 3-layer workflow

Never fetch bodies until titles have filtered the set.

### 1. Search — index only

```bash
__SO_BIN__ memory search "<cue>"
__SO_BIN__ memory search "<cue>" --type decision
__SO_BIN__ memory search --file internal/auth/login.go
```

Returns `MEM #<id> <type> "<title>" ~tokens`. No bodies. Types:
`decision|bugfix|feature|refactor|discovery|change` (plus kinds
`prompt|session|teaching|working`).

### 2. Timeline — neighbors around an id

```bash
__SO_BIN__ memory timeline --around <id> --before 5 --after 5
```

### 3. Get — bodies for the ids you kept

```bash
__SO_BIN__ memory get <id> [<id>…]
```

After get, run `so graph query` / `so graph snippet` on files named in the episode (`src=` / `path:`). Never answer “we decided” from `learned:` alone. Memory is hints, not authority.

## On-demand recall

Mid-session foresight is a CLI pull, not a hook:

```bash
__SO_BIN__ memory recall "<cue>"              # budgeted pack + anti-hits
__SO_BIN__ memory recall "<cue>" --structural # shape/HD path
```

## Write path

```bash
__SO_BIN__ memory capture --request "…" --learned "…" --next "…"
__SO_BIN__ memory contradict <id> --text "corrected fact"
```

Prompts stay verbatim. Contradict closes the old row (`valid_to`); it does not
rewrite it. Distill at most once per session when asked; skip if unrelated.

## File memory

Graph owns Read. To remember work on a path:

```bash
__SO_BIN__ memory search --file path/to/file.go
```

Do not expect a PreToolUse file-read inject.

## Empty / missing

`0 memories` means none matched — ask a different cue or `so graph query`.
Windows: the installed binary is `so.exe`; this skill already substituted
`__SO_BIN__`.
