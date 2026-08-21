//go:build !darwin && !linux && !windows

package engine

func systemRAMBytes() uint64 { return 0 }

func processRSSBytes() uint64 { return 0 }
