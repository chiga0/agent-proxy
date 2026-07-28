package config

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestIsValidDomain(t *testing.T) {
	valid := []string{"google.com", "sub.domain.org", "a-b.c-d.com", "x.co"}
	for _, d := range valid {
		if !IsValidDomain(d) {
			t.Errorf("IsValidDomain(%q) = false, want true", d)
		}
	}
	invalid := []string{"", "not a domain", "http://evil.com", "a\";alert(1)", "-bad.com", ".com", "a b.com"}
	for _, d := range invalid {
		if IsValidDomain(d) {
			t.Errorf("IsValidDomain(%q) = true, want false", d)
		}
	}
}

func TestAddDomainRejectsInvalid(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AddDomain("not valid!") {
		t.Error("should reject invalid domain")
	}
	if cfg.AddDomain("a\";return \"PROXY evil:1") {
		t.Error("should reject PAC injection attempt")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"simple", "'simple'"},
		{"has space", "'has space'"},
		{"has'quote", "'has'\\''quote'"},
		{"$(cmd)", "'$(cmd)'"},
	}
	for _, tt := range tests {
		if got := ShellQuote(tt.in); got != tt.want {
			t.Errorf("ShellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLocalPort(t *testing.T) {
	p := ProxyConfig{Port: 18443}
	if p.LocalPort() != 18443 {
		t.Errorf("LocalPort without tunnel = %d", p.LocalPort())
	}
	p.Tunnel = true
	if p.LocalPort() != 18443 {
		t.Errorf("LocalPort with tunnel, no custom = %d", p.LocalPort())
	}
	p.TunnelLocalPort = 18444
	if p.LocalPort() != 18444 {
		t.Errorf("LocalPort with custom = %d, want 18444", p.LocalPort())
	}
}

func TestValidate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Proxy.Host = "1.2.3.4"
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}

	cfg.Proxy.Host = ""
	if err := cfg.Validate(); err == nil {
		t.Error("empty host should fail validation")
	}

	cfg.Proxy.Host = "1.2.3.4"
	cfg.Proxy.User = "user"
	cfg.Proxy.Password = ""
	if err := cfg.Validate(); err == nil {
		t.Error("user without password should fail")
	}

	cfg.Proxy.User = ""
	cfg.Proxy.Tunnel = true
	cfg.Proxy.SSHKey = ""
	if err := cfg.Validate(); err == nil {
		t.Error("tunnel without ssh_key should fail")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir) // Windows uses USERPROFILE

	cfg := DefaultConfig()
	cfg.Proxy.Host = "10.0.0.1"
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(tmpDir, ConfigDir, ConfigFile))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// Windows uses ACLs, not Unix permission bits — skip perm check
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
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
