package proxy

import (
	"os"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestCheckResultFields(t *testing.T) {
	r := CheckResult{Name: "test", OK: true, Detail: "ok"}
	if r.Name != "test" || !r.OK || r.Detail != "ok" {
		t.Errorf("unexpected CheckResult: %+v", r)
	}
}

func TestPrintStatus(t *testing.T) {
	results := []CheckResult{
		{Name: "check1", OK: true, Detail: "pass"},
		{Name: "check2", OK: false, Detail: "fail"},
	}
	// Just verify it doesn't panic
	PrintStatus(results)
}

func TestPrintStatusEmpty(t *testing.T) {
	// Should print "0 passed, 0 failed" without panic
	PrintStatus(nil)
	PrintStatus([]CheckResult{})
}

func TestPrintStatusAllPass(t *testing.T) {
	results := []CheckResult{
		{Name: "a", OK: true, Detail: ""},
		{Name: "b", OK: true, Detail: "info"},
	}
	PrintStatus(results)
}

func TestPrintStatusNoDetail(t *testing.T) {
	// When Detail is empty, no parentheses should be printed
	results := []CheckResult{
		{Name: "noDetail", OK: true, Detail: ""},
	}
	PrintStatus(results)
}

// --- Status() structure ---

func TestStatusNonTunnelMode(t *testing.T) {
	setupTestHome(t)
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:   "127.0.0.1",
			Port:   19876, // unlikely to be open; connection refused is fast on localhost
			Tunnel: false,
		},
		Presets: []string{"ai"},
	}

	results := Status(cfg)

	// tunnel=false: SSH(22), Proxy port, Forwarding, System PAC, PAC file, PAC HTTP server
	if len(results) != 6 {
		t.Fatalf("got %d results, want 6: %+v", len(results), results)
	}

	expectedNames := []string{
		"SSH (22)",
		"Proxy port (19876)",
		"Proxy forwarding",
		"System PAC",
		"PAC file",
		"PAC HTTP server",
	}
	for i, name := range expectedNames {
		if results[i].Name != name {
			t.Errorf("results[%d].Name = %q, want %q", i, results[i].Name, name)
		}
	}
}

func TestStatusTunnelMode(t *testing.T) {
	setupTestHome(t)
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:            "127.0.0.1",
			Port:            19876,
			Tunnel:          true,
			TunnelLocalPort: 19877,
		},
		Presets: []string{"ai"},
	}

	results := Status(cfg)

	// tunnel=true: SSH(22) via tunnel, SSH tunnel, Forwarding, System PAC, PAC file, PAC HTTP server
	if len(results) != 6 {
		t.Fatalf("got %d results, want 6: %+v", len(results), results)
	}

	if results[0].Name != "SSH (22)" {
		t.Errorf("results[0].Name = %q, want 'SSH (22)'", results[0].Name)
	}
	if !results[0].OK {
		t.Error("SSH (22) should be OK in tunnel mode (skipped)")
	}
	if results[0].Detail != "via tunnel — direct check skipped" {
		t.Errorf("results[0].Detail = %q", results[0].Detail)
	}
	if results[1].Name != "SSH tunnel" {
		t.Errorf("results[1].Name = %q, want 'SSH tunnel'", results[1].Name)
	}
	// Tunnel is not actually running in test
	if results[1].OK {
		t.Error("SSH tunnel should not be OK in test (no tunnel running)")
	}
}

// --- checkPACFile ---

func TestCheckPACFileExists(t *testing.T) {
	setupTestHome(t)
	os.MkdirAll(config.DataDir(), 0700)

	content := `if (dnsDomainIs(host, ".example.com")) return "PROXY x";
if (dnsDomainIs(host, ".test.com")) return "PROXY x";
if (dnsDomainIs(host, ".foo.org")) return "PROXY x";`
	os.WriteFile(config.PACPath(), []byte(content), 0644)

	cfg := &config.Config{}
	result := checkPACFile(cfg)
	if !result.OK {
		t.Errorf("checkPACFile should succeed: %+v", result)
	}
	if result.Detail != "3 domains" {
		t.Errorf("Detail = %q, want '3 domains'", result.Detail)
	}
}

func TestCheckPACFileMissing(t *testing.T) {
	setupTestHome(t)

	cfg := &config.Config{}
	result := checkPACFile(cfg)
	if result.OK {
		t.Error("checkPACFile should fail when PAC file is missing")
	}
	if result.Detail != "not found" {
		t.Errorf("Detail = %q, want 'not found'", result.Detail)
	}
}

func TestCheckPACFileEmpty(t *testing.T) {
	setupTestHome(t)
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(config.PACPath(), []byte(""), 0644)

	cfg := &config.Config{}
	result := checkPACFile(cfg)
	if !result.OK {
		t.Error("checkPACFile should succeed for empty file (0 domains)")
	}
	if result.Detail != "0 domains" {
		t.Errorf("Detail = %q, want '0 domains'", result.Detail)
	}
}

// --- checkPACServer ---

func TestCheckPACServerNotRunning(t *testing.T) {
	setupTestHome(t)

	result := checkPACServer()
	if result.Name != "PAC HTTP server" {
		t.Errorf("Name = %q", result.Name)
	}
	if result.OK {
		t.Error("PAC server should not be running in test")
	}
	if result.Detail != "not running — health auto-recovery inactive" {
		t.Errorf("Detail = %q, want 'not running — health auto-recovery inactive'", result.Detail)
	}
}

// --- DetectSNIBlock ---

func TestDetectSNIBlockTunnelMode(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:   "127.0.0.1",
			Port:   19876,
			Tunnel: true,
		},
	}
	// When tunnel is enabled, SNI block detection is skipped → always false
	if DetectSNIBlock(cfg) {
		t.Error("DetectSNIBlock should return false when tunnel is enabled")
	}
}

func TestDetectSNIBlockUnreachableProxy(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:   "127.0.0.1",
			Port:   19876, // nothing listening → dial fails → returns false
			Tunnel: false,
		},
	}
	if DetectSNIBlock(cfg) {
		t.Error("DetectSNIBlock should return false when proxy is unreachable")
	}
}

// --- checkTunnel ---

func TestCheckTunnelNotRunning(t *testing.T) {
	setupTestHome(t)
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:            "127.0.0.1",
			Port:            19876,
			Tunnel:          true,
			TunnelLocalPort: 19877,
		},
	}

	result := checkTunnel(cfg)
	if result.Name != "SSH tunnel" {
		t.Errorf("Name = %q", result.Name)
	}
	if result.OK {
		t.Error("tunnel should not be running in test")
	}
	// Should report "not running" since no control socket and port is free
	if result.Detail != "not running — run: agent-proxy on" {
		t.Errorf("Detail = %q, want 'not running — run: agent-proxy on'", result.Detail)
	}
}
