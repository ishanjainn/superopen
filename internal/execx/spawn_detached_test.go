package execx_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/execx"
)

func TestSpawnDetachedNoopUnderTest(t *testing.T) {
	// Must not re-exec the test binary.
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	execx.SpawnDetached(dir, "sessions", "finalize")
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("spawn should be a no-op under go test")
	}
}
