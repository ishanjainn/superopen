//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func setDetachedProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func killProcessTree(pid int) error {
	// Negative pid: signal the process group started with Setsid.
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		p, findErr := os.FindProcess(pid)
		if findErr != nil {
			return err
		}
		return p.Signal(syscall.SIGTERM)
	}
	return nil
}
