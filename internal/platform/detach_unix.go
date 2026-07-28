//go:build darwin || linux

package platform

import (
	"os/exec"
	"syscall"
)

// DetachProcess configures cmd to run in a new session, fully detached
// from the parent's terminal. This prevents SIGHUP when the parent exits.
func DetachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
