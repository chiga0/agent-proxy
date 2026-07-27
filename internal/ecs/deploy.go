package ecs

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/chiga0/agent-proxy/internal/config"
)

func Deploy(cfg *config.Config) error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"Connectivity", func() error { return sshRun(cfg, "echo ok") }},
		{"Install Squid", installSquid},
	}

	if cfg.HasAuth() {
		steps = append(steps, struct {
			name string
			fn   func() error
		}{"Configure auth", func() error { return configureAuth(cfg) }})
	}

	steps = append(steps,
		struct {
			name string
			fn   func() error
		}{"Write Squid config", func() error { return writeSquidConfig(cfg) }},
		struct {
			name string
			fn   func() error
		}{"Restart Squid", func() error {
			return sshRun(cfg, "systemctl restart squid && sleep 1 && systemctl is-active squid")
		}},
	)

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

func RefreshIP(cfg *config.Config) error {
	ip, err := getPublicIP(cfg)
	if err != nil {
		return fmt.Errorf("get public IP: %w", err)
	}
	fmt.Printf("  Public IP: %s\n", ip)

	squidConf := generateSquidConfig(cfg, ip)
	cmd := fmt.Sprintf("cat > /etc/squid/squid.conf << 'EOF'\n%s\nEOF\nsystemctl restart squid", squidConf)
	return sshRun(cfg, cmd)
}

func CheckSSH(cfg *config.Config) error {
	return sshRun(cfg, "echo ok")
}

func installSquid() error {
	return sshRun(nil, "apt update -qq && apt install -y -qq squid apache2-utils >/dev/null 2>&1")
}

func configureAuth(cfg *config.Config) error {
	cmd := fmt.Sprintf(
		"htpasswd -cb /etc/squid/passwd %s %s 2>/dev/null && chmod 640 /etc/squid/passwd",
		cfg.Proxy.User, cfg.Proxy.Password)
	return sshRun(cfg, cmd)
}

func writeSquidConfig(cfg *config.Config) error {
	ip, _ := getPublicIP(cfg)
	conf := generateSquidConfig(cfg, ip)
	cmd := fmt.Sprintf("cat > /etc/squid/squid.conf << 'SQUID_EOF'\n%s\nSQUID_EOF", conf)
	return sshRun(cfg, cmd)
}

func generateSquidConfig(cfg *config.Config, trustedIP string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("http_port %d\n\n", cfg.Proxy.Port))

	// Auth is optional — SSH tunnel (127.0.0.1) and IP whitelist are the primary security
	if cfg.HasAuth() {
		b.WriteString("# Auth (optional, for direct access without SSH tunnel)\n")
		b.WriteString("auth_param basic program /usr/lib/squid/basic_ncsa_auth /etc/squid/passwd\n")
		b.WriteString("auth_param basic realm Agent-Proxy\n")
		b.WriteString("auth_param basic credentialsttl 2 hours\n")
		b.WriteString("acl authenticated proxy_auth REQUIRED\n\n")
	}

	// Always trust localhost (SSH tunnel) + user's public IP
	trustedSrcs := "127.0.0.1"
	if trustedIP != "" {
		trustedSrcs += " " + trustedIP
	}
	b.WriteString(fmt.Sprintf("# Trusted: SSH tunnel (127.0.0.1) + your public IP\n"))
	b.WriteString(fmt.Sprintf("acl trusted_ip src %s\n\n", trustedSrcs))

	b.WriteString("# CONNECT tunneling (HTTPS/WebSocket)\n")
	b.WriteString("acl SSL_ports port 443\n")
	b.WriteString("acl SSL_ports port 8443\n")
	b.WriteString("acl CONNECT method CONNECT\n")
	b.WriteString("http_access allow CONNECT SSL_ports trusted_ip\n")
	if cfg.HasAuth() {
		b.WriteString("http_access allow CONNECT SSL_ports authenticated\n")
	}
	b.WriteString("http_access allow trusted_ip\n")
	if cfg.HasAuth() {
		b.WriteString("http_access allow authenticated\n")
	}
	b.WriteString("http_access deny all\n\n")

	b.WriteString("# Privacy\n")
	b.WriteString("forwarded_for off\n")
	b.WriteString("request_header_access Via deny all\n\n")
	b.WriteString("# Logging\n")
	b.WriteString("access_log /var/log/squid/access.log squid\n")
	b.WriteString("cache_log /var/log/squid/cache.log\n\n")
	b.WriteString("# No caching\n")
	b.WriteString("cache deny all\n\n")
	b.WriteString("# DNS\n")
	b.WriteString("dns_nameservers 8.8.8.8 8.8.4.4\n")
	b.WriteString("visible_hostname agent-proxy\n\n")
	b.WriteString("# Performance\n")
	b.WriteString("server_persistent_connections on\n")
	b.WriteString("client_persistent_connections on\n")
	b.WriteString("persistent_request_timeout 30 seconds\n")
	b.WriteString("pconn_timeout 1 minute\n")

	return b.String()
}

func getPublicIP(cfg *config.Config) (string, error) {
	out, err := exec.Command("curl", "-s", "--max-time", "5", "https://ipinfo.io/ip").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func sshRun(cfg *config.Config, cmd string) error {
	args := []string{"-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10"}
	if cfg != nil && cfg.Proxy.SSHKey != "" {
		args = append(args, "-i", cfg.Proxy.SSHKey)
	}
	user := "root"
	host := ""
	if cfg != nil {
		if cfg.Proxy.SSHUser != "" {
			user = cfg.Proxy.SSHUser
		}
		host = cfg.Proxy.Host
	}
	args = append(args, fmt.Sprintf("%s@%s", user, host), cmd)

	out, err := exec.Command("ssh", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
