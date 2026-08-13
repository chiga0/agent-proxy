//go:build darwin

package platform

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chiga0/agent-proxy/internal/config"
)

const (
	tunnelLabel = "com.agent-proxy.ssh-tunnel"
	pacLabel    = "com.agent-proxy.pac-server"
)

func launchAgentDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents")
}

func logDir() string {
	return filepath.Join(config.DataDir(), "logs")
}

func InstallAutoStart(cfg *config.Config) error {
	dir := launchAgentDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	if err := os.MkdirAll(logDir(), 0700); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	if cfg.Proxy.Tunnel && cfg.Proxy.SSHKey != "" {
		if err := installTunnelAgent(cfg, dir); err != nil {
			return fmt.Errorf("install tunnel agent: %w", err)
		}
		loadAgent(tunnelLabel, dir)
	} else {
		// Declarative cleanup: remove stale tunnel agent when tunnel is disabled
		removeAgent(tunnelLabel, dir)
	}
	if err := installPACAgent(self, dir); err != nil {
		return fmt.Errorf("install PAC agent: %w", err)
	}
	loadAgent(pacLabel, dir)
	return nil
}

func loadAgent(label, dir string) {
	path := filepath.Join(dir, label+".plist")
	exec.Command("launchctl", "load", path).Run()
}

func removeAgent(label, dir string) {
	path := filepath.Join(dir, label+".plist")
	exec.Command("launchctl", "unload", path).Run()
	os.Remove(path)
}

// xmlEscape escapes a string for safe embedding in plist XML.
func xmlEscape(s string) string {
	var b xmlEncoder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

type xmlEncoder struct{ data []byte }

func (e *xmlEncoder) Write(p []byte) (int, error) { e.data = append(e.data, p...); return len(p), nil }
func (e *xmlEncoder) String() string              { return string(e.data) }

func plistString(s string) string {
	return "<string>" + xmlEscape(s) + "</string>"
}

func installTunnelAgent(cfg *config.Config, dir string) error {
	user := cfg.Proxy.SSHUserOrRoot()
	localPort := cfg.Proxy.LocalPort()
	remotePort := cfg.Proxy.Port

	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    ` + plistString(tunnelLabel) + `
    <key>ProgramArguments</key>
    <array>
        <string>/usr/bin/ssh</string>
        <string>-i</string>
        ` + plistString(cfg.Proxy.SSHKey) + `
        <string>-N</string>
        <string>-o</string>
        <string>ServerAliveInterval=15</string>
        <string>-o</string>
        <string>ServerAliveCountMax=2</string>
        <string>-o</string>
        <string>ExitOnForwardFailure=yes</string>
        <string>-o</string>
        <string>StrictHostKeyChecking=yes</string>
        <string>-o</string>
        ` + plistString("UserKnownHostsFile="+config.KnownHostsPath()) + `
        <string>-o</string>
        <string>BatchMode=yes</string>
        <string>-o</string>
        <string>Ciphers=aes128-gcm@openssh.com,aes256-gcm@openssh.com,chacha20-poly1305@openssh.com</string>
        <string>-o</string>
        <string>Compression=no</string>
        <string>-o</string>
        <string>ControlMaster=auto</string>
        <string>-o</string>
        ` + plistString("ControlPath="+cfg.Proxy.SSHControlPath()) + `
        <string>-o</string>
        <string>ControlPersist=yes</string>
        <string>-L</string>
        ` + plistString(fmt.Sprintf("%d:127.0.0.1:%d", localPort, remotePort)) + `
        ` + plistString(fmt.Sprintf("%s@%s", user, cfg.Proxy.Host)) + `
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardErrorPath</key>
    ` + plistString(filepath.Join(logDir(), "ssh-tunnel.log")) + `
</dict>
</plist>
`
	path := filepath.Join(dir, tunnelLabel+".plist")
	return os.WriteFile(path, []byte(plist), 0644)
}

func installPACAgent(self string, dir string) error {
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    ` + plistString(pacLabel) + `
    <key>ProgramArguments</key>
    <array>
        ` + plistString(self) + `
        <string>serve-pac</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardErrorPath</key>
    ` + plistString(filepath.Join(logDir(), "pac-server.log")) + `
</dict>
</plist>
`
	path := filepath.Join(dir, pacLabel+".plist")
	return os.WriteFile(path, []byte(plist), 0644)
}

// IsAutoStartInstalled returns true if the PAC server LaunchAgent plist exists.
func IsAutoStartInstalled() bool {
	_, err := os.Stat(filepath.Join(launchAgentDir(), pacLabel+".plist"))
	return err == nil
}

func UninstallAutoStart() error {
	var errs []string
	dir := launchAgentDir()
	for _, label := range []string{tunnelLabel, pacLabel} {
		path := filepath.Join(dir, label+".plist")
		if out, err := exec.Command("launchctl", "unload", path).CombinedOutput(); err != nil {
			// Ignore "not loaded" errors
			if !strings.Contains(string(out), "not loaded") && !strings.Contains(string(out), "Could not find") {
				errs = append(errs, fmt.Sprintf("unload %s: %s", label, strings.TrimSpace(string(out))))
			}
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("remove %s: %v", label, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("autostart cleanup: %s", strings.Join(errs, "; "))
	}
	return nil
}
