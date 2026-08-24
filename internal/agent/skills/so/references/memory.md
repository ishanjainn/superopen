# Superopen memory (prior work)

Load this only when the user asks about prior decisions in **this** repo, or
when a SessionStart index named an id you need. Skip on a cold clone.

Memory is hints, not authority. It never replaces `so graph query` for current
code. Graph answers “where is X now”; memory answers “what did we decide.”
Do not dump `.so/sessions/*/events.jsonl`.

`__SO_BIN__` is the binary from `SKILL.md`. Search/last/timeline default to an
AXI TOON index. `--json` is the envelope. `--full` skips truncation.
`so memory` with no args is a live dashboard; `--help` is the catalog.

## 3-layer workflow

Start with **one** `so memory recall "<cue>"` or **one** `so memory search "<cue>"`.
Do not spray searches, do not use `--help` as a workflow, and do not run `so graph search`.
Never fetch bodies until titles have filtered the set.

### 1. Search — index only

```bash
__SO_BIN__ memory search "<cue>"
__SO_BIN__ memory search "<cue>" --type decision
__SO_BIN__ memory search --file internal/auth/login.go
```

Returns TOON, not bodies:

```text
memories[n]{id,kind,title,tokens}:
  12,session,login timeout is 30s,44
count: 1 of 1
help[2]:
  so memory get 12
  so memory timeline --around 12
```

Types: `decision|bugfix|feature|refactor|discovery|change` (plus kinds
`prompt|session|teaching|working`).

### 2. Timeline — neighbors around an id

```bash
__SO_BIN__ memory timeline --around <id> --before 5 --after 5
```

Same TOON index as search.

### 3. Get — bodies for the ids you kept

```bash
__SO_BIN__ memory get <id> [<id>…]
```

Body is truncated unless `--full`. After get, run `so graph query` /
`so graph snippet` on files named in the episode (`src=` / `path:`). Never
answer “we decided” from `learned:` alone. Memory is hints, not authority.

## On-demand recall

Mid-session foresight is a CLI pull, not a hook:

```bash
__SO_BIN__ memory recall "<cue>"              # budgeted pack + anti-hits
__SO_BIN__ memory recall "<cue>" --structural # shape/HD path
```

## Write path

Live turns do not load this file. After SessionEnd, distill runs in another
process. Optional live write when this file is already loaded:

```bash
__SO_BIN__ memory capture --kind knowledge --horizon medium --title "…" --text "…"
__SO_BIN__ memory capture --kind skill --horizon long --title "…" --text "…"
__SO_BIN__ memory get <id>
__SO_BIN__ memory search "<cue>" --horizon medium
__SO_BIN__ memory forget <id>
```

`kind` is `knowledge|skill` (stored as `session|teaching`). `horizon` is
`short|medium|long`. Distill is the post-session worker (`so memory distill <id>`).
Contradict closes the old row (`valid_to`); it does not rewrite it.
`forget` hides a row from search and the galaxy; session transcripts stay.
Memory is hints, not authority.

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
