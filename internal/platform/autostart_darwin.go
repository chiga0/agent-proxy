//go:build darwin

package platform

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

func InstallAutoStart(cfg *config.Config) error {
	dir := launchAgentDir()
	os.MkdirAll(dir, 0755)

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	if cfg.Proxy.Tunnel && cfg.Proxy.SSHKey != "" {
		installTunnelAgent(cfg, dir)
	}
	installPACAgent(self, dir)
	return nil
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

func installTunnelAgent(cfg *config.Config, dir string) {
	user := cfg.Proxy.SSHUser
	if user == "" {
		user = "root"
	}
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
        <string>ServerAliveInterval=60</string>
        <string>-o</string>
        <string>ServerAliveCountMax=3</string>
        <string>-o</string>
        <string>ExitOnForwardFailure=yes</string>
        <string>-o</string>
        <string>StrictHostKeyChecking=accept-new</string>
        <string>-L</string>
        ` + plistString(fmt.Sprintf("%d:127.0.0.1:%d", localPort, remotePort)) + `
        ` + plistString(fmt.Sprintf("%s@%s", user, cfg.Proxy.Host)) + `
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardErrorPath</key>
    <string>/tmp/agent-proxy-ssh-tunnel.log</string>
</dict>
</plist>
`
	path := filepath.Join(dir, tunnelLabel+".plist")
	os.WriteFile(path, []byte(plist), 0644)
	exec.Command("launchctl", "load", path).Run()
}

func installPACAgent(self string, dir string) {
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
    <string>/tmp/agent-proxy-pac-server.log</string>
</dict>
</plist>
`
	path := filepath.Join(dir, pacLabel+".plist")
	os.WriteFile(path, []byte(plist), 0644)
	exec.Command("launchctl", "load", path).Run()
}

func UninstallAutoStart() {
	dir := launchAgentDir()
	for _, label := range []string{tunnelLabel, pacLabel} {
		path := filepath.Join(dir, label+".plist")
		exec.Command("launchctl", "unload", path).Run()
		os.Remove(path)
	}
}
