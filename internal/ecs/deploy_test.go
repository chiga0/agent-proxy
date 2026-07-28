package ecs

import (
	"strings"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestGenerateSquidConfigTunnelMode(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{Port: 18443, Tunnel: true},
	}
	conf := generateSquidConfig(cfg, "")

	// Tunnel mode: loopback-only listen
	if !strings.Contains(conf, "http_port 127.0.0.1:18443") {
		t.Error("tunnel mode should listen on 127.0.0.1 only")
	}
	if strings.Contains(conf, "http_port 18443\n") {
		t.Error("tunnel mode should NOT listen on all interfaces")
	}

	// Only 127.0.0.1 trusted, no public IP
	if !strings.Contains(conf, "acl trusted_ip src 127.0.0.1\n") {
		t.Error("tunnel mode should trust only 127.0.0.1")
	}

	// Deny-first ACL order
	denySafe := strings.Index(conf, "http_access deny !Safe_ports")
	denyConnect := strings.Index(conf, "http_access deny CONNECT !SSL_ports")
	denyLocal := strings.Index(conf, "http_access deny to_localhost")
	allowTrusted := strings.Index(conf, "http_access allow trusted_ip")
	denyAll := strings.Index(conf, "http_access deny all")

	if denySafe < 0 || denyConnect < 0 || denyLocal < 0 || allowTrusted < 0 || denyAll < 0 {
		t.Fatal("missing required ACL rules")
	}
	if !(denySafe < denyConnect && denyConnect < denyLocal && denyLocal < allowTrusted && allowTrusted < denyAll) {
		t.Error("ACL rules must be in deny-first order: deny !Safe_ports → deny CONNECT !SSL_ports → deny to_localhost → allow trusted_ip → deny all")
	}

	// No auth
	if strings.Contains(conf, "auth_param") {
		t.Error("should not contain auth_param")
	}

	// Private/metadata destination blocks
	for _, acl := range []string{"to_localhost", "to_linklocal", "to_rfc1918", "to_metadata"} {
		if !strings.Contains(conf, "http_access deny "+acl) {
			t.Errorf("should deny %s", acl)
		}
	}

	// Alibaba Cloud metadata address
	if !strings.Contains(conf, "100.100.100.200") {
		t.Error("should block Alibaba Cloud metadata 100.100.100.200")
	}

	// IPv6 coverage
	if !strings.Contains(conf, "::1") {
		t.Error("should block IPv6 loopback ::1")
	}
	if !strings.Contains(conf, "fe80::/10") {
		t.Error("should block IPv6 link-local fe80::/10")
	}
	if !strings.Contains(conf, "fc00::/7") {
		t.Error("should block IPv6 private fc00::/7")
	}
}

func TestGenerateSquidConfigDirectMode(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{Port: 18443, Tunnel: false},
	}
	conf := generateSquidConfig(cfg, "203.0.113.1")

	// Direct mode: listen on all interfaces
	if !strings.Contains(conf, "http_port 18443\n") {
		t.Error("direct mode should listen on all interfaces")
	}

	// Trusted includes public IP
	if !strings.Contains(conf, "acl trusted_ip src 127.0.0.1 203.0.113.1") {
		t.Error("direct mode should trust 127.0.0.1 and public IP")
	}

	// Still has deny-first ACL
	if !strings.Contains(conf, "http_access deny !Safe_ports") {
		t.Error("direct mode should still have deny-first ACL")
	}
}

func TestGenerateSquidConfigNoTrustedIP(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{Port: 18443},
	}
	conf := generateSquidConfig(cfg, "")

	if !strings.Contains(conf, "acl trusted_ip src 127.0.0.1\n") {
		t.Error("should trust 127.0.0.1 even without public IP")
	}
}

func TestGenerateSquidConfigSafePorts(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{Port: 18443},
	}
	conf := generateSquidConfig(cfg, "")

	if !strings.Contains(conf, "acl Safe_ports port 80 443 8443") {
		t.Error("should define Safe_ports")
	}
	if !strings.Contains(conf, "acl SSL_ports port 443 8443") {
		t.Error("should define SSL_ports")
	}
}

func TestGenerateSquidConfigPrivacy(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{Port: 18443},
	}
	conf := generateSquidConfig(cfg, "")

	if !strings.Contains(conf, "forwarded_for off") {
		t.Error("should disable forwarded_for")
	}
	if !strings.Contains(conf, "request_header_access Via deny all") {
		t.Error("should deny Via header")
	}
}
