//go:build !unix

package engine

import (
	"errors"
	"io/fs"
)

func mmapFSFile(fs.FS, string) ([]byte, error) {
	return nil, errors.New("mmap is not available")
}
