//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/chiga0/agent-proxy/internal/config"
)

const taskName = "AgentProxyPAC"

func InstallAutoStart(cfg *config.Config) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// Delete existing task if present
	exec.Command("schtasks", "/Delete", "/TN", taskName, "/F").Run()

	// Create scheduled task that runs at logon
	out, err := exec.Command("schtasks", "/Create",
		"/TN", taskName,
		"/TR", fmt.Sprintf(`"%s" serve-pac`, self),
		"/SC", "ONLOGON",
		"/RL", "LIMITED",
		"/F",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create scheduled task: %s: %w", string(out), err)
	}
	return nil
}

func UninstallAutoStart() {
	exec.Command("schtasks", "/Delete", "/TN", taskName, "/F").Run()
}
