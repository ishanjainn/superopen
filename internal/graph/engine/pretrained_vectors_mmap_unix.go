//go:build unix

package engine

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func mmapFSFile(files fs.FS, name string) ([]byte, error) {
	file, err := files.Open(name)
	if err != nil {
		return nil, err
	}
	osFile, ok := file.(*os.File)
	if !ok {
		_ = file.Close()
		return nil, errors.New("asset is not an os.File")
	}
	info, err := osFile.Stat()
	if err != nil {
		_ = osFile.Close()
		return nil, err
	}
	size := int(info.Size())
	if size == 0 {
		_ = osFile.Close()
		return []byte{}, nil
	}
	data, err := unix.Mmap(int(osFile.Fd()), 0, size, unix.PROT_READ, unix.MAP_SHARED)
	_ = osFile.Close()
	if err != nil {
		return nil, err
	}
	return data, nil
}
