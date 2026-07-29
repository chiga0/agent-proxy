package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	AppName    = "agent-proxy"
	ConfigDir  = ".config/agent-proxy"
	ConfigFile = "config.yaml"
	PACFile    = "proxy.pac"
	EnvFile    = "env.sh"
	PACPort    = 18080
)

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`)

type Config struct {
	Proxy         ProxyConfig `yaml:"proxy"`
	Presets       []string    `yaml:"presets"`
	CustomDomains []string    `yaml:"custom_domains,omitempty"`
	NoProxy       []string    `yaml:"no_proxy"`
	// Legacy: migrated to CustomDomains on load
	Whitelist []string `yaml:"whitelist,omitempty"`
}

type ProxyConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	SSHKey          string `yaml:"ssh_key,omitempty"`
	SSHUser         string `yaml:"ssh_user,omitempty"`
	Tunnel          bool   `yaml:"tunnel,omitempty"`
	TunnelLocalPort int    `yaml:"tunnel_local_port,omitempty"`

	// Transport backend: "ssh" (default) or "xray"
	Transport string       `yaml:"transport,omitempty"`
	Xray      XrayConfig   `yaml:"xray,omitempty"`

	// Fallback host for automatic failover when primary is unreachable.
	FallbackHost    string `yaml:"fallback_host,omitempty"`
	FallbackSSHKey  string `yaml:"fallback_ssh_key,omitempty"`
	FallbackSSHUser string `yaml:"fallback_ssh_user,omitempty"`
}

type XrayConfig struct {
	LocalPort  int    `yaml:"local_port,omitempty"`  // Local HTTP proxy port (default 18443)
	PublicKey  string `yaml:"public_key,omitempty"`   // Reality public key
	PrivateKey string `yaml:"private_key,omitempty"`  // Reality private key (server only)
	ServerName string `yaml:"server_name,omitempty"`  // Reality SNI (default www.microsoft.com)
	ShortID    string `yaml:"short_id,omitempty"`     // Reality short ID
	UUID       string `yaml:"uuid,omitempty"`         // VLESS user ID
	Mux        bool   `yaml:"mux,omitempty"`          // Connection multiplexing (default true)
}

// EffectiveHost returns the proxy host for client connections.
func (p *ProxyConfig) EffectiveHost() string {
	if p.IsXray() || p.Tunnel {
		return "127.0.0.1"
	}
	return p.Host
}

// LocalPort returns the port clients connect to.
func (p *ProxyConfig) LocalPort() int {
	if p.IsXray() {
		if p.Xray.LocalPort > 0 {
			return p.Xray.LocalPort
		}
		return p.Port
	}
	if p.Tunnel && p.TunnelLocalPort > 0 {
		return p.TunnelLocalPort
	}
	return p.Port
}

func (p *ProxyConfig) IsXray() bool {
	return p.Transport == "xray"
}

func (p *ProxyConfig) XrayServerName() string {
	if p.Xray.ServerName != "" {
		return p.Xray.ServerName
	}
	return "www.microsoft.com"
}

func (p *ProxyConfig) XrayMuxEnabled() bool {
	// Default true for xray transport
	return p.Xray.Mux || p.Xray.Mux == false && p.IsXray() && !muxExplicitlyDisabled(p)
}

func muxExplicitlyDisabled(p *ProxyConfig) bool {
	// If mux field is explicitly set to false in YAML, respect it
	// Since we can't distinguish "not set" from "false" with bool,
	// we default to true and let users set mux: false to disable
	return false
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
	return filepath.Join(DataDir(), "ssh-ctrl-%r@%h:%p")
}

// SSHBaseArgs returns common SSH connection arguments shared by tunnel,
// deploy, autostart, and trace.
func (p *ProxyConfig) SSHBaseArgs() []string {
	args := []string{
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + KnownHostsPath(),
		"-o", "ConnectTimeout=10",
	}
	if p.SSHKey != "" {
		args = append(args, "-i", p.SSHKey)
	}
	return args
}

// SSHTarget returns the user@host string for SSH commands.
func (p *ProxyConfig) SSHTarget() string {
	return fmt.Sprintf("%s@%s", p.SSHUserOrRoot(), p.Host)
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
	if p.FallbackSSHKey != "" {
		return p.FallbackSSHKey
	}
	return p.SSHKey
}

// FallbackSSHTarget returns the user@host for the fallback host.
func (p *ProxyConfig) FallbackSSHTarget() string {
	return fmt.Sprintf("%s@%s", p.FallbackSSHUserOrRoot(), p.FallbackHost)
}

// FallbackSSHBaseArgs returns SSH args for the fallback host.
func (p *ProxyConfig) FallbackSSHBaseArgs() []string {
	args := []string{
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + KnownHostsPath(),
		"-o", "ConnectTimeout=10",
	}
	key := p.FallbackSSHKeyOrPrimary()
	if key != "" {
		args = append(args, "-i", key)
	}
	return args
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

func Load() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			if err := cfg.Save(); err != nil {
				return nil, fmt.Errorf("create default config: %w", err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Migrate legacy whitelist → custom_domains + default presets
	if len(cfg.Whitelist) > 0 && len(cfg.Presets) == 0 {
		cfg.Presets = DefaultPresets()
		presetSet := make(map[string]bool)
		for _, d := range PresetDomains(cfg.Presets) {
			presetSet[d] = true
		}
		for _, d := range cfg.Whitelist {
			if !presetSet[d] {
				cfg.CustomDomains = append(cfg.CustomDomains, d)
			}
		}
		cfg.Whitelist = nil
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save migrated config: %v\n", err)
		}
	}

	// Migrate legacy Basic auth credentials: detect and strip user/password
	// fields that existed in versions prior to the security audit fix.
	var raw map[string]interface{}
	if yaml.Unmarshal(data, &raw) == nil {
		if proxy, ok := raw["proxy"].(map[string]interface{}); ok {
			_, hasUser := proxy["user"]
			_, hasPass := proxy["password"]
			if hasUser || hasPass {
				if err := cfg.Save(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to remove legacy Basic auth credentials: %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "Notice: removed legacy Basic auth credentials from config (no longer supported)\n")
				}
			}
		}
	}

	// Validate config consistency (warn but don't block for backward compat)
	if err := cfg.Validate(); err != nil && cfg.Proxy.Host != "" {
		fmt.Fprintf(os.Stderr, "Warning: config validation: %v\n", err)
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

func (c *Config) NoProxyString() string {
	return strings.Join(c.NoProxy, ",")
}

// ShellQuote wraps a string in single quotes for safe POSIX shell embedding.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// WriteEnvFile writes the CLI environment file (env.sh) with current proxy settings.
func (c *Config) WriteEnvFile() error {
	var b strings.Builder
	b.WriteString("# Auto-generated by agent-proxy\n")
	fmt.Fprintf(&b, "export https_proxy=%s\n", ShellQuote(c.ProxyURL()))
	fmt.Fprintf(&b, "export http_proxy=%s\n", ShellQuote(c.ProxyURL()))
	fmt.Fprintf(&b, "export no_proxy=%s\n", ShellQuote(c.NoProxyString()))
	b.WriteString("export NO_PROXY=\"${no_proxy}\"\n")
	return os.WriteFile(EnvPath(), []byte(b.String()), 0600)
}
