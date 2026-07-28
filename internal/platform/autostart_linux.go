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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create systemd user dir: %w", err)
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	if cfg.Proxy.Tunnel && cfg.Proxy.SSHKey != "" {
		if err := installTunnelUnit(cfg, dir); err != nil {
			return fmt.Errorf("install tunnel unit: %w", err)
		}
	} else {
		// Declarative cleanup: remove stale tunnel unit when tunnel is disabled
		removeUnit("agent-proxy-tunnel", dir)
	}
	if err := installPACUnit(self, dir); err != nil {
		return fmt.Errorf("install PAC unit: %w", err)
	}

	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("daemon-reload: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func removeUnit(name, dir string) {
	exec.Command("systemctl", "--user", "stop", name).Run()
	exec.Command("systemctl", "--user", "disable", name).Run()
	os.Remove(filepath.Join(dir, name+".service"))
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

func installTunnelUnit(cfg *config.Config, dir string) error {
	user := cfg.Proxy.SSHUserOrRoot()
	localPort := cfg.Proxy.LocalPort()
	remotePort := cfg.Proxy.Port

	unit := fmt.Sprintf(`[Unit]
Description=agent-proxy SSH tunnel
After=network-online.target

[Service]
ExecStart=/usr/bin/ssh -i %s -N -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -o ExitOnForwardFailure=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=%s -o BatchMode=yes -o Ciphers=aes128-gcm@openssh.com,aes256-gcm@openssh.com,chacha20-poly1305@openssh.com -o Compression=no -o ControlMaster=auto -o ControlPath=%s -o ControlPersist=600 -L %d:127.0.0.1:%d %s@%s
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, systemdQuote(cfg.Proxy.SSHKey), systemdQuote(config.KnownHostsPath()), systemdQuote(cfg.Proxy.SSHControlPath()), localPort, remotePort, user, cfg.Proxy.Host)

	path := filepath.Join(dir, "agent-proxy-tunnel.service")
	if err := os.WriteFile(path, []byte(unit), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("systemctl", "--user", "enable", "agent-proxy-tunnel").CombinedOutput(); err != nil {
		return fmt.Errorf("enable tunnel: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func installPACUnit(self string, dir string) error {
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
	if err := os.WriteFile(path, []byte(unit), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("systemctl", "--user", "enable", "agent-proxy-pac").CombinedOutput(); err != nil {
		return fmt.Errorf("enable PAC: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func UninstallAutoStart() error {
	var errs []string
	for _, name := range []string{"agent-proxy-tunnel", "agent-proxy-pac"} {
		if out, err := exec.Command("systemctl", "--user", "stop", name).CombinedOutput(); err != nil {
			if !strings.Contains(string(out), "not loaded") && !strings.Contains(string(out), "not found") {
				errs = append(errs, fmt.Sprintf("stop %s: %s", name, strings.TrimSpace(string(out))))
			}
		}
		exec.Command("systemctl", "--user", "disable", name).Run()
	}
	dir := systemdUserDir()
	for _, f := range []string{"agent-proxy-tunnel.service", "agent-proxy-pac.service"} {
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("remove %s: %v", f, err))
		}
	}
	exec.Command("systemctl", "--user", "daemon-reload").Run()
	if len(errs) > 0 {
		return fmt.Errorf("autostart cleanup: %s", strings.Join(errs, "; "))
	}
	return nil
}
