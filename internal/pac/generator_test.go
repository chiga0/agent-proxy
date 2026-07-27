package pac

import (
	"os"
	"strings"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestGenerate(t *testing.T) {
	cfg := &config.Config{
		Proxy:   config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		Presets: []string{"ai"},
	}
	pac, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(pac, "function FindProxyForURL") {
		t.Error("missing function")
	}
	if !strings.Contains(pac, `"chatgpt.com"`) {
		t.Error("missing chatgpt.com")
	}
	if !strings.Contains(pac, "PROXY 1.2.3.4:18443") {
		t.Error("missing proxy addr")
	}
	if !strings.Contains(pac, `return "DIRECT"`) {
		t.Error("missing DIRECT")
	}
}

func TestGenerateEmpty(t *testing.T) {
	cfg := &config.Config{}
	if _, err := Generate(cfg); err == nil {
		t.Error("should fail with empty config")
	}
}

func TestGenerateCustomOnly(t *testing.T) {
	cfg := &config.Config{
		Proxy:         config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		CustomDomains: []string{"mysite.com"},
	}
	pac, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(pac, `"mysite.com"`) {
		t.Error("missing custom domain")
	}
}

func TestWrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	cfg := &config.Config{
		Proxy:         config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		CustomDomains: []string{"example.com"},
	}
	if err := Write(cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, _ := os.ReadFile(config.PACPath())
	if !strings.Contains(string(data), "example.com") {
		t.Error("PAC missing example.com")
	}
}
