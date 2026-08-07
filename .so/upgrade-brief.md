# Superopen upgrade brief

Coding agents: use **your own model** to upgrade `.so/` context/guardrails/evals.
Do **not** ask the user for an API key. Produce the JSON below, then run:

```bash
so apply-upgrade
```

(paste/write the JSON to stdin, or `so apply-upgrade path/to.json`)

## System instructions

You are a Superopen engineer. Given a compact repository profile (graph summary + existing agent instruction excerpts), produce a high-quality Superopen seed.

Return ONLY valid JSON (no markdown fences) with this shape:
{
  "architecture_md": "markdown doc: what the repo is, top packages/dirs, how services relate, where agents should look first",
  "conventions_md": "markdown doc: coding/PR/test conventions distilled from agent docs - imperative bullets, no DO/DON'T noise",
  "guardrails": {
    "rules": [
      {"id": "kebab-case-id", "description": "clear imperative rule", "severity": "block|warn", "source": "llm"}
    ]
  },
  "evals": {
    "checks": ["tests", "lint", "..."],
    "agent_rules": ["top rules for an LLM judge"],
    "judge_rubric": "short paragraph for session scoring"
  },
  "brief": "short AGENT.md brief pointing agents at .so/"
}

Rules for quality:
- Prefer 6-12 guardrails; dedupe.
- severity **block** ONLY for secrets/credentials (max 2). All other rules must be **warn** (style, tests, concurrency, SQL, PR titles, rate limits).
- Invent NO secrets or fake paths. Stay faithful to the profile.
- Include baseline: no-secrets (block), run-tests (warn), avoid-unrelated (warn).
- checks should include stack-appropriate ones (e.g. go_build, race_patterns, sql_parameterized, pr_title_convention).
- architecture_md and conventions_md should be useful to a coding agent in <4000 chars each.


## Repository profile

## Stack
Go

## Top-level structure
- CODE_OF_CONDUCT.md (file)
- CONTRIBUTING.md (file)
- LICENSE (file)
- Makefile (file)
- NOTICE (file)
- README.md (file)
- SECURITY.md (file)
- VERSION (file)
- bin (dir)
- cmd (dir)
- docs (dir)
- go.mod (file)
- go.sum (file)
- internal (dir)
- npm (dir)
- plugins (dir)
- scripts (dir)
- sdk (dir)
- templates (dir)
- web (dir)

## Graph summary
- nodes=3097 edges=7297
- top_dirs: internal, web, cmd, sdk, scripts, plugins, npm, state, internal_coding_pricing_tokens_test_go_t, internal_graph_graph_html_test_go_t
- languages: python=82, go=1673, typescript=1009
- sample_files: .github/scripts/release-packages.py, .github/scripts/release-packages.py, .github/scripts/release-packages.py, .github/scripts/release-packages.py, .github/scripts/release-packages.py, .github/scripts/release-packages.py, .github/scripts/release-packages.py, .github/scripts/release-packages.py, .github/scripts/release-packages.py, .github/scripts/release-packages.py, .github/scripts/release-packages.py, .github/scripts/release-packages.py

## Themes
- Go toolchain

## Derived rules (heuristic)
- Prefer Conventional Commit titles, for example feat: add project prune or

## Agent sources (excerpts)
### CONTRIBUTING.md (contributing)
Headings: Contributing to Superopen; Before you start; Development setup; Development workflow; Pull requests; Issues and questions
# Contributing to Superopen

Thanks for helping improve Superopen. Contributions of code, documentation, bug
reports, and integration feedback are all welcome.

## Before you start

- Read the [README](README.md) for install and product overview.
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md).
- For a substantial change, open or discuss an issue first so maintainers can
  confirm the scope. Small documentation fixes can go straight to a pull
  request.

## Development setup


Optional smoke (writes a local `.so/` under the current directory):


## Development workflow

1. Fork the repository and clone your fork.
2. Create a focused branch: `git switch -c fix/short-description`
3. Make the change, including tests and documentation when they are relevant.
   Keep unrelated formatting and refactors out of the pull request.
4. Run the focused checks for what you changed:

   | Area | Checks |
   |---|---|
   | Go / CLI | `go test -race -count=1 ./...` · `go vet ./...` |
   | Web UI | `cd web && npm test && npm run typecheck && npm run lint` |
   | Plugins | `bash scripts/sync-plugins.sh` then commit marketplace drift |

5. Commit with a clear subject, push, and open a pull request against `main`.

## Pull requests

- Explain the problem and the solution; link the related issue when one exists.
- Prefer Conventional Commit titles, for example `feat: add project prune` or
  `fix(web): restore file search`.
- Include tests for behavior changes and update user-facing documentation.
- Complete the pull-request template honestly.

## Issues and questions

Search existing [issues](https://github.com/ishanjainn/superopen/issues) before opening
a new one. Bug reports should include reproducible steps, expected and actual
behavior, and relevant non-sensitive logs.

Do not report…

