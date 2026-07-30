package pac

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestDashboardReturnsHTML(t *testing.T) {
	mux := http.NewServeMux()
	registerDashboard(mux)

	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %q", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("dashboard HTML missing DOCTYPE")
	}
	if !strings.Contains(body, "Agent Proxy Dashboard") {
		t.Error("dashboard HTML missing title")
	}
	if !strings.Contains(body, "/api/status") {
		t.Error("dashboard HTML missing /api/status fetch")
	}
	if !strings.Contains(body, "/api/stats") {
		t.Error("dashboard HTML missing /api/stats fetch")
	}
}

func TestDashboardCacheControl(t *testing.T) {
	mux := http.NewServeMux()
	registerDashboard(mux)

	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if cc := w.Result().Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
}

func TestDashboardHTMLContainsExpectedElements(t *testing.T) {
	mux := http.NewServeMux()
	registerDashboard(mux)

	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	body := w.Body.String()

	expectedElements := []struct {
		name    string
		pattern string
	}{
		{"status grid", `id="status-grid"`},
		{"whitelist grid", `id="wl-grid"`},
		{"traffic table body", `id="traffic-body"`},
		{"traffic table header", "<th>Domain</th>"},
		{"requests column", "<th"},
		{"JavaScript fetch status", "fetch('/api/status')"},
		{"JavaScript fetch stats", "fetch('/api/stats')"},
		{"formatBytes function", "function formatBytes"},
		{"viewport meta", `name="viewport"`},
		{"charset meta", `charset="utf-8"`},
		{"CSS styling", "<style>"},
		{"error handling", "error"},
	}

	for _, el := range expectedElements {
		if !strings.Contains(body, el.pattern) {
			t.Errorf("dashboard HTML missing %s (%q)", el.name, el.pattern)
		}
	}
}

func TestAPIStatusReturnsValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Create a valid config so /api/status doesn't 500 on validation
	cfg := config.DefaultConfig()
	cfg.Proxy.Host = "1.2.3.4"
	cfg.Proxy.Tunnel = false // avoid ssh_key validation error
	cfg.Save()

	mux := http.NewServeMux()
	registerDashboard(mux)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("expected application/json content type, got %q", ct)
	}

	var status statusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	// Verify expected fields exist (port should be non-zero from default config)
	if status.Port == 0 {
		t.Error("expected non-zero port in status response")
	}
	if status.Presets == nil {
		t.Error("expected presets to be non-nil")
	}
}

func TestAPIStatusWithCustomConfig(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Write a specific config
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:         "proxy.test.com",
			Port:         9999,
			Tunnel:       true,
			FallbackHost: "fallback.test.com",
		},
		Presets:       []string{"ai"},
		CustomDomains: []string{"custom.example.com"},
		NoProxy:       []string{"localhost", "10.*"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	mux := http.NewServeMux()
	registerDashboard(mux)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var status statusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if status.Host != "proxy.test.com" {
		t.Errorf("Host = %q, want proxy.test.com", status.Host)
	}
	if status.Port != 9999 {
		t.Errorf("Port = %d, want 9999", status.Port)
	}
	if !status.Tunnel {
		t.Error("Tunnel should be true")
	}
	if status.FallbackHost != "fallback.test.com" {
		t.Errorf("FallbackHost = %q, want fallback.test.com", status.FallbackHost)
	}
	if len(status.Presets) != 1 || status.Presets[0] != "ai" {
		t.Errorf("Presets = %v, want [ai]", status.Presets)
	}
	// ai preset has 18 domains + 1 custom = 19
	if status.WhitelistCount < 2 {
		t.Errorf("WhitelistCount = %d, expected at least 2", status.WhitelistCount)
	}
	if status.NoProxyCount != 2 {
		t.Errorf("NoProxyCount = %d, want 2", status.NoProxyCount)
	}
}

func TestAPIStatusConfigLoadError(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Write an invalid config file
	configDir := filepath.Join(tmpDir, ".config", "agent-proxy")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(":::invalid yaml{{{"), 0600); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerDashboard(mux)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for invalid config, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "failed to load config") {
		t.Errorf("expected error message, got %q", w.Body.String())
	}
}

func TestAPIStatsReturnsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Write a valid config (stats handler needs config.Load to succeed)
	cfg := &config.Config{
		Proxy:   config.ProxyConfig{Host: "1.2.3.4", Port: 18443},
		Presets: []string{"ai"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	mux := http.NewServeMux()
	registerDashboard(mux)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Result().Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("expected application/json, got %q", ct)
	}

	var stats statsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Without a real SSH server, FetchRecentLogs will fail → error field set
	// OR if it somehow succeeds, domains will be empty. Either is valid.
	// The key assertion: response is valid JSON with the right structure.
	if stats.Error == "" && stats.Domains == nil {
		t.Error("expected either an error or a non-nil domains list")
	}
}

func TestAPIStatsConfigLoadError(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Write an invalid config file
	configDir := filepath.Join(tmpDir, ".config", "agent-proxy")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(":::bad{{"), 0600); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerDashboard(mux)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// apiStatsHandler returns 200 with error in JSON body for config errors
	var stats statsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if stats.Error != "failed to load config" {
		t.Errorf("Error = %q, want 'failed to load config'", stats.Error)
	}
}

func TestRegisterDashboardRoutes(t *testing.T) {
	mux := http.NewServeMux()
	registerDashboard(mux)

	routes := []string{"/dashboard", "/api/status", "/api/stats"}
	for _, route := range routes {
		req := httptest.NewRequest("GET", route, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Errorf("route %s returned 404, expected it to be registered", route)
		}
	}
}
