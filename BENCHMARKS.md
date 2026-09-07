# Superopen Benchmarks

Last updated: 2026-09-07. How to run: [benchmarks/README.md](benchmarks/README.md).

## System

| | |
|---|---|
| Machine | Ishans-MacBook-Pro.local · Apple M1 Pro · 8-core arm64 · 16 GB |
| OS | macOS 26.6.2 (darwin) |
| Agent | Claude Code 2.1.241 · `claude-sonnet-5` (claude-code) |
| Scale | small (LOCOMO n=100, LME n=50, compare 6, graph 12, SWE-bench 5/50 issues) |

## Run

| Metric | Combined | Native | Superopen | Extras |
|---|---|---|---|---|
| Duration | 4h 25m (temporal 14m 11s; memory_longmemeval 1h 0m; offline 0.3s; contradict 2.8s; latency 0.3s; graph 3m 18s; memory 1h 22m; compare 21m 40s; swe 1h 24m) | 13m 59s | 17m 34s | 3h 53m |
| Cost | $15.58 | $2.58 | $3.02 | $9.98 (SWE eval $0.00; LOCOMO judge $0.01; LME judge $0.01; other ledger) |

## Results

### SWE-bench (n=5)

| Correctness & efficiency | Cold Claude Code | Claude Code with Superopen | Improvement |
|---|---|---|---|
| Correctness | 1 / 5 (20%) | **4 / 5 (80%)** | **+60 pts** |
| Tokens | 19.1M | **9.6M** | **+50%** |
| Cost | $1.52 | **$0.87** | **+43%** |
| Tool calls | 81 | **44** | **+46%** |
| API requests | 82 | **45** | **+45%** |
| Wall-clock | 504s | **312s** | **+38%** |

<details>
<summary>Correctness over all instances</summary>

Correctness is scored on every instance. Tokens, cost, tool calls, API requests and wall-clock are over instances both arms resolved.

| SWE-bench instance | Cold Claude Code | Claude Code with Superopen | Token usage vs. cold | Tool usage vs. cold |
|---|---|---|---:|---:|
| `django-11532` | Failed | **Passed** | 173% | 123% |
| `django-16263` | Failed | **Passed** | 41% | 67% |
| `sphinx-9461` | Failed | Failed | 35% | 40% |
| `django-11400` | Failed | **Passed** | 73% | 71% |
| `pylint-6386` | Passed | **Passed** | 50% | 54% |

</details>

### Memory

#### LOCOMO (n=100 asked, 98 scored)

| System | recall@5 | recall@10 | Ingest cost |
|---|---|---|---|
| **Superopen** (`so memory recall`) | **88.8% (87/98)** | **90.8% (89/98)** | $0.00; so-bge-small-en-v1.5-384; pending=0; collapsed=0 skipped=0; worker=bge |
| BM25 | 83.7% (82/98) | 90.8% (89/98) | $0 (shared index) |
| RRF | 72.4% (71/98) | 86.7% (85/98) | $0 (shared index) |
| bow (bag-of-words) | 23.5% (23/98) | 40.8% (40/98) | $0 (shared index) |

QA accuracy: 31.2% (key-fact coverage); judge 88%; 5/16 scored; qa_n=20; 4 empty-gold skipped; claude-code.

Gold-in-store: hard_miss=0, rank_miss=9 of scored=98 (stored_docs=272). hard_miss = gold never captured; rank_miss = captured but not in top-10. BM25/bow/RRF search raw docs, Superopen searches the post-capture store.

#### LongMemEval-S (50)

| System | recall@5 | recall@10 | Ingest cost |
|---|---|---|---|
| **Superopen** (`so memory recall`) | **68.0% (34/50)** | **82.0% (41/50)** | $0.00; so-bge-small-en-v1.5-384; pending=0; collapsed=0 skipped=12; worker=bge |
| BM25 | 72.0% (36/50) | 76.0% (38/50) | $0 (shared index) |
| RRF | 58.0% (29/50) | 74.0% (37/50) | $0 (shared index) |
| bow (bag-of-words) | 6.0% (3/50) | 16.0% (8/50) | $0 (shared index) |

QA accuracy: 30.0% (key-fact coverage); judge 40%; 6/20 scored; qa_n=20; claude-code.

Gold-in-store: hard_miss=0, rank_miss=9 of scored=50 (stored_docs=2363). hard_miss = gold never captured; rank_miss = captured but not in top-10. BM25/bow/RRF search raw docs, Superopen searches the post-capture store.

#### Contradiction

| Metric | Superopen |
|---|---|
| Rescue@10 | 1.000 |
| historical-verbatim | 1.000 |

### Graph (Django 5.2.4)

| Metric | Score |
|---|---|
| Index time | 2m 54s |
| Index size | 52,355 nodes, 346,672 edges, 4,652 files |
| 12-probe score | 12/12 |
| Probe latency | p50=1757 ms, p95=3504 ms |

### Sessions (Django, 6)

| Metric | Score |
|---|---|
| Duration | 21m 40s |
| Key-fact coverage | 1.00 (native 1.00) |
| USD | $0.70 vs $1.06 native; 34% cheaper; cheaper on 7/7 questions |
| cache_read tokens | 5,722,372 vs 8,902,714 native (36% less cache_read) |

### Temporal (Django LTS)

| Checkpoint | Nodes | Edges | Files | `so init` |
|---|---:|---:|---:|---:|
| 1.11.29 | 40,786 | 254,226 | 3,980 | 157s |
| 2.2.28 | 43,673 | 274,297 | 4,200 | 163s |
| 3.2.25 | 46,643 | 300,457 | 4,396 | 164s |
| 4.2.20 | 49,774 | 325,550 | 4,563 | 171s |
| 5.2.4 | 52,355 | 346,672 | 4,652 | 186s |

### Latency

| Metric | Score |
|---|---|
| `so memory search` | p50=95 ms, p95=145 ms |
