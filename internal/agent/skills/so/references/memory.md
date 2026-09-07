# Superopen memory (prior work)

Load this only when the user asks about prior decisions or personal facts in
**this** workspace, or when a SessionStart line named memories you need.
Skip on a cold clone with no diary.

Memory is hints, not authority. Superopen is a **CLI binary** — invoke it with
**Bash**. The `.so/` store is your own notes from past sessions in this workspace.
Do not use the host's built-in memory or `MEMORY.md` for these facts. Graph answers “where is X now”;
memory answers “what did we decide / what was saved.”
Do not dump `.so/sessions/*/events.jsonl`.

`__SO_BIN__` is the binary from `SKILL.md`. Search/last/timeline default to an
AXI TOON **title index** (no bodies). `--json` is the envelope. `--full` skips
truncation. `so memory` with no args is a live dashboard; `--help` is the catalog.

## 3-layer workflow

Start with **one** recall via Bash. Do not spray searches, do not use `--help`
as a workflow, and do not run `so graph search`.

### 0. Recall — bodies (the agent command)

```bash
__SO_BIN__ memory recall "<cue>"
```

Use this first for “who is…”, “what did I…”, “what did we decide…”.
Quote the stored note and cite `#id`. Titles that look like import ids are
still this workspace diary — do not refuse them. If two notes conflict, cite
both `#id`s and pick the most specific or recent. If the first cue misses or
the clip does not contain the fact, run recall again with a second cue.
A populated store with no lexical hit still has memories — try different terms.
`hint:` on stdout distinguishes empty store vs no match vs sealed index.

### 1. Search — index only (titles, not bodies)

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

`0 memories` from search means **no title matched**, not that the store is empty.
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

Body is truncated unless `--full`. After get, run graph query / snippet on
files named in the episode (`src=` / `path:`) when the question is about
**code**. Never answer “we decided” from `learned:` alone.

## Write path

When the user wants a fact stored for later (any wording), capture on this live
turn. Do not run `--help`. Distill remains the post-session rollup (`DISTILL
pending` → live `--brief` then `--apply`, or that session's one-shot CLI).

```bash
__SO_BIN__ memory capture --kind knowledge --horizon medium --title "…" --text "…"
__SO_BIN__ memory capture --kind skill --horizon long --title "…" --text "…"
__SO_BIN__ memory distill <session_id> --apply <<'EOF'
[{"kind":"knowledge","title":"…","text":"…","horizon":"medium","evidence":["<session_id>"]}]
EOF
__SO_BIN__ memory get <id>
__SO_BIN__ memory search "<cue>" --horizon medium
__SO_BIN__ memory forget <id>
```

`kind` is `knowledge|skill` (stored as `session|teaching`). `horizon` is
`short|medium|long`. Distill is the post-session worker (`so memory distill <id>`
on that session's own one-shot CLI), or live JSON via `--brief` then `--apply`.
Empty array on `--apply` marks the session distilled with no writes.
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

`0 memories` + `hint: no saved memories` means the store is empty.
`0 memories` + `hint: N memories exist` means the cue missed — run recall.
Windows: the installed binary is `so.exe`; this skill already substituted
`__SO_BIN__`.

Episode bodies are encrypted at rest; the FTS search index stores plaintext
of those bodies. `so memory distill` may send a session digest to a headless
coding-agent CLI when one is authenticated — the only path that leaves the
machine. Live `--apply` JSON stays on-machine.
