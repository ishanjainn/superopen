# Go concurrency

- Prefer channels for ownership transfer; mutexes for shared state.
- Always run race-sensitive packages with `go test -race`.
- Never start a goroutine without a clear shutdown or WaitGroup story.
