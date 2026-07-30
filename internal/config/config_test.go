package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	cfg := &Config{Proxy: ProxyConfig{Host: "1.2.3.4", Port: 18443}}
	if cfg.ProxyURL() != "http://1.2.3.4:18443" {
		t.Errorf("ProxyURL = %q", cfg.ProxyURL())
	}
}

func TestProxyURLNoCredentials(t *testing.T) {
	cfg := &Config{Proxy: ProxyConfig{Host: "1.2.3.4", Port: 18443}}
	if cfg.ProxyURL() != "http://1.2.3.4:18443" {
		t.Errorf("ProxyURL = %q, want plain URL", cfg.ProxyURL())
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
	cfg.Proxy.Tunnel = true
	cfg.Proxy.SSHKey = ""
	t.Setenv("SSH_AUTH_SOCK", "") // ensure ssh-agent doesn't bypass the check
	if err := cfg.Validate(); err == nil {
		t.Error("tunnel without ssh_key should fail")
	}
}

func TestWriteEnvFileIncludesNpmProxy(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cfg := &Config{
		Proxy:   ProxyConfig{Host: "1.2.3.4", Port: 18443},
		NoProxy: []string{"localhost"},
	}
	if err := cfg.WriteEnvFile(); err != nil {
		t.Fatalf("WriteEnvFile: %v", err)
	}
	data, err := os.ReadFile(EnvPath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "npm config set proxy") {
		t.Error("env.sh missing npm proxy config")
	}
	if !strings.Contains(content, "npm config set https-proxy") {
		t.Error("env.sh missing npm https-proxy config")
	}
	if !strings.Contains(content, "http://1.2.3.4:18443") {
		t.Error("env.sh missing proxy URL in npm config")
	}
}

func TestWriteEnvFileWindowsFormats(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cfg := &Config{
		Proxy:   ProxyConfig{Host: "1.2.3.4", Port: 18443},
		NoProxy: []string{"localhost", "127.0.0.1"},
	}
	if err := cfg.WriteEnvFile(); err != nil {
		t.Fatalf("WriteEnvFile: %v", err)
	}

	// env.bat
	bat, err := os.ReadFile(EnvBatPath())
	if err != nil {
		t.Fatalf("env.bat not created: %v", err)
	}
	batStr := string(bat)
	if !strings.Contains(batStr, "@echo off") {
		t.Error("env.bat missing @echo off")
	}
	if !strings.Contains(batStr, "set https_proxy=http://1.2.3.4:18443") {
		t.Error("env.bat missing https_proxy")
	}
	if !strings.Contains(batStr, "set no_proxy=localhost,127.0.0.1") {
		t.Error("env.bat missing no_proxy")
	}

	// env.ps1
	ps1, err := os.ReadFile(EnvPs1Path())
	if err != nil {
		t.Fatalf("env.ps1 not created: %v", err)
	}
	ps1Str := string(ps1)
	if !strings.Contains(ps1Str, `$env:https_proxy = "http://1.2.3.4:18443"`) {
		t.Error("env.ps1 missing https_proxy")
	}
	if !strings.Contains(ps1Str, `$env:no_proxy = "localhost,127.0.0.1"`) {
		t.Error("env.ps1 missing no_proxy")
	}
}

func TestRemoveEnvFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cfg := &Config{
		Proxy:   ProxyConfig{Host: "1.2.3.4", Port: 18443},
		NoProxy: []string{"localhost"},
	}
	if err := cfg.WriteEnvFile(); err != nil {
		t.Fatalf("WriteEnvFile: %v", err)
	}
	for _, p := range []string{EnvPath(), EnvBatPath(), EnvPs1Path()} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
	}
	RemoveEnvFiles()
	for _, p := range []string{EnvPath(), EnvBatPath(), EnvPs1Path()} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", p)
		}
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

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

func TestLoadOrCreate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// No config file → LoadOrCreate creates default
	cfg, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate (no file): %v", err)
	}
	if cfg.Proxy.Port != 18443 {
		t.Errorf("default port = %d, want 18443", cfg.Proxy.Port)
	}
	// File should now exist
	if _, err := os.Stat(ConfigPath()); err != nil {
		t.Error("LoadOrCreate should have created config file")
	}

	// Existing valid config → LoadOrCreate returns it
	cfg.Proxy.Host = "1.2.3.4"
	cfg.Save()
	cfg2, err := LoadOrCreate()
	if err != nil {
		t.Fatalf("LoadOrCreate (existing): %v", err)
	}
	if cfg2.Proxy.Host != "1.2.3.4" {
		t.Errorf("host = %q, want 1.2.3.4", cfg2.Proxy.Host)
	}

	// Corrupt config → LoadOrCreate returns error (not auto-create)
	os.WriteFile(ConfigPath(), []byte("{{{invalid"), 0600)
	_, err = LoadOrCreate()
	if err == nil {
		t.Error("LoadOrCreate should fail on corrupt config")
	}
}
