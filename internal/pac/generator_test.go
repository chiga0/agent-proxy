package pac

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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
	setTestHome(t, tmpDir)
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

func TestGenerateTunnelMode(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:   "remote.example.com",
			Port:   18443,
			Tunnel: true,
		},
		Presets: []string{"ai"},
	}
	pac, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// In tunnel mode, EffectiveHost() returns ::1 (IPv6 loopback)
	if !strings.Contains(pac, "PROXY [::1]:18443") {
		t.Errorf("tunnel mode should use [::1], got:\n%s", pac)
	}
	if strings.Contains(pac, "remote.example.com") {
		t.Error("tunnel mode should not contain the remote host in PROXY directive")
	}
}

func TestGenerateTunnelCustomLocalPort(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:            "remote.example.com",
			Port:            18443,
			Tunnel:          true,
			TunnelLocalPort: 9999,
		},
		CustomDomains: []string{"test.com"},
	}
	pac, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(pac, "PROXY [::1]:9999") {
		t.Errorf("tunnel with custom local port should use [::1]:9999, got:\n%s", pac)
	}
}

func TestGenerateLargeWhitelist(t *testing.T) {
	domains := make([]string, 1000)
	for i := range domains {
		domains[i] = fmt.Sprintf("domain%04d.example.com", i)
	}
	cfg := &config.Config{
		Proxy:         config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		CustomDomains: domains,
	}

	start := time.Now()
	pac, err := Generate(cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("Generate took too long for 1000 domains: %v", elapsed)
	}
	// Verify first and last domains are present
	if !strings.Contains(pac, "domain0000.example.com") {
		t.Error("missing first domain")
	}
	if !strings.Contains(pac, "domain0999.example.com") {
		t.Error("missing last domain")
	}
	// Verify structure is intact
	if !strings.Contains(pac, `return "DIRECT"`) {
		t.Error("missing DIRECT fallback")
	}
	if !strings.HasSuffix(strings.TrimSpace(pac), "}") {
		t.Error("PAC should end with closing brace")
	}
}

func TestGenerateMultiplePresets(t *testing.T) {
	cfg := &config.Config{
		Proxy:   config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		Presets: []string{"ai", "dev", "media"},
	}
	pac, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Check domains from each preset
	for _, domain := range []string{"chatgpt.com", "github.com", "youtube.com"} {
		if !strings.Contains(pac, `"`+domain+`"`) {
			t.Errorf("missing domain %q from presets", domain)
		}
	}
}

func TestGenerateDeduplicatesDomains(t *testing.T) {
	// "chatgpt.com" is in the "ai" preset; adding it as custom should not duplicate
	cfg := &config.Config{
		Proxy:         config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		Presets:       []string{"ai"},
		CustomDomains: []string{"chatgpt.com", "unique.example.com"},
	}
	pac, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// "chatgpt.com" (with quotes) appears once per rule in `host == "chatgpt.com"`.
	// The dnsDomainIs uses ".chatgpt.com" (dot-prefixed), so `"chatgpt.com"` matches only once.
	count := strings.Count(pac, `"chatgpt.com"`)
	if count != 1 {
		t.Errorf("chatgpt.com should appear exactly 1 time (deduped), got %d", count)
	}
	if !strings.Contains(pac, `"unique.example.com"`) {
		t.Error("missing unique custom domain")
	}
}

func TestGenerateNoProxyNotInPAC(t *testing.T) {
	cfg := &config.Config{
		Proxy:         config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		CustomDomains: []string{"example.com"},
		NoProxy:       []string{"localhost", "127.0.0.1", "10.*"},
	}
	pac, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// no_proxy entries should NOT appear as PROXY rules in the PAC
	if strings.Contains(pac, `"localhost"`) {
		t.Error("no_proxy entry 'localhost' should not appear in PAC")
	}
	if strings.Contains(pac, `"127.0.0.1"`) {
		t.Error("no_proxy entry '127.0.0.1' should not appear in PAC")
	}
}

func TestGeneratePACStructure(t *testing.T) {
	cfg := &config.Config{
		Proxy:         config.ProxyConfig{Host: "10.0.0.1", Port: 8080},
		CustomDomains: []string{"test.org"},
	}
	pac, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	lines := strings.Split(pac, "\n")
	// First two lines are comments
	if !strings.HasPrefix(lines[0], "// Auto-generated by agent-proxy") {
		t.Error("first line should be auto-generated comment")
	}
	if !strings.HasPrefix(lines[1], "// Effective whitelist: 1 domains") {
		t.Errorf("second line should mention domain count, got: %q", lines[1])
	}
	// Third line is function declaration
	if lines[2] != "function FindProxyForURL(url, host) {" {
		t.Errorf("third line should be function declaration, got: %q", lines[2])
	}
	// Verify dnsDomainIs pattern
	if !strings.Contains(pac, `dnsDomainIs(host, ".test.org")`) {
		t.Error("missing dnsDomainIs rule")
	}
	if !strings.Contains(pac, `host == "test.org"`) {
		t.Error("missing exact host match rule")
	}
}

func TestGenerateIPv6Host(t *testing.T) {
	cfg := &config.Config{
		Proxy:         config.ProxyConfig{Host: "::1", Port: 18443},
		CustomDomains: []string{"example.com"},
	}
	pac, err := Generate(cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// net.JoinHostPort wraps IPv6 in brackets
	if !strings.Contains(pac, "PROXY [::1]:18443") {
		t.Errorf("IPv6 host should be bracketed, got:\n%s", pac)
	}
}

func TestWriteCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	cfg := &config.Config{
		Proxy:         config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		CustomDomains: []string{"mkdir-test.com"},
	}
	// DataDir should not exist yet
	if err := Write(cfg); err != nil {
		t.Fatalf("Write should create directory: %v", err)
	}
	if _, err := os.Stat(config.PACPath()); err != nil {
		t.Fatalf("PAC file should exist: %v", err)
	}
}

func TestWriteEmptyWhitelistFails(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
	}
	if err := Write(cfg); err == nil {
		t.Error("Write should fail with empty whitelist")
	}
}
