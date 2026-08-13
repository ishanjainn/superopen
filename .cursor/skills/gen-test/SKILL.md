# gen-test

Generate and run tests following this repo's conventions.

## When to use

- Adding or changing behavior in Go CLI packages (`internal/`, `cmd/so/`)
- Adding or changing React/TypeScript UI in `web/`
- User asks for test coverage or a failing test reproduced

## Go

1. Find sibling `*_test.go` files in the target package for patterns (table-driven tests, testify, etc.).
2. Cover happy path plus at least one failure/edge case for new behavior.
3. Run focused tests: `go test -race -count=1 ./path/to/pkg/...`
4. Run `go vet ./...` before claiming done.

## Web

1. Follow existing Vitest patterns under `web/`.
2. Run `cd web && npm test` for affected suites.
3. Run `npm run typecheck` and `npm run lint`.

## Rules

- Do not skip tests to finish faster.
- Do not add trivial tests that only assert constants.
- Prefer testing real behavior over mocking everything.
