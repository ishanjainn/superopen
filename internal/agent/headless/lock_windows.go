//go:build windows

package headless

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func TryLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	var ol windows.Overlapped
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 0xffffffff, 0xffffffff, &ol); err != nil {
		_ = f.Close()
		var errno windows.Errno
		if errors.As(err, &errno) && (errno == windows.ERROR_LOCK_VIOLATION || errno == windows.ERROR_IO_PENDING) {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("headless lock: %w", err)
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 0xffffffff, 0xffffffff, &ol)
		_ = f.Close()
	}, nil
}
