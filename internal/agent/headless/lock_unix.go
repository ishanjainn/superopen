//go:build darwin || linux || freebsd || netbsd || openbsd

package headless

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// TryLock acquires a non-blocking exclusive flock. Unlock must be called.
func TryLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		var errno syscall.Errno
		if errors.As(err, &errno) && (errno == syscall.EWOULDBLOCK || errno == syscall.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("headless lock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
