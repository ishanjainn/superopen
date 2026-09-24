# Scan

`so scan` checks recorded tool calls in `.so/sessions/` against detection rules.
It only reads. It does not block the agent, and it does not change session files.

## Run

```bash
so scan                         # every session in this repo
so scan --session <id>          # one session directory name
so scan --min-severity high     # hide findings below this rank
so scan --fail-on high          # exit with an error if any finding is this rank or higher
so scan --json                  # shared {ok, kind, data} envelope
so scan --rules <dir>           # use this directory instead of stored rules
```

Text output is one tab-separated line per finding: severity, rule id, session id, reason.
No matches prints `no findings`.

Ranks, low to high: `info`, `low`, `medium`, `high`, `critical`.

`--json` is the shared envelope. `data` is the findings array (`rule_id`, `title`, `severity`, `posture`, `session_id`, `reason`, `events`). The Scan page and each session page load this automatically.

## What it can see

Only these recorded tools become events:

| Tool | Action |
|------|--------|
| Bash, Shell | `command.executed` |
| Read | `file.read` |
| Edit, Write, MultiEdit | `file.edited` |

Other tools are skipped. Prompt text is already redacted in the session log, so rules see tool names and arguments that were stored, not the raw prompt.

## Rules

If you pass `--rules`, that directory is the whole set.

Otherwise scan looks in `.so/guards` for `*.rule.yaml`. If that directory has any rule files, those files replace the built-in set. They are not merged with it. `.so/guards` is not listed in `.so/.gitignore`, so those files can be committed.

If `.so/guards` is missing or empty, scan uses a small baseline compiled into `so`:

- curl or wget piped into a shell
- recursive delete from `/`
- credentials passed in curl data
- "ignore previous instructions" text
- a request to exfiltrate the system prompt
- a secret-file read followed by a network command in the same session, within 120 seconds

This phase reports findings only. `posture: enforce-capable` is allowed in the file, but nothing is blocked.

## Writing a rule

Copy a file under `internal/guards/rules/baseline/` and change it. A rule needs:

- `id` (lowercase, digits, hyphens)
- `version` (1 or more)
- `title`, `severity`, `status` (`experimental`, `stable`, or `deprecated`)
- `posture` (`detect` or `enforce-capable`)
- `emit.reason`
- exactly one of `match` (a CEL expression on `e`) or `correlation` (at least two ordered steps, `scope: session`, and a Go duration `window` such as `120s`)
- at least one `tests` entry with `verdict: match` or `verdict: no_match`

CEL fields are listed in [`internal/guards/FIELDS.md`](../internal/guards/FIELDS.md). Common ones: `e.event.action`, `e.command.command`, `e.file.path`. A missing object reads as empty, so a rule does not crash when a field is absent.

Load fails if the YAML is invalid or a fixture does not match its verdict.
