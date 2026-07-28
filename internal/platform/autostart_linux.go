//go:build linux

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chiga0/agent-proxy/internal/config"
)

func systemdUserDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

func InstallAutoStart(cfg *config.Config) error {
	dir := systemdUserDir()
	os.MkdirAll(dir, 0755)

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	if cfg.Proxy.Tunnel && cfg.Proxy.SSHKey != "" {
		installTunnelUnit(cfg, dir)
	}
	installPACUnit(self, dir)

	exec.Command("systemctl", "--user", "daemon-reload").Run()
	return nil
}

// systemdQuote quotes a string for systemd ExecStart using C-style escaping.
func systemdQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"'\\%") {
		return s
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `%`, `%%`)
	return `"` + s + `"`
}

func installTunnelUnit(cfg *config.Config, dir string) {
	user := cfg.Proxy.SSHUser
	if user == "" {
		user = "root"
	}
	localPort := cfg.Proxy.LocalPort()
	remotePort := cfg.Proxy.Port

	unit := fmt.Sprintf(`[Unit]
Description=agent-proxy SSH tunnel
After=network-online.target

[Service]
ExecStart=/usr/bin/ssh -i %s -N -o ServerAliveInterval=60 -o ServerAliveCountMax=3 -o ExitOnForwardFailure=yes -o StrictHostKeyChecking=accept-new -L %d:127.0.0.1:%d %s@%s
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, systemdQuote(cfg.Proxy.SSHKey), localPort, remotePort, user, cfg.Proxy.Host)

	path := filepath.Join(dir, "agent-proxy-tunnel.service")
	os.WriteFile(path, []byte(unit), 0644)
	exec.Command("systemctl", "--user", "enable", "agent-proxy-tunnel").Run()
	exec.Command("systemctl", "--user", "start", "agent-proxy-tunnel").Run()
}

func installPACUnit(self string, dir string) {
	unit := fmt.Sprintf(`[Unit]
Description=agent-proxy PAC server
After=network-online.target

[Service]
ExecStart=%s serve-pac
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
`, systemdQuote(self))

	path := filepath.Join(dir, "agent-proxy-pac.service")
	os.WriteFile(path, []byte(unit), 0644)
	exec.Command("systemctl", "--user", "enable", "agent-proxy-pac").Run()
	exec.Command("systemctl", "--user", "start", "agent-proxy-pac").Run()
}

func UninstallAutoStart() {
	for _, name := range []string{"agent-proxy-tunnel", "agent-proxy-pac"} {
		exec.Command("systemctl", "--user", "stop", name).Run()
		exec.Command("systemctl", "--user", "disable", name).Run()
	}
	dir := systemdUserDir()
	os.Remove(filepath.Join(dir, "agent-proxy-tunnel.service"))
	os.Remove(filepath.Join(dir, "agent-proxy-pac.service"))
	exec.Command("systemctl", "--user", "daemon-reload").Run()
}
