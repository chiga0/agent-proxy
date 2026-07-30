//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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
			`ssh -i "%s" -N -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -o ExitOnForwardFailure=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile="%s" -o BatchMode=yes -o ControlMaster=auto -o ControlPath="%s" -o ControlPersist=600 -L %d:127.0.0.1:%d %s@%s`,
			cfg.Proxy.SSHKey, config.KnownHostsPath(), cfg.Proxy.SSHControlPath(), cfg.Proxy.LocalPort(), cfg.Proxy.Port, user, cfg.Proxy.Host,
		)
		if err := createTask(tunnelTaskName, sshArgs); err != nil {
			return fmt.Errorf("create tunnel task: %w", err)
		}
	} else {
		// Declarative cleanup: remove stale tunnel task when tunnel is disabled
		exec.Command("schtasks", "/Delete", "/TN", tunnelTaskName, "/F").Run()
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

// IsAutoStartInstalled returns true if the PAC server scheduled task exists.
func IsAutoStartInstalled() bool {
	err := exec.Command("schtasks", "/Query", "/TN", pacTaskName).Run()
	return err == nil
}

func UninstallAutoStart() error {
	var errs []string
	for _, name := range []string{pacTaskName, tunnelTaskName} {
		if out, err := exec.Command("schtasks", "/Delete", "/TN", name, "/F").CombinedOutput(); err != nil {
			output := strings.TrimSpace(string(out))
			if !strings.Contains(output, "not found") && !strings.Contains(output, "找不到") {
				errs = append(errs, fmt.Sprintf("delete %s: %s", name, output))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("autostart cleanup: %s", strings.Join(errs, "; "))
	}
	return nil
}
