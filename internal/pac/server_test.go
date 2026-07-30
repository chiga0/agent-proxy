package pac

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
)

// setTestHome redirects both HOME and USERPROFILE so os.UserHomeDir()
// returns tmpDir on all platforms (Unix uses HOME, Windows uses USERPROFILE).
func setTestHome(t *testing.T, tmpDir string) {
	t.Helper()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
}

func TestGenerateNonceAndReadNonce(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	nonce, err := generateNonce()
	if err != nil {
		t.Fatalf("generateNonce: %v", err)
	}
	if len(nonce) != 32 { // 16 bytes → 32 hex chars
		t.Errorf("expected 32-char hex nonce, got %d chars: %q", len(nonce), nonce)
	}

	// readNonce should return the same value
	got := readNonce()
	if got != nonce {
		t.Errorf("readNonce = %q, want %q", got, nonce)
	}

	// Nonce file should exist on disk with correct permissions
	info, err := os.Stat(noncePath())
	if err != nil {
		t.Fatalf("nonce file should exist: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Errorf("nonce file permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestReadNonceMissing(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// No nonce file yet
	if got := readNonce(); got != "" {
		t.Errorf("readNonce with no file = %q, want empty", got)
	}
}

func TestNoncePath(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	expected := filepath.Join(tmpDir, ".config", "agent-proxy", "pac-nonce")
	if got := noncePath(); got != expected {
		t.Errorf("noncePath() = %q, want %q", got, expected)
	}
}

func TestPacHandlerSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Write a PAC file to disk
	pacContent := "function FindProxyForURL(url, host) { return \"DIRECT\"; }"
	if err := os.MkdirAll(config.DataDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.PACPath(), []byte(pacContent), 0644); err != nil {
		t.Fatal(err)
	}

	nonce := "test-nonce-abc123"
	handler := pacHandler(nonce)

	req := httptest.NewRequest(http.MethodGet, "/proxy.pac", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify headers
	if ct := resp.Header.Get("Content-Type"); ct != "application/x-ns-proxy-autoconfig" {
		t.Errorf("Content-Type = %q, want application/x-ns-proxy-autoconfig", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	if xn := resp.Header.Get("X-Agent-Proxy"); xn != nonce {
		t.Errorf("X-Agent-Proxy = %q, want %q", xn, nonce)
	}

	// Verify body
	if body := w.Body.String(); body != pacContent {
		t.Errorf("body = %q, want %q", body, pacContent)
	}
}

func TestPacHandlerMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// No PAC file on disk
	handler := pacHandler("some-nonce")

	req := httptest.NewRequest(http.MethodGet, "/proxy.pac", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing PAC file, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "PAC not found") {
		t.Errorf("expected 'PAC not found' error message, got %q", w.Body.String())
	}
}

func TestPacHandlerIncrementsCounter(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Write a PAC file
	if err := os.MkdirAll(config.DataDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.PACPath(), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	before := pacRequestsTotal.Load()
	handler := pacHandler("nonce")

	req := httptest.NewRequest(http.MethodGet, "/proxy.pac", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	after := pacRequestsTotal.Load()
	if after != before+1 {
		t.Errorf("pacRequestsTotal: before=%d, after=%d, want increment by 1", before, after)
	}
}

func TestServerRunningNoNonce(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// No nonce file → ServerRunning should be false
	if ServerRunning() {
		t.Error("ServerRunning should be false when no nonce file exists")
	}
}

func TestPortOccupiedNoServer(t *testing.T) {
	// Use a port that's very unlikely to be occupied
	// PortOccupied checks config.PACPort (18080). In test env, nothing should be there.
	// This is a best-effort test — if something IS on 18080, skip.
	if PortOccupied() {
		t.Skip("port 18080 is occupied, skipping")
	}
	// If we get here, port is free — that's the expected case
}

func TestStartStopServer(t *testing.T) {
	// Skip if PAC port is already occupied (e.g., agent-proxy running locally)
	if PortOccupied() {
		t.Skipf("port %d already occupied, skipping StartServer test", config.PACPort)
	}

	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Write a PAC file so the handler can serve it
	cfg := &config.Config{
		Proxy:         config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		CustomDomains: []string{"example.com"},
	}
	if err := Write(cfg); err != nil {
		t.Fatalf("Write PAC: %v", err)
	}

	if err := StartServer(); err != nil {
		t.Fatalf("StartServer: %v", err)
	}
	defer StopServer()

	// Server should now be running
	if !ServerRunning() {
		t.Error("ServerRunning should be true after StartServer")
	}

	// Calling StartServer again should be idempotent (no error)
	if err := StartServer(); err != nil {
		t.Errorf("second StartServer should be no-op, got: %v", err)
	}

	// Verify PAC endpoint works over HTTP
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
	}
	resp, err := client.Get("http://127.0.0.1:18080/proxy.pac")
	if err != nil {
		t.Fatalf("GET /proxy.pac: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 from /proxy.pac, got %d", resp.StatusCode)
	}

	// Verify dashboard endpoint
	resp2, err := client.Get("http://127.0.0.1:18080/dashboard")
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("expected 200 from /dashboard, got %d", resp2.StatusCode)
	}

	// Verify metrics endpoint
	resp3, err := client.Get("http://127.0.0.1:18080/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != 200 {
		t.Errorf("expected 200 from /metrics, got %d", resp3.StatusCode)
	}

	// Stop the server
	StopServer()

	// Nonce file should be removed
	if got := readNonce(); got != "" {
		t.Errorf("nonce should be removed after StopServer, got %q", got)
	}

	// Calling StopServer again should be safe
	StopServer()
}

func TestStopServerWithoutStart(t *testing.T) {
	// Should not panic
	StopServer()
}

func TestWatchConfigAndReloadPAC(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Write initial config + PAC
	cfg := &config.Config{
		Proxy:         config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		CustomDomains: []string{"initial.example.com"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	if err := Write(cfg); err != nil {
		t.Fatalf("Write PAC: %v", err)
	}

	// Verify initial PAC content
	data, err := os.ReadFile(config.PACPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "initial.example.com") {
		t.Fatal("initial PAC should contain initial.example.com")
	}

	// Start the watcher (infinite loop in goroutine — will be cleaned up on test exit)
	go watchConfigAndReloadPAC()

	// Modify the config: add a new domain
	cfg.CustomDomains = append(cfg.CustomDomains, "updated.example.com")
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save updated config: %v", err)
	}

	// The watcher polls every 5 seconds; wait up to 8 seconds for it to pick up
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		data, err = os.ReadFile(config.PACPath())
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "updated.example.com") {
			// Success — PAC was regenerated with the new domain
			return
		}
	}
	t.Error("PAC was not regenerated within 8 seconds after config change")
}
