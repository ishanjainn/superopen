# Generate tests

Use when adding or updating tests for a changed file.

1. Prefer existing test frameworks and helpers in the repo.
2. Cover happy path + one failure case for new behavior.
3. Do not skip tests to "finish faster".
4. Run the relevant test target before claiming done.

## Superopen targets

- Go: `go test -race -count=1 ./path/to/pkg/...`
- Web: `cd web && npm test`
