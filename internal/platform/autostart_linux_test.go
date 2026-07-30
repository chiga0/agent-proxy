//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestSystemdQuote(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no_special", "/usr/bin/ssh", "/usr/bin/ssh"},
		{"space", "/path/with space/bin", `"/path/with space/bin"`},
		{"tab", "a\tb", `"a	b"`},
		{"double_quote", `say"hi"`, `"say\"hi\""`},
		{"single_quote", "it's", `"it's"`},
		{"backslash", `C:\path`, `"C:\\path"`},
		{"percent", "100%", `"100%%"`},
		{"empty", "", ""},
		{"combined", `a "b" c\d %e`, `"a \"b\" c\\d %%e"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := systemdQuote(tt.input)
			if got != tt.want {
				t.Errorf("systemdQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSystemdUserDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := systemdUserDir()
	want := filepath.Join(tmpDir, ".config", "systemd", "user")
	if dir != want {
		t.Errorf("systemdUserDir() = %q, want %q", dir, want)
	}
}

func TestInstallPACUnit(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, "systemd", "user")
	os.MkdirAll(dir, 0755)

	self := "/usr/local/bin/agent-proxy"
	// installPACUnit calls systemctl enable which will fail in test,
	// but the file should still be written before that.
	installPACUnit(self, dir)

	path := filepath.Join(dir, "agent-proxy-pac.service")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read unit file: %v", err)
	}
	content := string(data)

	checks := []string{
		"[Unit]",
		"Description=agent-proxy PAC server",
		"After=network-online.target",
		"[Service]",
		"ExecStart=" + self + " serve-pac",
		"Restart=always",
		"RestartSec=3",
		"[Install]",
		"WantedBy=default.target",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("PAC unit missing %q", check)
		}
	}
}

func TestInstallTunnelUnit(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, "systemd", "user")
	os.MkdirAll(dir, 0755)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:   "proxy.example.com",
			Port:   22,
			SSHKey: "/home/user/.ssh/id_rsa",
			Tunnel: true,
		},
	}

	// installTunnelUnit calls systemctl enable which will fail in test,
	// but the file should still be written.
	installTunnelUnit(cfg, dir)

	path := filepath.Join(dir, "agent-proxy-tunnel.service")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read unit file: %v", err)
	}
	content := string(data)

	checks := []string{
		"[Unit]",
		"Description=agent-proxy SSH tunnel",
		"After=network-online.target",
		"[Service]",
		"ExecStart=/usr/bin/ssh",
		"-i /home/user/.ssh/id_rsa",
		"-N",
		"ServerAliveInterval=30",
		"ServerAliveCountMax=3",
		"ExitOnForwardFailure=yes",
		"StrictHostKeyChecking=yes",
		"BatchMode=yes",
		"ControlPersist=600",
		"-L 22:127.0.0.1:22",
		"root@proxy.example.com",
		"Restart=always",
		"RestartSec=5",
		"[Install]",
		"WantedBy=default.target",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("tunnel unit missing %q", check)
		}
	}
}

func TestInstallTunnelUnitCustomUserAndPort(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, "systemd", "user")
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

	installTunnelUnit(cfg, dir)

	path := filepath.Join(dir, "agent-proxy-tunnel.service")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read unit file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "-L 9999:127.0.0.1:2222") {
		t.Error("tunnel unit missing custom local port mapping")
	}
	if !strings.Contains(content, "deploy@myhost.io") {
		t.Error("tunnel unit missing custom user@host")
	}
}

func TestInstallPACUnitWithSpaces(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir := filepath.Join(tmpDir, "systemd", "user")
	os.MkdirAll(dir, 0755)

	self := "/opt/my apps/agent-proxy"
	installPACUnit(self, dir)

	path := filepath.Join(dir, "agent-proxy-pac.service")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read unit file: %v", err)
	}
	content := string(data)

	// Path with space should be quoted
	if !strings.Contains(content, `ExecStart="/opt/my apps/agent-proxy" serve-pac`) {
		t.Errorf("expected quoted path in ExecStart, got:\n%s", content)
	}
}
