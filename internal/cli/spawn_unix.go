//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

func detachFromTTY(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
