//go:build !windows

package memory

import (
	"errors"
	"os"
)

func isDirLockContention(err error) bool {
	return errors.Is(err, os.ErrExist)
}
