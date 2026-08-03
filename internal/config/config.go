package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/chiga0/agent-proxy/internal/rules"
	"gopkg.in/yaml.v3"
)

const (
	AppName    = "agent-proxy"
	ConfigDir  = ".config/agent-proxy"
	ConfigFile = "config.yaml"
	PACFile    = "proxy.pac"
	EnvFile    = "env.sh"
	EnvBatFile = "env.bat"
	EnvPs1File = "env.ps1"
	PACPort    = 18080
)

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`)

type Config struct {
	Proxy         ProxyConfig  `yaml:"proxy"`
	Presets       []string     `yaml:"presets"`
	CustomDomains []string     `yaml:"custom_domains,omitempty"`
	NoProxy       []string     `yaml:"no_proxy"`
	DomainRules   []DomainRule `yaml:"domain_rules,omitempty"`
}

// DomainRule is a remote domain list subscription.
type DomainRule struct {
	URL    string `yaml:"url"`
	Action string `yaml:"action,omitempty"` // "proxy" (default) or "direct"
}

type ProxyConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	SSHKey          string `yaml:"ssh_key,omitempty"`
	SSHUser         string `yaml:"ssh_user,omitempty"`
	Tunnel          bool   `yaml:"tunnel,omitempty"`
	TunnelLocalPort int    `yaml:"tunnel_local_port,omitempty"`

	// TunnelListen controls the listen address for the SSH tunnel.
	// "ipv6" (default): listen on [::1], avoids IPv4 interception by security agents.
	// "ipv4": listen on 127.0.0.1.
	// "dual": listen on both.
	// "auto": try ipv6 first, fall back to ipv4 if data link check fails.
	TunnelListen string `yaml:"tunnel_listen,omitempty"`

	// Fallback host for automatic failover when primary is unreachable.
	FallbackHost    string `yaml:"fallback_host,omitempty"`
	FallbackSSHKey  string `yaml:"fallback_ssh_key,omitempty"`
	FallbackSSHUser string `yaml:"fallback_ssh_user,omitempty"`
}

// EffectiveHost returns the proxy host for client connections.
func (p *ProxyConfig) EffectiveHost() string {
	if p.Tunnel {
		switch p.TunnelListen {
		case "ipv4":
			return "127.0.0.1"
		default: // "ipv6" or "dual" prefer IPv6 to avoid interception
			return "::1"
		}
	}
	return p.Host
}

// LocalPort returns the port clients connect to.
// When tunnel is enabled with a custom local port, use that; otherwise use Port.
func (p *ProxyConfig) LocalPort() int {
	if p.Tunnel && p.TunnelLocalPort > 0 {
		return p.TunnelLocalPort
	}
	return p.Port
}

// TunnelListenArgs returns the -L argument(s) for SSH tunnel based on TunnelListen setting.
func (p *ProxyConfig) TunnelListenArgs() []string {
	port := p.LocalPort()
	remote := fmt.Sprintf("127.0.0.1:%d", p.Port)
	switch p.TunnelListen {
	case "ipv4":
		return []string{"-L", fmt.Sprintf("127.0.0.1:%d:%s", port, remote)}
	case "dual":
		return []string{
			"-L", fmt.Sprintf("[::1]:%d:%s", port, remote),
			"-L", fmt.Sprintf("127.0.0.1:%d:%s", port, remote),
		}
	default: // "ipv6" or empty
		return []string{"-L", fmt.Sprintf("[::1]:%d:%s", port, remote)}
	}
}

// HostConfig is a resolved SSH endpoint (primary or fallback).
type HostConfig struct {
	Host    string
	SSHKey  string
	SSHUser string
}

func (h HostConfig) Target() string {
	user := h.SSHUser
	if user == "" {
		user = "root"
	}
	return fmt.Sprintf("%s@%s", user, h.Host)
}

func (h HostConfig) BaseArgs() []string {
	sockDir := filepath.Join(DataDir(), "sockets")
	os.MkdirAll(sockDir, 0700)
	args := []string{
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + KnownHostsPath(),
		"-o", "ConnectTimeout=10",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + filepath.Join(sockDir, "%r@%h-%p"),
		"-o", "ControlPersist=600",
	}
	if h.SSHKey != "" {
		args = append(args, "-i", h.SSHKey)
	}
	return args
}

// Primary returns the resolved primary host config.
func (p *ProxyConfig) Primary() HostConfig {
	return HostConfig{Host: p.Host, SSHKey: p.SSHKey, SSHUser: p.SSHUser}
}

// Fallback returns the resolved fallback host config, or nil if not configured.
func (p *ProxyConfig) Fallback() *HostConfig {
	if p.FallbackHost == "" {
		return nil
	}
	key := p.FallbackSSHKey
	if key == "" {
		key = p.SSHKey
	}
	user := p.FallbackSSHUser
	if user == "" {
		user = p.SSHUser
	}
	return &HostConfig{Host: p.FallbackHost, SSHKey: key, SSHUser: user}
}

// SSHUserOrRoot returns the SSH user, defaulting to "root".
func (p *ProxyConfig) SSHUserOrRoot() string {
	if p.SSHUser != "" {
		return p.SSHUser
	}
	return "root"
}

// SSHControlPath returns the ControlPath socket for SSH connection multiplexing.
func (p *ProxyConfig) SSHControlPath() string {
	return filepath.Join(DataDir(), "sockets", "%r@%h-%p")
}

// SSHBaseArgs returns common SSH connection arguments shared by tunnel,
// deploy, autostart, and trace.
func (p *ProxyConfig) SSHBaseArgs() []string {
	return p.Primary().BaseArgs()
}

// SSHTarget returns the user@host string for SSH commands.
func (p *ProxyConfig) SSHTarget() string {
	return p.Primary().Target()
}

// HasFallback returns true if a fallback host is configured.
func (p *ProxyConfig) HasFallback() bool {
	return p.FallbackHost != ""
}

// FallbackSSHUserOrRoot returns the fallback SSH user, defaulting to primary or "root".
func (p *ProxyConfig) FallbackSSHUserOrRoot() string {
	if p.FallbackSSHUser != "" {
		return p.FallbackSSHUser
	}
	return p.SSHUserOrRoot()
}

// FallbackSSHKeyOrPrimary returns the fallback SSH key, defaulting to primary.
func (p *ProxyConfig) FallbackSSHKeyOrPrimary() string {
	if fb := p.Fallback(); fb != nil {
		return fb.SSHKey
	}
	return p.SSHKey
}

// FallbackSSHTarget returns the user@host for the fallback host.
func (p *ProxyConfig) FallbackSSHTarget() string {
	if fb := p.Fallback(); fb != nil {
		return fb.Target()
	}
	return ""
}

// FallbackSSHBaseArgs returns SSH args for the fallback host.
func (p *ProxyConfig) FallbackSSHBaseArgs() []string {
	if fb := p.Fallback(); fb != nil {
		return fb.BaseArgs()
	}
	return p.SSHBaseArgs()
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintf(os.Stderr, "Warning: cannot determine home directory: %v\n", err)
		return "."
	}
	return home
}

func ConfigPath() string {
	return filepath.Join(homeDir(), ConfigDir, ConfigFile)
}

func DataDir() string {
	return filepath.Join(homeDir(), ConfigDir)
}

func PACPath() string {
	return filepath.Join(DataDir(), PACFile)
}

func EnvPath() string {
	return filepath.Join(DataDir(), EnvFile)
}

func EnvBatPath() string {
	return filepath.Join(DataDir(), EnvBatFile)
}

func EnvPs1Path() string {
	return filepath.Join(DataDir(), EnvPs1File)
}

// KnownHostsPath returns the project-specific SSH known_hosts file.
func KnownHostsPath() string {
	return filepath.Join(DataDir(), "known_hosts")
}

func DefaultConfig() *Config {
	return &Config{
		Proxy: ProxyConfig{
			Port:    18443,
			SSHUser: "root",
		},
		Presets: DefaultPresets(),
		NoProxy: []string{
			"localhost", "127.0.0.1", "::1",
			"10.*",
			"172.16.*", "172.17.*", "172.18.*", "172.19.*",
			"172.20.*", "172.21.*", "172.22.*", "172.23.*",
			"172.24.*", "172.25.*", "172.26.*", "172.27.*",
			"172.28.*", "172.29.*", "172.30.*", "172.31.*",
			"192.168.*",
			".alibaba-inc.com", ".aliyun.com", ".aliyuncs.com", ".aliyun-inc.com", ".aliyunportal.com", ".taobao.org",
			// Alibaba Cloud AI services (explicit entries for clients with limited no_proxy wildcard support)
			".dashscope.aliyuncs.com", ".maas.aliyuncs.com", ".bailian.aliyuncs.com",
			".antgroup.com", ".alipay.com", ".dingtalk.com",
			".baidu.com", ".qq.com", ".tencent.com",
			".bilibili.com", ".zhihu.com",
			".npmmirror.com", ".mirrors.aliyun.com",
			// Go module proxy (needed for go install/build)
			"proxy.golang.org", "sum.golang.org", "index.golang.org",
		},
	}
}

// EffectiveWhitelist returns the union of preset domains + custom domains.
func (c *Config) EffectiveWhitelist() []string {
	domains := PresetDomains(c.Presets)
	seen := make(map[string]bool, len(domains))
	for _, d := range domains {
		seen[d] = true
	}
	for _, d := range c.CustomDomains {
		if !seen[d] {
			seen[d] = true
			domains = append(domains, d)
		}
	}
	return domains
}

// Load reads and parses the config file. Returns an error if the file
// does not exist — daemon goroutines use this to avoid side effects.
func Load() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil && cfg.Proxy.Host != "" {
		fmt.Fprintf(os.Stderr, "Warning: config validation: %v\n", err)
	}

	return cfg, nil
}

// LoadOrCreate reads the config, creating a default one on first run.
// Used by CLI entry points (PersistentPreRunE) for zero-config experience.
func LoadOrCreate() (*Config, error) {
	cfg, err := Load()
	if err == nil {
		return cfg, nil
	}
	// If the file exists but failed to parse, surface the error
	if _, statErr := os.Stat(ConfigPath()); statErr == nil {
		return nil, err
	}
	// File doesn't exist — create default
	cfg = DefaultConfig()
	if saveErr := cfg.Save(); saveErr != nil {
		return nil, fmt.Errorf("create default config: %w", saveErr)
	}
	return cfg, nil
}

// Save writes config atomically: temp file → chmod → rename.
func (c *Config) Save() error {
	dir := DataDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	path := ConfigPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// Validate checks config consistency.
func (c *Config) Validate() error {
	if c.Proxy.Host == "" {
		return fmt.Errorf("proxy.host is required")
	}
	if c.Proxy.Port < 1 || c.Proxy.Port > 65535 {
		return fmt.Errorf("proxy.port must be 1-65535, got %d", c.Proxy.Port)
	}
	if c.Proxy.Tunnel && c.Proxy.SSHKey == "" && os.Getenv("SSH_AUTH_SOCK") == "" {
		return fmt.Errorf("proxy.ssh_key is required when tunnel is enabled (or set SSH_AUTH_SOCK for ssh-agent)")
	}
	for _, p := range c.Presets {
		if _, ok := Presets[p]; !ok {
			return fmt.Errorf("unknown preset: %q", p)
		}
	}
	for _, d := range c.CustomDomains {
		if !IsValidDomain(d) {
			return fmt.Errorf("invalid domain: %q", d)
		}
	}
	return nil
}

func IsValidDomain(d string) bool {
	return domainRe.MatchString(d)
}

func (c *Config) AddDomain(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" || !IsValidDomain(domain) {
		return false
	}
	// Check if already in presets
	for _, d := range PresetDomains(c.Presets) {
		if d == domain {
			return false
		}
	}
	// Check if already in custom
	for _, d := range c.CustomDomains {
		if d == domain {
			return false
		}
	}
	c.CustomDomains = append(c.CustomDomains, domain)
	return true
}

func (c *Config) RemoveDomain(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	for i, d := range c.CustomDomains {
		if d == domain {
			c.CustomDomains = append(c.CustomDomains[:i], c.CustomDomains[i+1:]...)
			return true
		}
	}
	return false
}

func (c *Config) EnablePreset(name string) bool {
	for _, p := range c.Presets {
		if p == name {
			return false
		}
	}
	if _, ok := Presets[name]; !ok {
		return false
	}
	c.Presets = append(c.Presets, name)
	return true
}

func (c *Config) DisablePreset(name string) bool {
	for i, p := range c.Presets {
		if p == name {
			c.Presets = append(c.Presets[:i], c.Presets[i+1:]...)
			return true
		}
	}
	return false
}

func (c *Config) ProxyURL() string {
	return "http://" + net.JoinHostPort(c.Proxy.EffectiveHost(), strconv.Itoa(c.Proxy.LocalPort()))
}

// NoProxyString joins no_proxy entries. Note: reads cached rule files from disk.
func (c *Config) NoProxyString() string {
	entries := c.NoProxy
	if extra := rules.CachedDomains(DataDir(), "direct"); len(extra) > 0 {
		entries = append(append([]string{}, entries...), extra...)
	}
	return strings.Join(entries, ",")
}

// ShellQuote wraps a string in single quotes for safe POSIX shell embedding.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// WriteEnvFile writes CLI environment files for all platforms (env.sh, env.bat, env.ps1).
func (c *Config) WriteEnvFile() error {
	if err := os.MkdirAll(DataDir(), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	proxyURL := c.ProxyURL()
	noProxy := c.NoProxyString()

	// POSIX shell
	var sh strings.Builder
	sh.WriteString("# Auto-generated by agent-proxy\n")
	fmt.Fprintf(&sh, "export https_proxy=%s\n", ShellQuote(proxyURL))
	fmt.Fprintf(&sh, "export http_proxy=%s\n", ShellQuote(proxyURL))
	fmt.Fprintf(&sh, "export no_proxy=%s\n", ShellQuote(noProxy))
	sh.WriteString("export NO_PROXY=\"${no_proxy}\"\n")
	sh.WriteString("\n# npm does not read http_proxy; set its own proxy config\n")
	fmt.Fprintf(&sh, "npm config set proxy %s 2>/dev/null\n", ShellQuote(proxyURL))
	fmt.Fprintf(&sh, "npm config set https-proxy %s 2>/dev/null\n", ShellQuote(proxyURL))
	if err := os.WriteFile(EnvPath(), []byte(sh.String()), 0600); err != nil {
		return err
	}

	// Windows cmd
	var bat strings.Builder
	bat.WriteString("@echo off\r\nREM Auto-generated by agent-proxy\r\n")
	fmt.Fprintf(&bat, "set https_proxy=%s\r\n", proxyURL)
	fmt.Fprintf(&bat, "set http_proxy=%s\r\n", proxyURL)
	fmt.Fprintf(&bat, "set no_proxy=%s\r\n", noProxy)
	fmt.Fprintf(&bat, "set NO_PROXY=%s\r\n", noProxy)
	fmt.Fprintf(&bat, "npm config set proxy %s 2>nul\r\n", proxyURL)
	fmt.Fprintf(&bat, "npm config set https-proxy %s 2>nul\r\n", proxyURL)
	if err := os.WriteFile(EnvBatPath(), []byte(bat.String()), 0600); err != nil {
		return err
	}

	// Windows PowerShell
	var ps1 strings.Builder
	ps1.WriteString("# Auto-generated by agent-proxy\r\n")
	fmt.Fprintf(&ps1, "$env:https_proxy = \"%s\"\r\n", proxyURL)
	fmt.Fprintf(&ps1, "$env:http_proxy = \"%s\"\r\n", proxyURL)
	fmt.Fprintf(&ps1, "$env:no_proxy = \"%s\"\r\n", noProxy)
	fmt.Fprintf(&ps1, "$env:NO_PROXY = \"%s\"\r\n", noProxy)
	fmt.Fprintf(&ps1, "npm config set proxy \"%s\" 2>$null\r\n", proxyURL)
	fmt.Fprintf(&ps1, "npm config set https-proxy \"%s\" 2>$null\r\n", proxyURL)
	return os.WriteFile(EnvPs1Path(), []byte(ps1.String()), 0600)
}

// RemoveEnvFiles removes all generated CLI environment files.
func RemoveEnvFiles() {
	for _, p := range []string{EnvPath(), EnvBatPath(), EnvPs1Path()} {
		os.Remove(p)
	}
}
