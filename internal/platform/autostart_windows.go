//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/chiga0/agent-proxy/internal/config"
)

const (
	pacTaskName    = "AgentProxyPAC"
	tunnelTaskName = "AgentProxyTunnel"
)

func InstallAutoStart(cfg *config.Config) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// PAC server task
	if err := createTask(pacTaskName, fmt.Sprintf(`"%s" serve-pac`, self)); err != nil {
		return fmt.Errorf("create PAC task: %w", err)
	}

	// SSH tunnel task (only when tunnel is enabled)
	if cfg.Proxy.Tunnel && cfg.Proxy.SSHKey != "" {
		user := cfg.Proxy.SSHUserOrRoot()
		sshArgs := fmt.Sprintf(
			`ssh -i "%s" -N -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -o ExitOnForwardFailure=yes -o StrictHostKeyChecking=accept-new -o BatchMode=yes -L %d:127.0.0.1:%d %s@%s`,
			cfg.Proxy.SSHKey, cfg.Proxy.LocalPort(), cfg.Proxy.Port, user, cfg.Proxy.Host,
		)
		if err := createTask(tunnelTaskName, sshArgs); err != nil {
			return fmt.Errorf("create tunnel task: %w", err)
		}
	}

	return nil
}

func createTask(name, command string) error {
	// Delete existing task if present
	exec.Command("schtasks", "/Delete", "/TN", name, "/F").Run()

	out, err := exec.Command("schtasks", "/Create",
		"/TN", name,
		"/TR", command,
		"/SC", "ONLOGON",
		"/RL", "LIMITED",
		"/F",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", string(out), err)
	}
	return nil
}

func UninstallAutoStart() {
	for _, name := range []string{pacTaskName, tunnelTaskName} {
		exec.Command("schtasks", "/Delete", "/TN", name, "/F").Run()
	}
}
