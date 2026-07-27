package pac

import (
	"os"
	"strings"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestGenerate(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host: "1.2.3.4",
			Port: 18443,
		},
		Whitelist: []string{"chatgpt.com", "openai.com"},
	}

	pac, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Should contain function
	if !strings.Contains(pac, "function FindProxyForURL(url, host)") {
		t.Error("PAC missing function declaration")
	}

	// Should contain both domains
	if !strings.Contains(pac, `"chatgpt.com"`) {
		t.Error("PAC missing chatgpt.com")
	}
	if !strings.Contains(pac, `"openai.com"`) {
		t.Error("PAC missing openai.com")
	}

	// Should contain proxy address
	if !strings.Contains(pac, "PROXY 1.2.3.4:18443") {
		t.Error("PAC missing proxy address")
	}

	// Should end with DIRECT
	if !strings.Contains(pac, `return "DIRECT"`) {
		t.Error("PAC missing DIRECT fallback")
	}

	// Should contain subdomain matching
	if !strings.Contains(pac, `dnsDomainIs(host, ".chatgpt.com")`) {
		t.Error("PAC missing subdomain match for chatgpt.com")
	}
}

func TestGenerateEmpty(t *testing.T) {
	cfg := &config.Config{
		Whitelist: []string{},
	}
	_, err := Generate(cfg)
	if err == nil {
		t.Error("Generate should fail with empty whitelist")
	}
}

func TestWrite(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &config.Config{
		Proxy:     config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		Whitelist: []string{"example.com"},
	}

	if err := Write(cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Verify file exists at expected path
	path := config.PACPath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PAC file: %v", err)
	}
	if !strings.Contains(string(data), "example.com") {
		t.Error("written PAC missing example.com")
	}
}
