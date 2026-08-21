//go:build linux

package engine

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func systemRAMBytes() uint64 {
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return 0
	}
	return uint64(info.Totalram) * uint64(info.Unit)
}

func processRSSBytes() uint64 {
	body, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(body))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * uint64(os.Getpagesize())
}
