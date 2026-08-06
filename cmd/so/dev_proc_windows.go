//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func setDetachedProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows Signal(0) is not supported; OpenProcess via FindProcess
	// succeeds for existing pids - try a no-op wait with timeout 0.
	const stillActive = 259 // STILL_ACTIVE
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	_ = p
	return code == stillActive
}

func killProcessTree(pid int) error {
	// taskkill /T kills the process tree (next + npm children).
	cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
	if err := cmd.Run(); err != nil {
		p, findErr := os.FindProcess(pid)
		if findErr != nil {
			return err
		}
		return p.Kill()
	}
	return nil
}
