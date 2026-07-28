package ecs

import (
	"strings"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestGenerateSquidConfigNoAuth(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{Port: 18443},
	}
	conf := generateSquidConfig(cfg, "1.2.3.4")

	if !strings.Contains(conf, "http_port 18443") {
		t.Error("should contain http_port")
	}
	if !strings.Contains(conf, "acl trusted_ip src 127.0.0.1 1.2.3.4") {
		t.Error("should trust 127.0.0.1 and user IP")
	}
	if strings.Contains(conf, "auth_param") {
		t.Error("should not contain auth_param without credentials")
	}
	if strings.Contains(conf, "authenticated") {
		t.Error("should not reference authenticated ACL without credentials")
	}
}

func TestGenerateSquidConfigWithAuth(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{Port: 18443, User: "testuser", Password: "testpass"},
	}
	conf := generateSquidConfig(cfg, "1.2.3.4")

	if !strings.Contains(conf, "auth_param basic program") {
		t.Error("should contain auth_param with credentials")
	}
	if !strings.Contains(conf, "acl authenticated proxy_auth REQUIRED") {
		t.Error("should define authenticated ACL")
	}
	if !strings.Contains(conf, "http_access allow authenticated") {
		t.Error("should allow authenticated users")
	}
	if !strings.Contains(conf, "127.0.0.1") {
		t.Error("should always trust 127.0.0.1")
	}
}

func TestGenerateSquidConfigNoTrustedIP(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{Port: 18443},
	}
	conf := generateSquidConfig(cfg, "")

	if !strings.Contains(conf, "acl trusted_ip src 127.0.0.1") {
		t.Error("should trust 127.0.0.1 even without public IP")
	}
}
