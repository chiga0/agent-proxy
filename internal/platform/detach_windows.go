//go:build windows

package platform

import (
	"os/exec"
	"syscall"
)

// DetachProcess configures cmd to run in a new process group on Windows.
func DetachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
