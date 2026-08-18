//go:build windows

package engine

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func acquireBuildLock(path string) (func(), error) {
	return lockFileEx(path, windows.LOCKFILE_EXCLUSIVE_LOCK)
}

func tryAcquireBuildLock(path string) (func(), error) {
	unlock, err := lockFileEx(path, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err == nil {
		return unlock, nil
	}
	var errno windows.Errno
	if errors.As(err, &errno) && (errno == windows.ERROR_LOCK_VIOLATION || errno == windows.ERROR_IO_PENDING) {
		return nil, ErrBuildInProgress
	}
	return nil, err
}

func lockFileEx(path string, flags uint32) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	var ol windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 0xffffffff, 0xffffffff, &ol); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock graph build: %w", err)
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 0xffffffff, 0xffffffff, &ol)
		_ = f.Close()
	}, nil
}
