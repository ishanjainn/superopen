# Harvest

After a coding session, harvest proposes small patches to instruction files —
`AGENTS.md`, vendor rules, skills. **Humans approve before anything becomes
always-on.** A bad always-on rule is paid on every future session, so harvest
stays off the hot path: no hook injection while you work. Live agent first:
SessionStart and the first prompt-submit inject `HARVEST pending` (`brief`,
then `propose`, or `skip <id>`). SessionEnd uses the live vendor's own
one-shot CLI when it is authenticated; otherwise work stays pending for the
next SessionStart. There is no cross-vendor fallback. Headless workers are
not recorded as sessions.

## Flow

```bash
so harvest brief [session]   # prompt for the live agent (pending session if omitted)
so harvest propose           # ingest JSON from stdin / --file
so harvest skip <session>    # close pending with nothing to propose
so harvest scan [session]    # SessionEnd: own one-shot CLI only
so harvest list              # open proposals
so harvest list --history    # closed proposals + skipped runs (session retention window)
so harvest show <id>         # reason, evidence, unified diff
so harvest review            # compact OPEN pack (load only when asked)
so harvest apply <id>        # gated write to the live playbook
so harvest decline <id>
so harvest inventory         # discovered playbook files (hash, protected)
```

Proposals enter either from the live agent's wrap-up or from `scan`
(`propose` ingests JSON on stdin/`--file`). Every proposal requires a
`reason`; `evidence` is required when the session id is known. A propose or
skip with a session id clears that session's pending row.

## Guardrails

- **Cap 3 proposals** per run; skip when already harvested, empty, duplicate,
  or the live file already contains the change.
- Playbook files under protection (sentinel + `/so` tripwire + Superopen
  hooks) are never touched silently.
- Nothing writes live playbooks from finalize or headless runs — only
  `apply` writes, and it re-validates before doing so.
- Graph refresh never waits on harvest.
- At most one optional **pending** line on SessionStart **and** the first
  prompt-submit (`HARVEST pending … brief` then `propose`, or `skip <id>`).
  OPEN review is `so harvest review` on demand, never injected.
- Headless workers set `SUPEROPEN_HEADLESS` and are not recorded as sessions.
  Generation sees inventory hashes plus a compact session digest — not full
  transcripts or playbook bodies. `scan` never launches another vendor's CLI.

## For contributors

Layout and product rules:
[`internal/harvest/AGENTS.md`](../internal/harvest/AGENTS.md); CLI in
[`cmd/so/harvest_cmd.go`](../cmd/so/harvest_cmd.go).
