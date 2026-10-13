# Superopen Benchmarks

Last updated: 2026-10-13. How to run: [benchmarks/README.md](benchmarks/README.md).

## System

| | |
|---|---|
| Machine | MacBookPro · Apple M1 Pro · 8-core arm64 · 16 GB |
| OS | macOS 26.6.2 (darwin) |
| Agent | Claude Code 2.1.241 · `claude-sonnet-5` (claude-code) |
| Scale | small (LOCOMO n=100, LME n=50, compare 6, graph 12, SWE-bench 5/50 issues) |

## Run

| Metric | Combined | Native | Superopen | Extras |
|---|---|---|---|---|
| Duration | 2h 10m (memory 48m 15s; contradict 2.8s; latency 0.6s; graph 1m 35s; compare 22m 17s; temporal 6m 34s; swe 51m 36s) | 29m 50s | 29m 8s | 1h 11m |
| Cost | $11.62 | $5.30 | $6.30 | $0.03 (SWE eval $0.00; LOCOMO judge $0.01; LME judge $0.02) |

## Results at a glance

| Suite | Dataset (n) | Metric | Superopen | Compared with |
|---|---|---|---|---|
| SWE-bench | Verified (n=5) | Correctness | 4 / 5 (80%) | native 4 / 5 (80%) |
| SWE-bench | Verified (n=5) | Cost | $2.70 | native $4.31 |
| Memory | LOCOMO (n=100) | recall@10 | 92.9% (91/98) | BM25 90.8% (89/98), RRF 86.7% (85/98), bow 40.8% (40/98) |
| Memory | LOCOMO (n=100) | QA accuracy | 50.0% (key-fact coverage); judge 88%; 8/16 scored; qa_n=20; 4 empty-gold skipped; claude-code | — |
| Memory | LongMemEval-S (50) | recall@10 | 82.0% (41/50) | BM25 76.0% (38/50), RRF 72.0% (36/50), bow 16.0% (8/50) |
| Memory | LongMemEval-S (50) | QA accuracy | 58.0% (key-fact coverage); judge 66%; 29/50 scored; qa_n=50; claude-code | — |
| Graph | Django 5.2.4 | 12-probe | 12/12 | — |
| Sessions | Django (6) | Key-fact coverage | 1.00 | native 1.00 |
| Sessions | Django (6) | Cost | $0.66 | native $0.99 |

## Results

### SWE-bench (n=5)

| Correctness & efficiency | Cold Claude Code | Claude Code with Superopen | Improvement |
|---|---|---|---|
| Correctness | 4 / 5 (80%) | **4 / 5 (80%)** | **+0 pts** |
| Tokens | 61.4M | **36.9M** | **+40%** |
| Cost | $4.31 | **$2.70** | **+37%** |
| Tool calls | 249 | **161** | **+35%** |
| API requests | 254 | **166** | **+35%** |
| Wall-clock | 1,429s | **933s** | **+35%** |

<details>
<summary>Correctness over all instances</summary>

Correctness is scored on every instance. Tokens, cost, tool calls, API requests and wall-clock are over every completed session. Both arms passed 3 instances (cost $2.39 vs $1.25; not the headline).

| SWE-bench instance | Cold Claude Code | Claude Code with Superopen | Token usage vs. cold | Tool usage vs. cold |
|---|---|---|---:|---:|
| `django-11532` | Passed | Failed | 47% | 56% |
| `django-16263` | Passed | **Passed** | 64% | 58% |
| `sphinx-9461` | Failed | **Passed** | 100% | 98% |
| `django-11400` | Passed | **Passed** | 21% | 33% |
| `pylint-6386` | Passed | **Passed** | 91% | 97% |

</details>

### Memory

#### LOCOMO (n=100 asked, 98 scored)

| System | recall@5 | recall@10 | Ingest cost |
|---|---|---|---|
| **Superopen** (`so memory recall`) | **90.8% (89/98)** | **92.9% (91/98)** | $0.00; so-bge-small-en-v1.5-384; pending=0; collapsed=0 skipped=0; worker=bge |
| BM25 | 83.7% (82/98) | 90.8% (89/98) | $0 (shared index) |
| RRF | 72.4% (71/98) | 86.7% (85/98) | $0 (shared index) |
| bow (bag-of-words) | 23.5% (23/98) | 40.8% (40/98) | $0 (shared index) |

QA accuracy: 50.0% (key-fact coverage); judge 88%; 8/16 scored; qa_n=20; 4 empty-gold skipped; claude-code.

Gold-in-store: hard_miss=0, rank_miss=7 of scored=98 (stored_docs=272). hard_miss = gold never captured; rank_miss = captured but not in top-10. BM25/bow/RRF search raw docs, Superopen searches the post-capture store.

#### LongMemEval-S (50)

| System | recall@5 | recall@10 | Ingest cost |
|---|---|---|---|
| **Superopen** (`so memory recall`) | **68.0% (34/50)** | **82.0% (41/50)** | $0.00; so-bge-small-en-v1.5-384; pending=0; collapsed=0 skipped=3; worker=bge |
| BM25 | 70.0% (35/50) | 76.0% (38/50) | $0 (shared index) |
| RRF | 58.0% (29/50) | 72.0% (36/50) | $0 (shared index) |
| bow (bag-of-words) | 6.0% (3/50) | 16.0% (8/50) | $0 (shared index) |

QA accuracy: 58.0% (key-fact coverage); judge 66%; 29/50 scored; qa_n=50; claude-code.

Gold-in-store: hard_miss=0, rank_miss=9 of scored=50 (stored_docs=2375). hard_miss = gold never captured; rank_miss = captured but not in top-10. BM25/bow/RRF search raw docs, Superopen searches the post-capture store.

#### Contradiction

| Metric | Superopen |
|---|---|
| Rescue@10 | 1.000 |
| historical-verbatim | 1.000 |

### Graph (Django 5.2.4)

| Metric | Score |
|---|---|
| Index time | 1m 18s |
| Index size | 52,355 nodes, 346,672 edges, 4,652 files |
| 12-probe score | 12/12 |
| Probe latency | p50=567 ms, p95=1125 ms |

### Sessions (Django, 6)

| Metric | Score |
|---|---|
| Duration | 22m 17s |
| Key-fact coverage | 1.00 (native 1.00) |
| USD | $0.66 vs $0.99 native; 34% cheaper; cheaper on 6/7 questions |
| cache_read tokens | 3,562,977 vs 8,485,501 native (58% less cache_read) |

### Temporal (Django LTS)

| Checkpoint | Nodes | Edges | Files | `so init` |
|---|---:|---:|---:|---:|
| 1.11.29 | 40,786 | 254,226 | 3,980 | 62s |
| 2.2.28 | 43,673 | 274,297 | 4,200 | 70s |
| 3.2.25 | 46,643 | 300,457 | 4,396 | 68s |
| 4.2.20 | 49,774 | 325,550 | 4,563 | 73s |
| 5.2.4 | 52,355 | 346,672 | 4,652 | 80s |

### Latency

| Metric | Score |
|---|---|
| `so memory search` | p50=89 ms, p95=415 ms |
