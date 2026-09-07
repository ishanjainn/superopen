# Benchmarks

Run from the repository root. Every run overwrites repo-root
[BENCHMARKS.md](../BENCHMARKS.md). Work directories are deleted after the
report is written. Product modes default to `--isolate docker`. Manual GitHub
Action: `.github/workflows/bench.yml` (`workflow_dispatch`).

## Critical notice

Superopen is a CLI for any coding agent on Linux, macOS, and Windows. Benchmarks
must look like that product:

- Real **Claude Code** or **OpenCode** chats in isolated Docker HOMEs (`so init` + `so install`).
- Bind-mount the locally built Superopen CLI (`--so-bin`); do not bake `so` into the image.
- **No benchmark-only hacks** in `so` (no gold-id rankers, dataset packing, API pack-readers).
- Superopen **modes** (`--mode`, `--scale`). Do not port another product's harness into `so`.
- `--scale small` is a valid gate (LOCOMO 100 stratified / LME 50 / compare 6 / graph 12 / SWE-bench 10). `--scale full` is what you publish (LOCOMO retrieve n=300; QA still `--qa-n 20`; SWE-bench n=50).

## Two ways to run

Same harness, same locally built `so`, same `BENCHMARKS.md`. Pick one:

**1. Python on this machine.** Claude Code runs in a per-arm container (`--isolate docker`). Developer HOME is never mounted.

```bash
python3 benchmarks/run.py --mode compare --scale small --host claude-code --isolate docker --so-bin ./bin/so
```

**2. Docker for the whole harness.** The `so-bench` image runs `benchmarks/run.py`. Isolation is the container (`--isolate host` inside). Auth files are copied in; host HOME is not a volume.

```bash
./benchmarks/docker-run.sh --mode compare --scale small --host claude-code --max-spend 20
```

Default mode is `offline`: **harness smoke only**, not a Superopen score. Default scale is **small**. Use `--scale full` only after small clears the bar.

## Harness smoke vs product benchmarks

| Kind | Modes | What runs |
|------|-------|-----------|
| **Harness smoke** | `offline` | Python unit tests for grading math, spend ledger, question JSON layout. No `so`, no OpenCode, no Claude, no Django. CI gate (`make bench-offline`). |
| **Product benchmarks** | everything else | Real `so` binary bind-mounted into Docker (default `--isolate docker`), real CLI paths users use (`so init`, `so graph *`, `so memory capture` / `search`, `so install`). Session compare uses real `opencode run` or `claude -p` in container HOMEs (auth copied only; developer HOME is never mounted). `--mode offline` / `contradict` / `latency` stay on the host. |

### What uses real Superopen / agents

| Mode | `so init` | `so install` (plugin) | Agent host | Notes |
|------|-----------|------------------------|------------|-------|
| `index` | yes (Django worktree) | no (graph CLI only) | - | Wall time + graph counts |
| `graph` | yes (Django worktree) | no | - | 12 probes via `so graph *` |
| `temporal` | yes (5 Django LTS tags) | no | - | Index growth table |
| `memory` | yes (per-run memory store) | phase 3: yes | phase 3: Claude Code or OpenCode | Ingest via `so memory capture`; rank via `so --json memory recall`. Phase 3 is a **coding-agent session** (user question only). Extractive substring is debug. |
| `latency` | yes (tiny fixture) | no | - | `so memory search` timing on your hardware |
| `contradict` | - | - | - | Go tests in `internal/memory/` (contradiction ranking semantics) |
| `compare` **native** arm | **no** (stock agent) | **no** | OpenCode or Claude Code | Baseline: grep/read without Superopen |
| `compare` **superopen** arm | yes (per-question Django worktree) | yes (`--vendor=opencode` or `claude-code`) | same host + model | One `so init` seed, then copy `.so/` into a fresh worktree/HOME per question |

Compare runs **identical prompts** on both arms. Only the superopen arm gets `so init` + `so install`; that is the native-vs-Superopen contrast, not a bug. Harness prompts never mention Superopen. `--compare-arms` / `--compare-baseline` resume one arm like SWE.

## Modes

| Mode | What it measures | API / LLM | Django clone | Est. duration |
|------|------------------|-----------|--------------|---------------|
| `offline` | Harness integrity (not Superopen) | no | no | seconds |
| `contradict` | Rescue@10 + historical-verbatim (Go memory tests) | no | no | seconds |
| `latency` | `so memory search` p50/p95 on local store | no | no | seconds |
| `index` | `so init` wall time, node/edge/file counts on Django | no | yes | minutes |
| `graph` | 12 graph-tool probes via `so graph *` on Django | no | yes | minutes after index |
| `memory` | LOCOMO / LongMemEval recall@10 (+ optional phase 3 agent QA) | phase 2: no; phase 3: coding agent | no | phase 2: minutes; phase 3: spend-capped |
| `compare` | Native vs Superopen: coverage, tokens, **token savings %** | host agent | yes | tens of minutes |
| `swe` | Native vs Superopen on SWE-bench Verified (official grader) | host agent | per-issue checkout | tens of minutes |
| `temporal` | Index size across 5 Django LTS tags | no | yes | longer (optional) |
| `all` | Full suite including harness smoke and SWE-bench | mixed | yes | longest |

`--max-spend 0` forbids LLM calls (`compare`, `swe`, and `memory --phase 3` skip). `--mode all` always runs memory phase 3 and `--swe-grade` so `BENCHMARKS.md` has no `pending` cells (needs `--max-spend > 0` and `ANTHROPIC_API_KEY`). Pass `--no-swe-grade` only if you want patches without official verdicts.

### Reproduce commands

| `--mode` | Command |
|----------|---------|
| `offline` | `python3 benchmarks/run.py --mode offline` |
| `contradict` | `python3 benchmarks/run.py --mode contradict` |
| `latency` | `python3 benchmarks/run.py --mode latency` |
| `index` | `python3 benchmarks/run.py --mode index --repo django --index-timeout 1800` |
| `graph` | `python3 benchmarks/run.py --mode graph --repo django --index-timeout 1800` |
| `memory` (retrieve, small) | `python3 benchmarks/run.py --mode memory --phase 2 --split locomo --scale small --adapters superopen,bm25,bow,rrf --max-spend 0` |
| `memory` (QA) | `python3 benchmarks/run.py --mode memory --phase 3 --split locomo --scale small --host claude-code --model claude-sonnet-5 --max-spend 15` |
| `memory` (LME QA) | `python3 benchmarks/run.py --mode memory --phase 3 --split longmemeval --scale small --host claude-code --model claude-sonnet-5 --max-spend 15` |
| `compare` | `python3 benchmarks/run.py --mode compare --scale small --host claude-code --model claude-sonnet-5 --max-spend 20` |
| `swe` | `python3 benchmarks/run.py --mode swe --scale small --host claude-code --model claude-sonnet-5 --max-spend 50` |
| `temporal` | `python3 benchmarks/run.py --mode temporal --repo django` |
| `all` | `python3 benchmarks/run.py --mode all --repo django --phase 2 --max-spend 70` |

OpenCode: `--host opencode --model opencode/big-pickle`. Never `--host anthropic-api`.

Build `so` first if needed: `make build-native` or pass `--so-bin ./bin/so`. Product modes bind-mount that CLI into Docker. If Docker is down: `--isolate host`.

### Generated `BENCHMARKS.md`

```bash
python3 benchmarks/run.py --mode graph --so-bin ./bin/so
```

The run prints `benchmarks.md: ./BENCHMARKS.md`. System, combined duration, and
cost (native vs Superopen vs extras) sit at the top. **Results** is one section
per suite, SWE-bench first. How-to-run lives only in this README.
Missing suites are filled from `benchmarks/.last-summary.json` (gitignored) and
optional `--fill-from` result dirs so a memory-only run does not wipe SWE rows.
Pass `--no-fill` for a clean report. `--mode all` includes SWE and skips a
duplicate Django `index` because `graph` already records `so init`.

### GitHub Action

`.github/workflows/bench.yml` is **manual only** (`workflow_dispatch`). It builds
Linux `so`, runs the same `benchmarks/run.py` with `--isolate docker`, and
uploads `BENCHMARKS.md` as an artifact. It does not commit.

Secrets: **`ANTHROPIC_API_KEY` required** for `compare`, `swe`, and memory `--phase 3`
(Claude Code billing + QA judge), plus `--max-spend > 0`. Optional `CLAUDE_CREDENTIALS_JSON`,
`SUPEROPEN_LOCOMO_URL` / `SUPEROPEN_LME_URL` (https) for datasets that are not in
the Actions cache.

## Offline tests (`--mode offline`)

Unit tests in `tests/test_harness.py`. **These do not exercise Superopen.** They verify the harness grader, baselines, and fixtures.

| Test | What it checks | Est. duration |
|------|----------------|---------------|
| `test_grade_partial_credit` | Key-fact coverage formula | under 1 s |
| `test_graph_probe_grades` | PASS / PARTIAL / FAIL scoring | under 1 s |
| `test_bm25_search` | Internal BM25 + bow + RRF (Python baselines, not `so`) | under 1 s |
| `test_spend_ledger` | `--max-spend` ledger | under 1 s |
| `test_django_questions_json` | Compare question bank has 6 prompts | under 1 s |
| `test_dataset_readme_layout` | Dataset README layout | under 1 s |

CI entry: `make bench-offline`.

## Datasets

Academic datasets are not redistributed. See [datasets/README.md](datasets/README.md).

| Dataset | n | Used by |
|---------|---|---------|
| LOCOMO (`locomo10.json`) | 300 | `memory --split locomo` |
| LongMemEval-S (English subset) | 50 | `memory --split longmemeval` |
| Django (pinned LTS tag) | - | `index`, `graph`, `compare`, `temporal` |
| SWE-bench Verified | 5 small / 50 full | `swe` |
| `fixtures/tiny/` | - | `latency` smoke |

## Artifacts

Gitignored: `cache/`, `datasets/` (except README), `work/`, `.last-summary.json`. The published
artifact is repo-root `BENCHMARKS.md` only.
