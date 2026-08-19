//go:build darwin || linux || freebsd || netbsd || openbsd

package engine

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func acquireBuildLock(path string) (func(), error) {
	return flockBuildLock(path, syscall.LOCK_EX)
}

func tryAcquireBuildLock(path string) (func(), error) {
	unlock, err := flockBuildLock(path, syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return unlock, nil
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && (errno == syscall.EWOULDBLOCK || errno == syscall.EAGAIN) {
		return nil, ErrBuildInProgress
	}
	return nil, err
}

func flockBuildLock(path string, how int) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock graph build: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
