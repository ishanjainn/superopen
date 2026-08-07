---
name: go-focused-tests
description: Run focused Go checks for packages you touched.
---

# Go focused tests

When editing Go under `cmd/` or `internal/`:

```bash
go test -race -count=1 ./path/to/package
go vet ./path/to/package
```

Prefer package-scoped runs while iterating; broaden only before merge.
