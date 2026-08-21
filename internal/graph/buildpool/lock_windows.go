//go:build windows

package buildpool

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func tryLockSlot(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	var ol windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 0xffffffff, 0xffffffff, &ol); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock slot: %w", err)
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 0xffffffff, 0xffffffff, &ol)
		_ = f.Close()
	}, nil
}
