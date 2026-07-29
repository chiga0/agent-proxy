package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

// setupTestHome redirects HOME to a temp dir so config.DataDir() is isolated.
func setupTestHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestIsOurPAC(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://127.0.0.1:18080/proxy.pac", true},
		{"http://127.0.0.1:9999/proxy.pac", false},
		{"http://evil.com/proxy.pac", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isOurPAC(tt.url); got != tt.want {
			t.Errorf("isOurPAC(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestAgentProxyPACURL(t *testing.T) {
	url := agentProxyPACURL()
	want := "http://127.0.0.1:18080/proxy.pac"
	if url != want {
		t.Errorf("agentProxyPACURL() = %q, want %q", url, want)
	}
}

func TestPacStatePath(t *testing.T) {
	setupTestHome(t)
	path := pacStatePath()
	if path == "" {
		t.Fatal("pacStatePath() returned empty")
	}
	if filepath.Base(path) != "pac-state.json" {
		t.Errorf("pacStatePath() base = %q, want pac-state.json", filepath.Base(path))
	}
}

func TestPACStateFileSaveLoadRoundTrip(t *testing.T) {
	setupTestHome(t)

	original := pacStateFile{
		"Wi-Fi": {
			OriginalURL: "http://old-proxy.example.com/proxy.pac",
			WasEnabled:  true,
			Extra:       map[string]string{"mode": "auto"},
		},
		"Ethernet": {
			OriginalURL: "",
			WasEnabled:  false,
		},
	}

	if err := savePACStateFile(original); err != nil {
		t.Fatalf("savePACStateFile() error: %v", err)
	}

	loaded, err := loadPACStateFile()
	if err != nil {
		t.Fatalf("loadPACStateFile() error: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("loaded %d services, want 2", len(loaded))
	}

	wifi := loaded["Wi-Fi"]
	if wifi.OriginalURL != "http://old-proxy.example.com/proxy.pac" {
		t.Errorf("Wi-Fi.OriginalURL = %q", wifi.OriginalURL)
	}
	if !wifi.WasEnabled {
		t.Error("Wi-Fi.WasEnabled = false, want true")
	}
	if wifi.Extra["mode"] != "auto" {
		t.Errorf("Wi-Fi.Extra[mode] = %q, want auto", wifi.Extra["mode"])
	}

	eth := loaded["Ethernet"]
	if eth.OriginalURL != "" {
		t.Errorf("Ethernet.OriginalURL = %q, want empty", eth.OriginalURL)
	}
	if eth.WasEnabled {
		t.Error("Ethernet.WasEnabled = true, want false")
	}
}

func TestPACStateFileCrossServiceSnapshots(t *testing.T) {
	setupTestHome(t)

	// Simulate saving state for multiple services independently
	state := make(pacStateFile)
	state["Wi-Fi"] = pacSnapshot{OriginalURL: "http://wifi.pac", WasEnabled: true}
	if err := savePACStateFile(state); err != nil {
		t.Fatalf("save Wi-Fi: %v", err)
	}

	// Load, add another service, save again
	loaded, err := loadPACStateFile()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loaded["Ethernet"] = pacSnapshot{OriginalURL: "http://eth.pac", WasEnabled: false}
	if err := savePACStateFile(loaded); err != nil {
		t.Fatalf("save Ethernet: %v", err)
	}

	// Verify both services persisted
	final, err := loadPACStateFile()
	if err != nil {
		t.Fatalf("final load: %v", err)
	}
	if len(final) != 2 {
		t.Fatalf("got %d services, want 2", len(final))
	}
	if final["Wi-Fi"].OriginalURL != "http://wifi.pac" {
		t.Errorf("Wi-Fi URL = %q", final["Wi-Fi"].OriginalURL)
	}
	if final["Ethernet"].OriginalURL != "http://eth.pac" {
		t.Errorf("Ethernet URL = %q", final["Ethernet"].OriginalURL)
	}
}

func TestLoadPACStateFileCorrupted(t *testing.T) {
	setupTestHome(t)

	// Write invalid JSON
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(pacStatePath(), []byte("{invalid json!!!"), 0600)

	_, err := loadPACStateFile()
	if err == nil {
		t.Fatal("expected error for corrupted JSON")
	}
	// Should mention corruption
	if got := err.Error(); !contains(got, "corrupted") {
		t.Errorf("error = %q, want mention of 'corrupted'", got)
	}
}

func TestLoadPACStateFileLegacyFormat(t *testing.T) {
	setupTestHome(t)

	// Write legacy single-service format
	legacy := map[string]interface{}{
		"service":      "Wi-Fi",
		"original_url": "http://legacy.pac",
		"was_enabled":  true,
	}
	data, _ := json.Marshal(legacy)
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(pacStatePath(), data, 0600)

	loaded, err := loadPACStateFile()
	if err != nil {
		t.Fatalf("loadPACStateFile() legacy error: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("got %d services, want 1", len(loaded))
	}
	snap, ok := loaded["Wi-Fi"]
	if !ok {
		t.Fatal("missing Wi-Fi service in legacy migration")
	}
	if snap.OriginalURL != "http://legacy.pac" {
		t.Errorf("OriginalURL = %q, want http://legacy.pac", snap.OriginalURL)
	}
	if !snap.WasEnabled {
		t.Error("WasEnabled = false, want true")
	}
}

func TestLoadPACStateFileNotExist(t *testing.T) {
	setupTestHome(t)

	_, err := loadPACStateFile()
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected IsNotExist error, got: %v", err)
	}
}

func TestSavePACStateFileAtomicWrite(t *testing.T) {
	setupTestHome(t)

	state := pacStateFile{"svc": {OriginalURL: "http://x.pac"}}
	if err := savePACStateFile(state); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify no .tmp file remains (atomic rename completed)
	tmpPath := pacStatePath() + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error(".tmp file should not exist after successful save")
	}

	// Verify file permissions are 0600
	info, err := os.Stat(pacStatePath())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file perm = %o, want 0600", perm)
	}
}

func TestSavePACStateFileOverwrite(t *testing.T) {
	setupTestHome(t)

	// Save initial state
	state1 := pacStateFile{"svc1": {OriginalURL: "http://first.pac"}}
	if err := savePACStateFile(state1); err != nil {
		t.Fatalf("save1: %v", err)
	}

	// Overwrite with different state
	state2 := pacStateFile{"svc2": {OriginalURL: "http://second.pac"}}
	if err := savePACStateFile(state2); err != nil {
		t.Fatalf("save2: %v", err)
	}

	loaded, err := loadPACStateFile()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := loaded["svc1"]; ok {
		t.Error("svc1 should not exist after overwrite")
	}
	if loaded["svc2"].OriginalURL != "http://second.pac" {
		t.Errorf("svc2 URL = %q", loaded["svc2"].OriginalURL)
	}
}

func TestPACStateFileEmptyMap(t *testing.T) {
	setupTestHome(t)

	state := pacStateFile{}
	if err := savePACStateFile(state); err != nil {
		t.Fatalf("save empty: %v", err)
	}

	loaded, err := loadPACStateFile()
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("got %d entries, want 0", len(loaded))
	}
}

func TestPACSnapshotJSONRoundTrip(t *testing.T) {
	snap := pacSnapshot{
		OriginalURL: "http://test.pac",
		WasEnabled:  true,
		Extra:       map[string]string{"auto_detect": "1", "mode": "manual"},
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded pacSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.OriginalURL != snap.OriginalURL {
		t.Errorf("OriginalURL = %q, want %q", decoded.OriginalURL, snap.OriginalURL)
	}
	if decoded.WasEnabled != snap.WasEnabled {
		t.Errorf("WasEnabled = %v, want %v", decoded.WasEnabled, snap.WasEnabled)
	}
	if len(decoded.Extra) != 2 {
		t.Errorf("Extra len = %d, want 2", len(decoded.Extra))
	}
}

func TestPACSnapshotJSONOmitEmptyExtra(t *testing.T) {
	snap := pacSnapshot{OriginalURL: "http://x.pac", WasEnabled: false}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Extra should be omitted when nil
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if _, ok := m["extra"]; ok {
		t.Error("extra should be omitted when nil")
	}
}

func TestWriteEnvFile(t *testing.T) {
	home := setupTestHome(t)

	// Create the config directory (writeEnvFile uses os.WriteFile directly)
	os.MkdirAll(filepath.Join(home, config.ConfigDir), 0700)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:   "1.2.3.4",
			Port:   18443,
			Tunnel: false,
		},
		Presets: []string{"ai"},
		NoProxy: []string{"localhost"},
	}

	if err := writeEnvFile(cfg); err != nil {
		t.Fatalf("writeEnvFile: %v", err)
	}

	envPath := filepath.Join(home, config.ConfigDir, config.EnvFile)
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}

	content := string(data)
	if !contains(content, "export https_proxy=") {
		t.Error("missing https_proxy export")
	}
	if !contains(content, "export http_proxy=") {
		t.Error("missing http_proxy export")
	}
	if !contains(content, "export no_proxy=") {
		t.Error("missing no_proxy export")
	}
	if !contains(content, "export NO_PROXY=") {
		t.Error("missing NO_PROXY export")
	}
	if !contains(content, "1.2.3.4:18443") {
		t.Error("missing proxy address in env file")
	}
}

func TestPacPIDFile(t *testing.T) {
	setupTestHome(t)
	path := pacPIDFile()
	if filepath.Base(path) != "pac-server.pid" {
		t.Errorf("pacPIDFile() base = %q, want pac-server.pid", filepath.Base(path))
	}
}

// contains is a test helper for substring checks.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
