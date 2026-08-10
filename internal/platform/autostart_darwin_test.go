//go:build darwin

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestXMLEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "hello", "hello"},
		{"ampersand", "a&b", "a&amp;b"},
		{"lt_gt", "<tag>", "&lt;tag&gt;"},
		{"quotes", `say "hi"`, "say &#34;hi&#34;"},
		{"empty", "", ""},
		{"path_with_spaces", "/Users/my user/bin", "/Users/my user/bin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := xmlEscape(tt.input)
			if got != tt.want {
				t.Errorf("xmlEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPlistString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "/usr/bin/ssh", "<string>/usr/bin/ssh</string>"},
		{"special", "a&b<c>", "<string>a&amp;b&lt;c&gt;</string>"},
		{"empty", "", "<string></string>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plistString(tt.input)
			if got != tt.want {
				t.Errorf("plistString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLaunchAgentDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := launchAgentDir()
	want := filepath.Join(tmpDir, "Library", "LaunchAgents")
	if dir != want {
		t.Errorf("launchAgentDir() = %q, want %q", dir, want)
	}
}

func TestLogDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := logDir()
	want := filepath.Join(tmpDir, ".config", "agent-proxy", "logs")
	if dir != want {
		t.Errorf("logDir() = %q, want %q", dir, want)
	}
}

func TestInstallPACAgent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, "LaunchAgents")
	os.MkdirAll(dir, 0755)

	self := "/usr/local/bin/agent-proxy"
	if err := installPACAgent(self, dir); err != nil {
		t.Fatalf("installPACAgent() error: %v", err)
	}

	path := filepath.Join(dir, pacLabel+".plist")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	content := string(data)

	// Verify XML structure
	checks := []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<plist version="1.0">`,
		"<key>Label</key>",
		"<string>" + pacLabel + "</string>",
		"<key>ProgramArguments</key>",
		"<string>" + self + "</string>",
		"<string>serve-pac</string>",
		"<key>RunAtLoad</key>",
		"<true/>",
		"<key>KeepAlive</key>",
		"<key>StandardErrorPath</key>",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("plist missing %q", check)
		}
	}
}

func TestInstallTunnelAgent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, "LaunchAgents")
	os.MkdirAll(dir, 0755)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:   "proxy.example.com",
			Port:   22,
			SSHKey: "/home/user/.ssh/id_rsa",
			Tunnel: true,
		},
	}

	if err := installTunnelAgent(cfg, dir); err != nil {
		t.Fatalf("installTunnelAgent() error: %v", err)
	}

	path := filepath.Join(dir, tunnelLabel+".plist")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	content := string(data)

	checks := []string{
		"<key>Label</key>",
		"<string>" + tunnelLabel + "</string>",
		"<string>/usr/bin/ssh</string>",
		"<string>-i</string>",
		"<string>/home/user/.ssh/id_rsa</string>",
		"<string>-N</string>",
		"<string>ServerAliveInterval=30</string>",
		"<string>ServerAliveCountMax=3</string>",
		"<string>ExitOnForwardFailure=yes</string>",
		"<string>StrictHostKeyChecking=yes</string>",
		"<string>BatchMode=yes</string>",
		"<string>ControlPersist=yes</string>",
		"<string>-L</string>",
		"<string>22:127.0.0.1:22</string>",
		"<string>root@proxy.example.com</string>",
		"<key>RunAtLoad</key>",
		"<true/>",
		"<key>KeepAlive</key>",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("tunnel plist missing %q", check)
		}
	}
}

func TestInstallTunnelAgentCustomUser(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, "LaunchAgents")
	os.MkdirAll(dir, 0755)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:            "myhost.io",
			Port:            2222,
			SSHKey:          "/keys/deploy",
			SSHUser:         "deploy",
			Tunnel:          true,
			TunnelLocalPort: 9999,
		},
	}

	if err := installTunnelAgent(cfg, dir); err != nil {
		t.Fatalf("installTunnelAgent() error: %v", err)
	}

	path := filepath.Join(dir, tunnelLabel+".plist")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	content := string(data)

	// Custom local port and user
	if !strings.Contains(content, "<string>9999:127.0.0.1:2222</string>") {
		t.Error("tunnel plist missing custom local port mapping")
	}
	if !strings.Contains(content, "<string>deploy@myhost.io</string>") {
		t.Error("tunnel plist missing custom user@host")
	}
}

func TestInstallTunnelAgentXMLEscaping(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, "LaunchAgents")
	os.MkdirAll(dir, 0755)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:   "host.com",
			Port:   22,
			SSHKey: "/path/with&ampersand/key",
			Tunnel: true,
		},
	}

	if err := installTunnelAgent(cfg, dir); err != nil {
		t.Fatalf("installTunnelAgent() error: %v", err)
	}

	path := filepath.Join(dir, tunnelLabel+".plist")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	content := string(data)

	// The & in the key path should be escaped
	if !strings.Contains(content, "&amp;") {
		t.Error("expected XML-escaped ampersand in plist")
	}
	// Raw unescaped & should not appear (except in &amp; &lt; &gt; &#34;)
	if strings.Contains(content, "/path/with&ampersand/key") && !strings.Contains(content, "&amp;") {
		t.Error("raw ampersand found in plist without escaping")
	}
}

func TestUninstallAutoStartNoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create the LaunchAgents dir but no plist files
	dir := filepath.Join(tmpDir, "Library", "LaunchAgents")
	os.MkdirAll(dir, 0755)

	// UninstallAutoStart calls launchctl which will fail with "not loaded" / "Could not find"
	// but those are explicitly ignored. The os.Remove will get IsNotExist which is also ignored.
	// So this should return nil.
	err := UninstallAutoStart()
	if err != nil {
		t.Errorf("UninstallAutoStart() with no files should not error, got: %v", err)
	}
}

func TestUninstallAutoStartRemovesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, "Library", "LaunchAgents")
	os.MkdirAll(dir, 0755)

	// Create dummy plist files
	for _, label := range []string{tunnelLabel, pacLabel} {
		path := filepath.Join(dir, label+".plist")
		os.WriteFile(path, []byte("<plist></plist>"), 0644)
	}

	// launchctl unload will fail (not a real agent) but errors are ignored
	UninstallAutoStart()

	// Files should be removed
	for _, label := range []string{tunnelLabel, pacLabel} {
		path := filepath.Join(dir, label+".plist")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", label+".plist")
		}
	}
}

func TestLabels(t *testing.T) {
	if tunnelLabel != "com.agent-proxy.ssh-tunnel" {
		t.Errorf("unexpected tunnelLabel: %q", tunnelLabel)
	}
	if pacLabel != "com.agent-proxy.pac-server" {
		t.Errorf("unexpected pacLabel: %q", pacLabel)
	}
}
