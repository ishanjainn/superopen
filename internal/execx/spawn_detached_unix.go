//go:build !windows

package execx

import (
	"os/exec"
	"syscall"
)

func detachFromTTY(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
