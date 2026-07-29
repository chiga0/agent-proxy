package xray

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/chiga0/agent-proxy/internal/config"
)

// Deploy installs and starts xray on the remote server via SSH.
func Deploy(cfg *config.Config) error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"SSH connectivity", func() error { return sshRun(cfg, "echo ok") }},
		{"Install xray", func() error { return installRemote(cfg) }},
		{"Write config", func() error { return writeRemoteConfig(cfg) }},
		{"Start xray", func() error { return startRemote(cfg) }},
	}

	for _, s := range steps {
		fmt.Printf("  → %s... ", s.name)
		if err := s.fn(); err != nil {
			fmt.Println("✗")
			return fmt.Errorf("%s: %w", s.name, err)
		}
		fmt.Println("✓")
	}
	return nil
}

func installRemote(cfg *config.Config) error {
	// Check if already installed
	err := sshRun(cfg, "test -f /usr/local/bin/xray")
	if err == nil {
		return nil // already installed
	}

	// Detect arch
	arch := "64"
	out, _ := sshOutput(cfg, "uname -m")
	if strings.Contains(strings.TrimSpace(out), "aarch64") {
		arch = "arm64-v8a"
	}

	script := fmt.Sprintf(`
curl -sL https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-%s.zip -o /tmp/xray.zip && \
cd /tmp && unzip -o xray.zip xray && mv xray /usr/local/bin/xray && chmod +x /usr/local/bin/xray && rm -f /tmp/xray.zip
`, arch)
	return sshRun(cfg, script)
}

func writeRemoteConfig(cfg *config.Config) error {
	confData, err := ServerConfig(cfg)
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("mkdir -p /usr/local/etc/xray && cat > /usr/local/etc/xray/config.json << 'XRAY_EOF'\n%s\nXRAY_EOF", string(confData))
	return sshRun(cfg, cmd)
}

func startRemote(cfg *config.Config) error {
	// Create systemd service
	unit := `[Unit]
Description=Xray Service
After=network.target

[Service]
ExecStart=/usr/local/bin/xray run -c /usr/local/etc/xray/config.json
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
`
	cmd := fmt.Sprintf(`cat > /etc/systemd/system/xray.service << 'UNIT_EOF'
%s
UNIT_EOF
systemctl daemon-reload && systemctl enable xray && systemctl restart xray && sleep 1 && systemctl is-active xray`, unit)
	return sshRun(cfg, cmd)
}

func sshArgs(cfg *config.Config) []string {
	args := []string{"-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10"}
	if cfg.Proxy.SSHKey != "" {
		args = append(args, "-i", cfg.Proxy.SSHKey)
	}
	return args
}

func sshTarget(cfg *config.Config) string {
	user := cfg.Proxy.SSHUser
	if user == "" {
		user = "root"
	}
	return fmt.Sprintf("%s@%s", user, cfg.Proxy.Host)
}

func sshRun(cfg *config.Config, cmd string) error {
	args := sshArgs(cfg)
	args = append(args, sshTarget(cfg), cmd)
	out, err := exec.Command("ssh", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func sshOutput(cfg *config.Config, cmd string) (string, error) {
	args := sshArgs(cfg)
	args = append(args, sshTarget(cfg), cmd)
	out, err := exec.Command("ssh", args...).Output()
	return string(out), err
}
