# Dataset layout

Academic datasets are **not** redistributed. Place files locally:

- `benchmarks/datasets/locomo/locomo10.json`
- `benchmarks/datasets/longmemeval/longmemeval_s.json`

The harness reads these paths when `--mode memory` runs.

CI / GitHub Actions may populate them from https URLs in
`SUPEROPEN_LOCOMO_URL` and `SUPEROPEN_LME_URL`.

SWE-bench Verified instance metadata is fetched into `benchmarks/cache/swebench/`
when `--mode swe` runs. Do not commit it.
