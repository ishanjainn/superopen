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
No matches prints `count: 0` and `findings: none`.

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

The rules a scan runs are the built-in set plus your own. Custom rules are `*.rule.yaml` files in `.so/guards` (not listed in `.so/.gitignore`, so they can be committed). A custom rule with a built-in's `id` replaces that built-in; remove it to get the built-in back. A rule with `disabled: true` stays on file but is skipped. If you pass `--rules <dir>`, only the rules in that directory run.

The built-in rules:

- curl or wget piped into a shell
- recursive delete from `/`
- credentials passed in curl data
- "ignore previous instructions" text
- a request to exfiltrate the system prompt
- a secret-file read followed by a network command in the same session, within 120 seconds

This phase reports findings only. `posture: enforce-capable` is allowed in the file, but nothing is blocked.

### Manage rules

```bash
so scan rules                    # list every rule: id, severity, source, enabled
so scan rules show <id>          # the rule's YAML
so scan rules set rule.yaml      # create, or replace the rule with the same id (use - for stdin)
so scan rules disable <id>       # skip it in scans; works on built-ins
so scan rules enable <id>
so scan rules remove <id>        # delete a custom rule; removing an edited built-in restores it
```

`set` checks the rule before writing anything: the fields, the expression, and that every test fixture gives its stated verdict. An invalid rule is rejected and no file changes. A built-in cannot be deleted, only disabled.

The Scan page has a Rules tab that does the same: list, edit the YAML, save, disable, delete or revert, and add a new rule. It reads and writes the same files, so a change in either place applies to both and to the next scan.

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
