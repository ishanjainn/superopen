package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/cli"
)

func TestSpawnDetachedNoopUnderTest(t *testing.T) {
	// Must not re-exec the test binary.
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	cli.SpawnDetached(dir, "sessions", "finalize")
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("spawn should be a no-op under go test")
	}
}
