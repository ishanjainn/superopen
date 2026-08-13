//go:build windows

package memory

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// RemoveDirectory can leave a directory delete-pending while another goroutine
// still has a metadata handle open. During that handoff CreateDirectory reports
// access denied (or, less commonly, a sharing violation) instead of already
// exists. These errors mean the lock is still contended and should be retried.
func isDirLockContention(err error) bool {
	return errors.Is(err, os.ErrExist) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
