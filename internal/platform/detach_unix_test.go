//go:build darwin || linux

package platform

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestDetachProcess(t *testing.T) {
	cmd := exec.Command("echo", "hello")
	DetachProcess(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("DetachProcess() did not set SysProcAttr")
	}
	if !cmd.SysProcAttr.Setsid {
		t.Error("DetachProcess() did not set Setsid = true")
	}
}

func TestDetachProcessPreservesExisting(t *testing.T) {
	cmd := exec.Command("echo", "hello")
	// Set something else first
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	DetachProcess(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr should not be nil")
	}
	if !cmd.SysProcAttr.Setsid {
		t.Error("Setsid should be true after DetachProcess")
	}
}
