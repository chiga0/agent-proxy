package ecs

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/chiga0/agent-proxy/internal/config"
)

var validUserRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func Deploy(cfg *config.Config) error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"Connectivity", func() error { return sshRun(cfg, "echo ok") }},
		{"Install Squid", func() error { return installSquid(cfg) }},
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
		}{"System tuning", func() error {
			return sshRun(cfg, `grep -q tcp_fastopen /etc/sysctl.conf || echo "net.ipv4.tcp_fastopen = 3" >> /etc/sysctl.conf; sysctl -w net.ipv4.tcp_fastopen=3 >/dev/null 2>&1; echo ok`)
		}},
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
	ip, err := getPublicIP()
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

func installSquid(cfg *config.Config) error {
	return sshRun(cfg, "apt update -qq && apt install -y -qq squid apache2-utils >/dev/null 2>&1")
}

func configureAuth(cfg *config.Config) error {
	if !validUserRe.MatchString(cfg.Proxy.User) {
		return fmt.Errorf("invalid proxy username: %q (allowed: a-z, 0-9, . _ -)", cfg.Proxy.User)
	}
	// Pass password via stdin to avoid shell injection
	cmd := fmt.Sprintf(
		"htpasswd -cbi /etc/squid/passwd %s 2>/dev/null && chmod 640 /etc/squid/passwd",
		cfg.Proxy.User)
	return sshRunWithStdin(cfg, cmd, cfg.Proxy.Password)
}

func writeSquidConfig(cfg *config.Config) error {
	ip, err := getPublicIP()
	if err != nil {
		// Tunnel mode: only 127.0.0.1 is needed, public IP is optional
		if cfg.Proxy.Tunnel {
			ip = ""
		} else if !cfg.HasAuth() {
			return fmt.Errorf("cannot get public IP for IP whitelist (required in direct mode without auth): %w", err)
		} else {
			// Direct mode with auth: IP whitelist is nice-to-have
			ip = ""
		}
	}
	conf := generateSquidConfig(cfg, ip)
	cmd := fmt.Sprintf("cat > /etc/squid/squid.conf << 'SQUID_EOF'\n%s\nSQUID_EOF", conf)
	return sshRun(cfg, cmd)
}

func generateSquidConfig(cfg *config.Config, trustedIP string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("http_port %d\n\n", cfg.Proxy.Port))

	if cfg.HasAuth() {
		b.WriteString("# Auth (optional, for direct access without SSH tunnel)\n")
		b.WriteString("auth_param basic program /usr/lib/squid/basic_ncsa_auth /etc/squid/passwd\n")
		b.WriteString("auth_param basic realm Agent-Proxy\n")
		b.WriteString("auth_param basic credentialsttl 2 hours\n")
		b.WriteString("acl authenticated proxy_auth REQUIRED\n\n")
	}

	trustedSrcs := "127.0.0.1"
	if trustedIP != "" {
		trustedSrcs += " " + trustedIP
	}
	b.WriteString("# Trusted: SSH tunnel (127.0.0.1) + your public IP\n")
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
	b.WriteString("persistent_request_timeout 60 seconds\n")
	b.WriteString("pconn_timeout 2 minutes\n")
	b.WriteString("half_closed_clients off\n")
	b.WriteString("read_timeout 5 minutes\n")
	b.WriteString("connect_timeout 10 seconds\n\n")
	b.WriteString("# DNS cache\n")
	b.WriteString("positive_dns_ttl 1 hours\n")
	b.WriteString("negative_dns_ttl 30 seconds\n\n")
	b.WriteString("# File descriptors\n")
	b.WriteString("max_filedescriptors 65536\n\n")
	b.WriteString("# Request deduplication\n")
	b.WriteString("collapsed_forwarding on\n")

	return b.String()
}

func getPublicIP() (string, error) {
	out, err := exec.Command("curl", "-s", "--max-time", "5", "https://ipinfo.io/ip").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func sshArgs(cfg *config.Config) []string {
	args := []string{"-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10"}
	if cfg != nil && cfg.Proxy.SSHKey != "" {
		args = append(args, "-i", cfg.Proxy.SSHKey)
	}
	return args
}

func sshTarget(cfg *config.Config) string {
	user := "root"
	host := ""
	if cfg != nil {
		if cfg.Proxy.SSHUser != "" {
			user = cfg.Proxy.SSHUser
		}
		host = cfg.Proxy.Host
	}
	return fmt.Sprintf("%s@%s", user, host)
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

func sshRunWithStdin(cfg *config.Config, cmd string, stdin string) error {
	args := sshArgs(cfg)
	args = append(args, sshTarget(cfg), cmd)

	proc := exec.Command("ssh", args...)
	proc.Stdin = strings.NewReader(stdin + "\n")
	out, err := proc.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
