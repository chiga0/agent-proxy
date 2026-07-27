package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.Presets) == 0 {
		t.Error("default presets should not be empty")
	}
	if len(cfg.NoProxy) == 0 {
		t.Error("default no_proxy should not be empty")
	}
	if cfg.Proxy.Port != 18443 {
		t.Errorf("default port = %d, want 18443", cfg.Proxy.Port)
	}
	if len(cfg.EffectiveWhitelist()) == 0 {
		t.Error("effective whitelist should not be empty")
	}
}

func TestAddRemoveDomain(t *testing.T) {
	cfg := DefaultConfig()
	initial := len(cfg.CustomDomains)

	if !cfg.AddDomain("example.com") {
		t.Error("AddDomain should return true for new domain")
	}
	if len(cfg.CustomDomains) != initial+1 {
		t.Errorf("custom len = %d, want %d", len(cfg.CustomDomains), initial+1)
	}
	if cfg.AddDomain("example.com") {
		t.Error("AddDomain should return false for duplicate")
	}
	if cfg.AddDomain("chatgpt.com") {
		t.Error("AddDomain should return false for preset domain")
	}
	if !cfg.RemoveDomain("example.com") {
		t.Error("RemoveDomain should return true")
	}
	if cfg.RemoveDomain("nonexistent.com") {
		t.Error("RemoveDomain should return false for non-existing")
	}
}

func TestAddDomainNormalization(t *testing.T) {
	cfg := &Config{}
	cfg.AddDomain("  Example.COM  ")
	if len(cfg.CustomDomains) != 1 || cfg.CustomDomains[0] != "example.com" {
		t.Errorf("not normalized: %v", cfg.CustomDomains)
	}
	if cfg.AddDomain("") || cfg.AddDomain("   ") {
		t.Error("should reject empty/whitespace")
	}
}

func TestPresets(t *testing.T) {
	cfg := &Config{Presets: []string{"ai"}}
	wl := cfg.EffectiveWhitelist()
	if len(wl) == 0 {
		t.Error("ai preset should have domains")
	}
	if !cfg.EnablePreset("dev") {
		t.Error("EnablePreset should succeed")
	}
	if cfg.EnablePreset("dev") || cfg.EnablePreset("nonexistent") {
		t.Error("EnablePreset should fail for dup/unknown")
	}
	if len(cfg.EffectiveWhitelist()) <= len(wl) {
		t.Error("enabling dev should add domains")
	}
	if !cfg.DisablePreset("dev") {
		t.Error("DisablePreset should succeed")
	}
	if cfg.DisablePreset("dev") {
		t.Error("DisablePreset should fail for already disabled")
	}
}

func TestEffectiveWhitelistDedup(t *testing.T) {
	cfg := &Config{Presets: []string{"ai"}, CustomDomains: []string{"chatgpt.com"}}
	count := 0
	for _, d := range cfg.EffectiveWhitelist() {
		if d == "chatgpt.com" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("chatgpt.com appears %d times, want 1", count)
	}
}

func TestProxyURL(t *testing.T) {
	cfg := &Config{Proxy: ProxyConfig{Host: "1.2.3.4", Port: 18443, User: "u", Password: "p"}}
	if cfg.ProxyURL() != "http://u:p@1.2.3.4:18443" {
		t.Errorf("ProxyURL = %q", cfg.ProxyURL())
	}
	if cfg.ProxyURLNoAuth() != "http://1.2.3.4:18443" {
		t.Errorf("ProxyURLNoAuth = %q", cfg.ProxyURLNoAuth())
	}
}

func TestProxyURLNoCredentials(t *testing.T) {
	cfg := &Config{Proxy: ProxyConfig{Host: "1.2.3.4", Port: 18443}}
	if cfg.HasAuth() {
		t.Error("HasAuth should be false without credentials")
	}
	if cfg.ProxyURL() != "http://1.2.3.4:18443" {
		t.Errorf("ProxyURL without auth = %q, want no-auth URL", cfg.ProxyURL())
	}
}

func TestEffectiveHost(t *testing.T) {
	p := ProxyConfig{Host: "1.2.3.4", Port: 18443}
	if p.EffectiveHost() != "1.2.3.4" {
		t.Errorf("EffectiveHost without tunnel = %q", p.EffectiveHost())
	}
	p.Tunnel = true
	if p.EffectiveHost() != "127.0.0.1" {
		t.Errorf("EffectiveHost with tunnel = %q, want 127.0.0.1", p.EffectiveHost())
	}
}

func TestProxyURLWithTunnel(t *testing.T) {
	cfg := &Config{Proxy: ProxyConfig{Host: "1.2.3.4", Port: 18443, Tunnel: true}}
	if cfg.ProxyURL() != "http://127.0.0.1:18443" {
		t.Errorf("ProxyURL with tunnel = %q, want 127.0.0.1", cfg.ProxyURL())
	}
}

func TestMediaPreset(t *testing.T) {
	info, ok := Presets["media"]
	if !ok {
		t.Fatal("media preset not found")
	}
	if len(info.Domains) == 0 {
		t.Error("media preset should have domains")
	}
	found := false
	for _, d := range info.Domains {
		if d == "youtube.com" {
			found = true
			break
		}
	}
	if !found {
		t.Error("media preset should include youtube.com")
	}
	// media should be in default presets
	defaults := DefaultPresets()
	hasMedia := false
	for _, p := range defaults {
		if p == "media" {
			hasMedia = true
		}
	}
	if !hasMedia {
		t.Error("media should be in default presets")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := DefaultConfig()
	cfg.Proxy.Host = "10.0.0.1"
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, _ := os.Stat(filepath.Join(tmpDir, ConfigDir, ConfigFile))
	if info.Mode().Perm() != 0600 {
		t.Errorf("perm = %o", info.Mode().Perm())
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Proxy.Host != "10.0.0.1" || len(loaded.Presets) == 0 {
		t.Error("loaded config mismatch")
	}
}
