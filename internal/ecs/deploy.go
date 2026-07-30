package ecs

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
)

var squidTmpRe = regexp.MustCompile(`^/etc/squid/squid\.conf\.[A-Za-z0-9]+$`)

func Deploy(cfg *config.Config) error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"Connectivity", func() error { return sshRun(cfg, "echo ok") }},
		{"Internet access", func() error { return checkECSInternet(cfg) }},
		{"Install Squid", func() error { return installSquid(cfg) }},
		{"System tuning", func() error {
			return sshRun(cfg, `grep -q tcp_fastopen /etc/sysctl.conf 2>/dev/null || echo "net.ipv4.tcp_fastopen = 3" >> /etc/sysctl.conf; sysctl -w net.ipv4.tcp_fastopen=3 >/dev/null 2>&1`)
		}},
		{"Write Squid config", func() error { return writeSquidConfig(cfg) }},
	}

	for _, s := range steps {
		fmt.Printf("  → %s... ", s.name)
		if err := s.fn(); err != nil {
			if s.name == "System tuning" {
				// Best-effort: show warning but continue
				fmt.Printf("⚠ (non-fatal: %v)\n", err)
				continue
			}
			fmt.Println("✗")
			return fmt.Errorf("%s: %w", s.name, err)
		}
		fmt.Println("✓")
	}
	return nil
}

func RefreshIP(cfg *config.Config) error {
	if cfg.Proxy.Tunnel {
		fmt.Println("  Tunnel mode: Squid listens on loopback only — no IP whitelist needed.")
		return nil
	}

	ip, err := getPublicIP()
	if err != nil {
		return fmt.Errorf("get public IP: %w", err)
	}
	fmt.Printf("  Public IP: %s\n", ip)

	dnsServers := fetchECSDNS(cfg)
	conf := generateSquidConfig(cfg, ip, dnsServers)
	return deploySquidConfig(cfg, conf)
}

func CheckSSH(cfg *config.Config) error {
	return sshRun(cfg, "echo ok")
}

func installSquid(cfg *config.Config) error {
	cmd := `if command -v apt >/dev/null 2>&1; then
		apt update -qq && apt install -y -qq squid
	elif command -v yum >/dev/null 2>&1; then
		yum install -y -q squid
	elif command -v apk >/dev/null 2>&1; then
		apk add --quiet squid
	else
		echo "ERROR: unsupported package manager (need apt, yum, or apk)" >&2
		exit 1
	fi`
	if err := sshRun(cfg, cmd); err != nil {
		return err
	}
	// Enable squid to start on boot
	enableCmd := `if command -v systemctl >/dev/null 2>&1; then
		systemctl enable squid >/dev/null 2>&1
	elif command -v rc-update >/dev/null 2>&1; then
		rc-update add squid default >/dev/null 2>&1
	elif command -v chkconfig >/dev/null 2>&1; then
		chkconfig squid on >/dev/null 2>&1
	fi`
	return sshRun(cfg, enableCmd)
}

// checkECSInternet verifies the ECS can reach the internet (needed for package install).
func checkECSInternet(cfg *config.Config) error {
	cmd := `curl -s --max-time 5 -o /dev/null http://archive.ubuntu.com 2>/dev/null || wget -q --timeout=5 -O /dev/null http://archive.ubuntu.com 2>/dev/null || exit 1`
	if err := sshRun(cfg, cmd); err != nil {
		return fmt.Errorf("ECS cannot reach the internet — check EIP/NAT gateway/security group outbound rules")
	}
	return nil
}

// restartSquidCmd returns the appropriate restart command for the ECS's init system.
func restartSquidCmd() string {
	return `if command -v systemctl >/dev/null 2>&1; then
		systemctl restart squid && sleep 1 && systemctl is-active squid
	elif command -v rc-service >/dev/null 2>&1; then
		rc-service squid restart && sleep 1 && rc-service squid status
	elif command -v service >/dev/null 2>&1; then
		service squid restart && sleep 1 && service squid status
	else
		echo "ERROR: no init system found (need systemctl, rc-service, or service)" >&2
		exit 1
	fi`
}

// rollbackSquidCmd returns the appropriate rollback command.
func rollbackSquidCmd() string {
	return `if [ -f /etc/squid/squid.conf.bak ]; then
		cp /etc/squid/squid.conf.bak /etc/squid/squid.conf
		if command -v systemctl >/dev/null 2>&1; then systemctl restart squid
		elif command -v rc-service >/dev/null 2>&1; then rc-service squid restart
		elif command -v service >/dev/null 2>&1; then service squid restart
		fi
		echo ROLLBACK_OK
	fi`
}

// writeSquidConfig generates and deploys the Squid configuration.
// In tunnel mode, Squid listens on loopback only and no public IP is fetched.
func writeSquidConfig(cfg *config.Config) error {
	var trustedIP string
	if !cfg.Proxy.Tunnel {
		ip, err := getPublicIP()
		if err != nil {
			return fmt.Errorf("cannot get public IP for direct mode: %w", err)
		}
		trustedIP = ip
	}
	dnsServers := fetchECSDNS(cfg)
	conf := generateSquidConfig(cfg, trustedIP, dnsServers)
	return deploySquidConfig(cfg, conf)
}

// fetchECSDNS reads nameservers from the ECS's /etc/resolv.conf.
// Falls back to 8.8.8.8 8.8.4.4 if unreadable.
func fetchECSDNS(cfg *config.Config) string {
	out, err := sshRunOutput(cfg, `grep '^nameserver' /etc/resolv.conf 2>/dev/null | head -2 | awk '{print $2}' | tr '\n' ' '`)
	if err != nil {
		return "8.8.8.8 8.8.4.4"
	}
	servers := strings.TrimSpace(out)
	if servers == "" {
		return "8.8.8.8 8.8.4.4"
	}
	return servers
}

// deploySquidConfig writes the Squid config transactionally:
// temp file → syntax check → backup → atomic replace → restart → health check → rollback on failure.
func deploySquidConfig(cfg *config.Config, conf string) error {
	// 1. Write to a unique temp file on the remote
	tmpCmd := "mktemp /etc/squid/squid.conf.XXXXXX"
	tmpOut, err := sshRunOutput(cfg, tmpCmd)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := strings.TrimSpace(tmpOut)
	// Strict validation: guard against SSH banner/warning pollution
	if !squidTmpRe.MatchString(tmpPath) {
		return fmt.Errorf("unexpected mktemp output: %q", tmpPath)
	}

	writeCmd := fmt.Sprintf("cat > %s << 'SQUID_EOF'\n%s\nSQUID_EOF\nchmod 644 %s", tmpPath, conf, tmpPath)
	if err := sshRun(cfg, writeCmd); err != nil {
		sshRun(cfg, "rm -f "+tmpPath)
		return fmt.Errorf("write temp config: %w", err)
	}

	// 2. Syntax check — must pass before we touch the real config
	parseCmd := fmt.Sprintf("squid -k parse -f %s 2>&1", tmpPath)
	if err := sshRun(cfg, parseCmd); err != nil {
		sshRun(cfg, "rm -f "+tmpPath)
		return fmt.Errorf("squid config syntax check failed: %w", err)
	}

	// 3. Backup existing config (check success before replacing)
	backupCmd := "if [ -f /etc/squid/squid.conf ]; then cp /etc/squid/squid.conf /etc/squid/squid.conf.bak || exit 1; fi"
	if err := sshRun(cfg, backupCmd); err != nil {
		sshRun(cfg, "rm -f "+tmpPath)
		return fmt.Errorf("backup config failed — aborting: %w", err)
	}

	// 4. Atomic replace
	replaceCmd := fmt.Sprintf("mv %s /etc/squid/squid.conf", tmpPath)
	if err := sshRun(cfg, replaceCmd); err != nil {
		sshRun(cfg, "rm -f "+tmpPath)
		return fmt.Errorf("replace config: %w", err)
	}

	// 5. Restart and health check
	if err := sshRun(cfg, restartSquidCmd()); err != nil {
		// Rollback: restore backup and restart
		rbOut, rbErr := sshRunOutput(cfg, rollbackSquidCmd())
		if rbErr == nil && strings.Contains(rbOut, "ROLLBACK_OK") {
			return fmt.Errorf("squid restart failed — config rolled back successfully: %w", err)
		}
		return fmt.Errorf("squid restart failed and rollback also failed: %w", err)
	}

	return nil
}

// CheckSquidListenMode checks whether ALL ECS Squid http_port directives
// are loopback-only. Returns (loopbackOnly bool, detail string, err error).
func CheckSquidListenMode(cfg *config.Config) (bool, string, error) {
	out, err := sshRunOutput(cfg, "grep -E '^http_port' /etc/squid/squid.conf 2>/dev/null || echo 'NO_CONFIG'")
	if err != nil {
		return false, "", fmt.Errorf("read squid config: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 1 && (lines[0] == "NO_CONFIG" || lines[0] == "") {
		return false, "no Squid config found on ECS", nil
	}
	var details []string
	allLoopback := true
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		details = append(details, line)
		if !strings.Contains(line, "127.0.0.1:") {
			allLoopback = false
		}
	}
	return allLoopback, strings.Join(details, "; "), nil
}

func generateSquidConfig(cfg *config.Config, trustedIP string, dnsServers string) string {
	var b strings.Builder

	// Listen address: loopback-only in tunnel mode, all interfaces in direct mode
	if cfg.Proxy.Tunnel {
		b.WriteString(fmt.Sprintf("http_port 127.0.0.1:%d\n\n", cfg.Proxy.Port))
	} else {
		b.WriteString(fmt.Sprintf("http_port %d\n\n", cfg.Proxy.Port))
	}

	// Trusted sources
	trustedSrcs := "127.0.0.1"
	if trustedIP != "" {
		trustedSrcs += " " + trustedIP
	}
	b.WriteString(fmt.Sprintf("# Trusted: SSH tunnel (127.0.0.1)%s\n", func() string {
		if trustedIP != "" {
			return " + your public IP"
		}
		return ""
	}()))
	b.WriteString(fmt.Sprintf("acl trusted_ip src %s\n\n", trustedSrcs))

	// Port and method ACLs
	b.WriteString("# Safe ports and CONNECT restrictions\n")
	b.WriteString("acl Safe_ports port 80 443 8443\n")
	b.WriteString("acl SSL_ports port 443 8443\n")
	b.WriteString("acl CONNECT method CONNECT\n\n")

	// Dangerous destination ACLs
	b.WriteString("# Block access to local/private/metadata targets\n")
	b.WriteString("acl to_localhost dst 127.0.0.0/8 ::1\n")
	b.WriteString("acl to_linklocal dst 169.254.0.0/16 fe80::/10\n")
	b.WriteString("acl to_rfc1918 dst 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fc00::/7\n")
	b.WriteString("acl to_metadata dst 169.254.169.254 100.100.100.200\n\n")

	// Deny-first access rules
	b.WriteString("# Deny-first access rules\n")
	b.WriteString("http_access deny !Safe_ports\n")
	b.WriteString("http_access deny CONNECT !SSL_ports\n")
	b.WriteString("http_access deny to_localhost\n")
	b.WriteString("http_access deny to_linklocal\n")
	b.WriteString("http_access deny to_rfc1918\n")
	b.WriteString("http_access deny to_metadata\n")
	b.WriteString("http_access allow trusted_ip\n")
	b.WriteString("http_access deny all\n\n")

	// Privacy
	b.WriteString("# Privacy\n")
	b.WriteString("forwarded_for off\n")
	b.WriteString("request_header_access Via deny all\n\n")

	// Logging
	b.WriteString("# Logging\n")
	b.WriteString("access_log /var/log/squid/access.log squid\n")
	b.WriteString("cache_log /var/log/squid/cache.log\n\n")

	// No caching
	b.WriteString("# No caching\n")
	b.WriteString("cache deny all\n\n")

	// DNS
	b.WriteString("# DNS\n")
	b.WriteString(fmt.Sprintf("dns_nameservers %s\n", dnsServers))
	b.WriteString("visible_hostname agent-proxy\n\n")

	// Performance
	b.WriteString("# Performance\n")
	b.WriteString("server_persistent_connections on\n")
	b.WriteString("client_persistent_connections on\n")
	b.WriteString("persistent_request_timeout 60 seconds\n")
	b.WriteString("pconn_timeout 2 minutes\n")
	b.WriteString("half_closed_clients off\n")
	b.WriteString("read_timeout 5 minutes\n")
	b.WriteString("connect_timeout 10 seconds\n\n")

	// DNS cache
	b.WriteString("# DNS cache\n")
	b.WriteString("positive_dns_ttl 1 hours\n")
	b.WriteString("negative_dns_ttl 30 seconds\n\n")

	// File descriptors
	b.WriteString("# File descriptors\n")
	b.WriteString("max_filedescriptors 65536\n\n")

	// Request deduplication
	b.WriteString("# Request deduplication\n")
	b.WriteString("collapsed_forwarding on\n")

	return b.String()
}

// getPublicIP fetches the client's public IP with validation.
// Uses a Go HTTP client with environment proxy disabled to avoid loops.
func getPublicIP() (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy: nil, // bypass environment proxy to avoid loops
		},
	}
	resp, err := client.Get("https://ipinfo.io/ip")
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	ip := strings.TrimSpace(string(body))
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("invalid IP in response: %q", ip)
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
		return "", fmt.Errorf("non-public IP in response: %s", ip)
	}

	return ip, nil
}

func sshRun(cfg *config.Config, cmd string) error {
	args := cfg.Proxy.SSHBaseArgs()
	args = append(args, cfg.Proxy.SSHTarget(), cmd)

	out, err := exec.Command("ssh", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func sshRunOutput(cfg *config.Config, cmd string) (string, error) {
	args := cfg.Proxy.SSHBaseArgs()
	args = append(args, cfg.Proxy.SSHTarget(), cmd)

	out, err := exec.Command("ssh", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// FetchRecentLogs fetches the last N lines of Squid access log via SSH.
func FetchRecentLogs(cfg *config.Config, lines int) (string, error) {
	return sshRunOutput(cfg, fmt.Sprintf("tail -%d /var/log/squid/access.log 2>/dev/null", lines))
}
