//go:build windows

package memory

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDirLockTreatsWindowsDeletePendingErrorsAsContention(t *testing.T) {
	for _, err := range []error{
		&os.PathError{Op: "mkdir", Path: `C:\runtime\memory-state.lock`, Err: windows.ERROR_ACCESS_DENIED},
		&os.PathError{Op: "mkdir", Path: `C:\runtime\memory-state.lock`, Err: windows.ERROR_SHARING_VIOLATION},
	} {
		if !isDirLockContention(err) {
			t.Fatalf("%v should be retried as lock contention", err)
		}
	}
}
