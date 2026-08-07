# Superopen schema (CLI ↔ UI parity)

<p align="center">
  <img src="../web/public/brand-mark.png" alt="SO shield" width="72" />
  <img src="../web/public/brand-wordmark.png" alt="SUPEROPEN" width="280" />
</p>


Guidance lives in native developer paths. Regenerable / machine-local runtime
lives under `.so/` and is gitignored. Both the Go CLI (`so`) and the TypeScript UI
(Next.js App Router) read/write these paths. Heavy algorithms use `so … --json`.

## Native guidance (tracked on the feature branch)

| Path | Role |
|------|------|
| `AGENTS.md` | Knowledge / agent instructions (root + nested `dir/AGENTS.md`). Wandering evals with a hot package propose creating that package’s `AGENTS.md`; otherwise they amend root. Rec `why` always states problem → change → next-session benefit. |
| Rules | **Index/UI:** all vendor trees. **Writes:** update-in-place across every existing stem copy (keep in sync); if none, create under the session vendor’s rules dir; else preferred fallback. |
| Skills | **Index/UI:** all vendor trees. **Writes:** same sync/create policy as rules. `/so` skill is reserved. |
| Retrieve | Session-vendor weighted: matching vendor rules/skills boosted; other vendors down-weighted; `AGENTS.md` always shared/high. |
| `CLAUDE.md` | Brief inject only (Superopen markers) |

## Runtime under `.so/` (mostly untracked)

**Write** lessons/prefs → **Store** under `.so/memory/` → **Inject** as `active-context.md` on every SessionStart.

Session port (`so sessions port`) moves chat text between agents; after a successful port Superopen refreshes the same ACTIVE pack so the destination agent gets prefs/lessons plus continuity. Port is not a second memory system.

| Path | Role | Tracked? |
|------|------|----------|
| `config.yaml` | Harness config | yes |
| `discovery.json` | Init profile snapshot | yes (rarely changes) |
| `AGENT.md` | Short agent brief | yes |
| `guardrails/guardrails.yaml` | Enforcement + advisory | yes |
| `evals/configs.yaml` | Eval check config | yes |
| `evals/history.json` | Eval run history | no |
| `sessions/` | Local working cache of materialized sessions | no (also mirrored to `refs/so/sessions/<id>`) |
| `traces/` | Local OTLP export | no |
| `memory/**` | Packs, lessons, harvest ledgers | no |
| `graph/**` | Structural graph (local rebuild) | no |
| `viz/**` | Citymap / HTML viz | no |
| `recommendations/pending.json` | Pending harness updates | no |
| `recommendations/history.json` | Applied / dismissed / reverted | no |
| `audit/events.jsonl` | Append-only SEL audit trail | no |
| `port/ledger.json` | Session port idempotency ledger | no |

## Version-control policy

Commit **native docs** (`AGENTS.md`, discovered rules/skills trees) and stable
`.so/` config (config, guardrails, evals configs, discovery, AGENT.md).

Do **not** commit regenerable Superopen runtime: graph, memory packs, sessions,
traces, eval history, or recommendation pending/history. Agents still use
`so graph query` against a local `.so/graph/graph.json` rebuilt by `so sync` /
hooks after HEAD moves.

The generated `.so/.gitignore` excludes that runtime so feature branches stay
clean after commit. Harvest that learns durable guidance writes native docs
(intentional `git status` dirt until the user commits).

Hooks (`post-commit`, `post-merge`, `post-checkout`) finalize sessions and rebuild
**untracked** runtime only; injectors are byte-idempotent. Session blobs are also
written to git side refs `refs/so/sessions/<id>` (no checkout). Live session state
lives under `.git/so-sessions/`. `pre-push` fast-forwards those refs (never force).
`SO-Session:` trailers on user commits link work to the active session.

Do not commit secrets or information your team is not authorized to share.

## AXI output (`so … --json` / `--full`)

Agent eXperience Interface: compact text by default, machine JSON on demand.

| Flag / env | Effect |
|---|---|
| `--json` / `SO_JSON=1` | JSON envelope on stdout (errors on stderr) |
| `--full` / `SO_FULL=1` | Do not truncate long fields |

```json
{ "ok": true, "kind": "sessions", "count": 1, "items": […], "next": ["so doctor"] }
```

Errors:

```json
{ "ok": false, "code": 1, "error": "…", "hint": "so sync" }
```

Exit codes: `0` ok · `1` fail · `2` usage · `3` not found.

Bare `so` prints a content-first status snapshot (not only help).

## Parity matrix (Phase 1+)

| Capability | CLI | UI |
|------------|-----|-----|
| Memory CRUD / search / active-context | `so memory *` (Write→Store→Inject) | `/memory` (Active Context → Lessons → Prefs → Projects) |
| Active Context inject | SessionStart when `memory.enabled` | Memory → Active Context |
| Learn lesson | `so learn add` | Memory → Lessons |
| Retrieve corpus | `so retrieve` / `so graph query` | Graph search + `/api/retrieve` |
| Knowledge | `so knowledge` / `AGENTS.md` | `/knowledge` |
| Rules | Discovered vendor rules dir | `/rules` |
| Skills | Discovered vendor skills `<name>/SKILL.md` | `/skills` |
| Guardrails | `so guard` / `so guard show\|check` | `/guardrails` |
| Audit | `so audit list` | Settings → Audit Trail |
| Session harvest | `so harvest run\|idle` | (automatic on session end) |
| Session port | `so sessions detect\|port\|verify` | Sessions → Port |
| Open UI | `so open memory` | - |

## Observability (local)

Edit `.so/config.yaml` only - the UI shows values as read-only:

```yaml
observability:
  listen: http://127.0.0.1:4318
  exporters:
    - type: local_jsonl
      path: .so/traces
```

Only `local_jsonl` is supported. Other exporter types are ignored by the CLI.

## Session porting

Hub-and-spoke session porting (`internal/port/`):

```bash
so sessions detect
so sessions port --from claude --to codex --preview
so sessions port --from claude --to so --id <session-id>
so sessions verify --from claude --sample 3
```

Harness IDs: `claude`, `codex`, `opencode`, `cursor`, `pi`, `so`. Text turns only in v1
(tools/thinking dropped). Ledger: `.so/port/ledger.json`. Cursor export writes a
first-class resume pack under `.cursor/so-port/<id>/` plus hub mirror; resume with
`so sessions resume --vendor=cursor --id=…`. Pi writes `~/.pi/agent/sessions/so-port/`.
