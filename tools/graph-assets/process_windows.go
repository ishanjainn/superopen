//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"syscall"
)

func configureCompilerCommand(command *exec.Cmd) {
	const createNewProcessGroup = 0x00000200
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		killer := exec.Command("taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
		if err := killer.Run(); err != nil {
			return command.Process.Kill()
		}
		return nil
	}
}
