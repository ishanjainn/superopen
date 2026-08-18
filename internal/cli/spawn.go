package cli

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
)

// SpawnDetached re-execs the current binary as a fire-and-forget child running
// args. The child survives the parent (new session / process group), inherits
// env, and discards stdout/stderr. Best-effort: errors are swallowed.
//
// dir is the child working directory (os.TempDir when empty) so the child does
// not pin the parent's cwd. Prefer the repo root when spawning so sessions
// finalize / harvest can find .so/.
//
// No-op under `go test` (re-exec would fork the test binary).
func SpawnDetached(dir string, args ...string) {
	if testing.Testing() {
		return
	}
	executable, err := os.Executable()
	if err != nil || executable == "" {
		return
	}
	cmd := exec.CommandContext(context.Background(), executable, args...)
	detachFromTTY(cmd)
	if dir != "" {
		cmd.Dir = dir
	} else {
		cmd.Dir = os.TempDir()
	}
	cmd.Env = os.Environ()
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return
	}
	_ = cmd.Process.Release()
}

// SpawnSO is SpawnDetached using the current so binary with the given args.
func SpawnSO(repoRoot string, args ...string) {
	SpawnDetached(repoRoot, args...)
}
