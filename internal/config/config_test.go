package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if len(cfg.Whitelist) == 0 {
		t.Error("default whitelist should not be empty")
	}
	if len(cfg.NoProxy) == 0 {
		t.Error("default no_proxy should not be empty")
	}
	if cfg.Proxy.Port != 18443 {
		t.Errorf("default port = %d, want 18443", cfg.Proxy.Port)
	}
}

func TestAddRemoveDomain(t *testing.T) {
	cfg := DefaultConfig()
	initial := len(cfg.Whitelist)

	// Add new domain
	if !cfg.AddDomain("example.com") {
		t.Error("AddDomain should return true for new domain")
	}
	if len(cfg.Whitelist) != initial+1 {
		t.Errorf("whitelist len = %d, want %d", len(cfg.Whitelist), initial+1)
	}

	// Add duplicate
	if cfg.AddDomain("example.com") {
		t.Error("AddDomain should return false for duplicate")
	}
	if len(cfg.Whitelist) != initial+1 {
		t.Error("whitelist should not grow on duplicate add")
	}

	// Remove existing
	if !cfg.RemoveDomain("example.com") {
		t.Error("RemoveDomain should return true for existing domain")
	}
	if len(cfg.Whitelist) != initial {
		t.Errorf("whitelist len = %d, want %d", len(cfg.Whitelist), initial)
	}

	// Remove non-existing
	if cfg.RemoveDomain("nonexistent.com") {
		t.Error("RemoveDomain should return false for non-existing domain")
	}
}

func TestAddDomainNormalization(t *testing.T) {
	cfg := &Config{}

	cfg.AddDomain("  Example.COM  ")
	if len(cfg.Whitelist) != 1 || cfg.Whitelist[0] != "example.com" {
		t.Errorf("domain not normalized: %v", cfg.Whitelist)
	}

	// Empty string
	if cfg.AddDomain("") {
		t.Error("AddDomain should return false for empty string")
	}
	if cfg.AddDomain("   ") {
		t.Error("AddDomain should return false for whitespace")
	}
}

func TestProxyURL(t *testing.T) {
	cfg := &Config{
		Proxy: ProxyConfig{
			Host:     "1.2.3.4",
			Port:     18443,
			User:     "user",
			Password: "pass",
		},
	}

	want := "http://user:pass@1.2.3.4:18443"
	if got := cfg.ProxyURL(); got != want {
		t.Errorf("ProxyURL() = %q, want %q", got, want)
	}

	wantNoAuth := "http://1.2.3.4:18443"
	if got := cfg.ProxyURLNoAuth(); got != wantNoAuth {
		t.Errorf("ProxyURLNoAuth() = %q, want %q", got, wantNoAuth)
	}
}

func TestNoProxyString(t *testing.T) {
	cfg := &Config{
		NoProxy: []string{"localhost", "127.0.0.1", ".example.com"},
	}
	want := "localhost,127.0.0.1,.example.com"
	if got := cfg.NoProxyString(); got != want {
		t.Errorf("NoProxyString() = %q, want %q", got, want)
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Use temp dir
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := DefaultConfig()
	cfg.Proxy.Host = "10.0.0.1"
	cfg.Proxy.User = "testuser"

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file permissions
	path := filepath.Join(tmpDir, ConfigDir, ConfigFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("config file perm = %o, want 0600", perm)
	}

	// Load back
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Proxy.Host != "10.0.0.1" {
		t.Errorf("loaded host = %q, want %q", loaded.Proxy.Host, "10.0.0.1")
	}
	if loaded.Proxy.User != "testuser" {
		t.Errorf("loaded user = %q, want %q", loaded.Proxy.User, "testuser")
	}
	if len(loaded.Whitelist) != len(cfg.Whitelist) {
		t.Errorf("loaded whitelist len = %d, want %d", len(loaded.Whitelist), len(cfg.Whitelist))
	}
}
