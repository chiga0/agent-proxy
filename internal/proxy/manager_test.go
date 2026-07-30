package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

// setupTestHome redirects HOME/USERPROFILE to a temp dir so config.DataDir() is isolated.
func setupTestHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // Windows uses USERPROFILE
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

	// Verify file permissions are 0600 (skip on Windows — ACLs, not Unix bits)
	info, err := os.Stat(pacStatePath())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file perm = %o, want 0600", perm)
		}
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

	if err := cfg.WriteEnvFile(); err != nil {
		t.Fatalf("WriteEnvFile: %v", err)
	}

	envPath := config.EnvPath()
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

// --- loadPACStateFile edge cases ---

func TestLoadPACStateFileEmptyFile(t *testing.T) {
	setupTestHome(t)
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(pacStatePath(), []byte(""), 0600)

	_, err := loadPACStateFile()
	if err == nil {
		t.Fatal("expected error for empty file")
	}
	if !contains(err.Error(), "corrupted") {
		t.Errorf("error = %q, want mention of 'corrupted'", err.Error())
	}
}

func TestLoadPACStateFileWrongType(t *testing.T) {
	setupTestHome(t)
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(pacStatePath(), []byte(`["array","not","map"]`), 0600)

	_, err := loadPACStateFile()
	if err == nil {
		t.Fatal("expected error for JSON array (wrong type)")
	}
	if !contains(err.Error(), "corrupted") {
		t.Errorf("error = %q, want mention of 'corrupted'", err.Error())
	}
}

func TestLoadPACStateFileNullJSON(t *testing.T) {
	setupTestHome(t)
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(pacStatePath(), []byte(`null`), 0600)

	m, err := loadPACStateFile()
	if err != nil {
		t.Fatalf("null JSON should not error: %v", err)
	}
	if m != nil {
		t.Errorf("expected nil map for null JSON, got %v", m)
	}
}

// --- savePACStateFile additional cases ---

func TestSavePACStateFileCreatesDataDir(t *testing.T) {
	home := setupTestHome(t)
	// Ensure data dir does not exist
	dataDir := config.DataDir()
	os.RemoveAll(dataDir)

	state := pacStateFile{"svc": {OriginalURL: "http://x.pac"}}
	if err := savePACStateFile(state); err != nil {
		t.Fatalf("savePACStateFile should create data dir: %v", err)
	}
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("data dir should have been created")
	}
	// Verify file is inside home
	if !contains(pacStatePath(), home) {
		t.Errorf("pacStatePath %q should be under home %q", pacStatePath(), home)
	}
}

func TestSavePACStateFileSpecialChars(t *testing.T) {
	setupTestHome(t)

	state := pacStateFile{
		"Service With Spaces & <Special>": {
			OriginalURL: "http://proxy.example.com/pac?url=a&b=c",
			WasEnabled:  true,
			Extra:       map[string]string{"key with spaces": "value/with/slashes"},
		},
	}
	if err := savePACStateFile(state); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := loadPACStateFile()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	snap := loaded["Service With Spaces & <Special>"]
	if snap.OriginalURL != "http://proxy.example.com/pac?url=a&b=c" {
		t.Errorf("URL = %q", snap.OriginalURL)
	}
	if snap.Extra["key with spaces"] != "value/with/slashes" {
		t.Errorf("Extra = %v", snap.Extra)
	}
}

// --- stopPACDaemon PID file management ---

func TestStopPACDaemonNoPIDFile(t *testing.T) {
	setupTestHome(t)
	// Should not panic when no PID file exists
	stopPACDaemon()
}

func TestStopPACDaemonInvalidPIDContent(t *testing.T) {
	setupTestHome(t)
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(pacPIDFile(), []byte("not-a-number"), 0600)

	stopPACDaemon()

	// PID file should still be removed (cleanup happens regardless of parse)
	if _, err := os.Stat(pacPIDFile()); !os.IsNotExist(err) {
		t.Error("PID file should be removed even with invalid content")
	}
}

func TestStopPACDaemonStalePID(t *testing.T) {
	setupTestHome(t)
	os.MkdirAll(config.DataDir(), 0700)
	// Use a PID that almost certainly doesn't exist
	os.WriteFile(pacPIDFile(), []byte("99999999"), 0600)

	stopPACDaemon()

	if _, err := os.Stat(pacPIDFile()); !os.IsNotExist(err) {
		t.Error("PID file should be removed after stale PID cleanup")
	}
}

func TestStopPACDaemonPIDWithWhitespace(t *testing.T) {
	setupTestHome(t)
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(pacPIDFile(), []byte("  99999999\n"), 0600)

	stopPACDaemon()

	// Whitespace should be trimmed and PID parsed; file removed
	if _, err := os.Stat(pacPIDFile()); !os.IsNotExist(err) {
		t.Error("PID file should be removed after whitespace-padded PID")
	}
}

// --- killIfPACServer ---

func TestKillIfPACServerNonexistentPID(t *testing.T) {
	// GetProcessArgs returns "" for non-existent PID → early return, no panic
	killIfPACServer(99999999, "serve-pac", "__pac-server")
}

// --- Off() ---

func newTestConfig() *config.Config {
	return &config.Config{
		Proxy: config.ProxyConfig{
			Host: "127.0.0.1",
			Port: 18443,
		},
		Presets: []string{"ai"},
		NoProxy: []string{"localhost"},
	}
}

func TestOffNoStateNoEnv(t *testing.T) {
	setupTestHome(t)
	cfg := newTestConfig()

	// Off should succeed even with nothing to clean up (best-effort)
	if err := Off(cfg); err != nil {
		t.Fatalf("Off() = %v, want nil", err)
	}
}

func TestOffRemovesEnvFile(t *testing.T) {
	setupTestHome(t)
	cfg := newTestConfig()

	// Create env file
	if err := cfg.WriteEnvFile(); err != nil {
		t.Fatalf("WriteEnvFile: %v", err)
	}
	if _, err := os.Stat(config.EnvPath()); os.IsNotExist(err) {
		t.Fatal("env file should exist before Off")
	}

	if err := Off(cfg); err != nil {
		t.Fatalf("Off() = %v", err)
	}

	if _, err := os.Stat(config.EnvPath()); !os.IsNotExist(err) {
		t.Error("env file should be removed after Off")
	}
}

func TestOffWithCorruptedStateFile(t *testing.T) {
	setupTestHome(t)
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(pacStatePath(), []byte("{corrupted!!!"), 0600)

	cfg := newTestConfig()
	// Off should return nil even with corrupted state (best-effort with warnings)
	if err := Off(cfg); err != nil {
		t.Fatalf("Off() = %v, want nil even with corrupted state", err)
	}
}

func TestOffWithStateFile(t *testing.T) {
	setupTestHome(t)

	// Create a state file with a snapshot
	state := pacStateFile{
		"Wi-Fi": {OriginalURL: "", WasEnabled: false},
	}
	if err := savePACStateFile(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	cfg := newTestConfig()
	if err := Off(cfg); err != nil {
		t.Fatalf("Off() = %v", err)
	}
	// Off always returns nil — errors are printed as warnings
}

func TestOffTunnelDisabledSkipsTunnelStop(t *testing.T) {
	setupTestHome(t)
	cfg := newTestConfig()
	cfg.Proxy.Tunnel = false

	// Should not panic or error when tunnel is disabled
	if err := Off(cfg); err != nil {
		t.Fatalf("Off() = %v", err)
	}
}

// --- On() error paths ---

func TestOnTunnelFails(t *testing.T) {
	setupTestHome(t)
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:   "127.0.0.1",
			Port:   18443,
			Tunnel: true,
			SSHKey: "/nonexistent/key",
		},
		Presets: []string{"ai"},
	}

	err := On(cfg)
	if err == nil {
		Off(cfg)
		t.Fatal("expected error when SSH tunnel fails to start")
	}
	if !contains(err.Error(), "SSH tunnel") {
		t.Errorf("error should mention SSH tunnel: %v", err)
	}
}

func TestOnPACDaemonFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode: takes ~1s due to PAC daemon timeout")
	}
	setupTestHome(t)
	cfg := newTestConfig()
	cfg.Proxy.Tunnel = false

	err := On(cfg)
	if err == nil {
		Off(cfg)
		t.Skip("PAC daemon unexpectedly started — cannot test failure path")
	}
	if !contains(err.Error(), "PAC server") {
		t.Errorf("error should mention PAC server: %v", err)
	}
	// PAC file should have been written before daemon start failed
	if _, statErr := os.Stat(config.PACPath()); os.IsNotExist(statErr) {
		t.Error("PAC file should exist after pac.Write succeeded (before daemon failure)")
	}
}

// --- pacPIDFile ---

func TestPacPIDFileUnderDataDir(t *testing.T) {
	setupTestHome(t)
	pidPath := pacPIDFile()
	dataDir := config.DataDir()
	if filepath.Dir(pidPath) != dataDir {
		t.Errorf("pacPIDFile dir = %q, want data dir %q", filepath.Dir(pidPath), dataDir)
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
