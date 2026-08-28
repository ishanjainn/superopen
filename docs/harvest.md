# Harvest

After a coding session, harvest proposes small patches to instruction files —
`AGENTS.md`, vendor rules, skills. **Humans approve before anything becomes
always-on.** A bad always-on rule is paid on every future session, so harvest
stays off the hot path: no hook injection while you work, at most one bounded
headless call after finalize.

## Flow

```bash
so harvest scan        # skip gates, then at most one bounded headless generate
so harvest list        # open proposals
so harvest show <id>   # reason, evidence, unified diff
so harvest review      # compact OPEN pack (load only when asked)
so harvest apply <id>  # gated write to the live playbook
so harvest decline <id>
so harvest inventory   # discovered playbook files (hash, protected)
```

Proposals enter either from the live agent's wrap-up or from `scan`
(`propose` ingests JSON on stdin/`--file`). Every proposal requires a
`reason`; `evidence` is required when the session id is known.

## Guardrails

- **Cap 3 proposals** per run; skip when already harvested, empty, duplicate,
  or the live file already contains the change.
- Playbook files under protection (sentinel + `/so` tripwire + Superopen
  hooks) are never touched silently.
- Nothing writes live playbooks from finalize or headless runs — only
  `apply` writes, and it re-validates before doing so.
- Graph refresh never waits on harvest.
- At most one optional status line (`HARVEST N OPEN`) on SessionStart; no
  methodology text anywhere else.
- Headless generation sees inventory hashes plus a compact session digest —
  not full transcripts or playbook bodies.

## For contributors

Layout and product rules:
[`internal/harvest/AGENTS.md`](../internal/harvest/AGENTS.md); CLI in
[`cmd/so/harvest_cmd.go`](../cmd/so/harvest_cmd.go).
