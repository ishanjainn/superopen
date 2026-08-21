//go:build darwin

package engine

import "golang.org/x/sys/unix"

func systemRAMBytes() uint64 {
	n, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return n
}

func processRSSBytes() uint64 {
	var usage unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	if usage.Maxrss <= 0 {
		return 0
	}
	return uint64(usage.Maxrss)
}
