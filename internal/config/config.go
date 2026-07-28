package config

import (
	"fmt"
	"net"
	"net/url"
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
	User            string `yaml:"user,omitempty"`
	Password        string `yaml:"password,omitempty"`
	SSHKey          string `yaml:"ssh_key,omitempty"`
	SSHUser         string `yaml:"ssh_user,omitempty"`
	Tunnel          bool   `yaml:"tunnel,omitempty"`
	TunnelLocalPort int    `yaml:"tunnel_local_port,omitempty"`
}

// EffectiveHost returns the proxy host for client connections.
func (p *ProxyConfig) EffectiveHost() string {
	if p.Tunnel {
		return "127.0.0.1"
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
			".alibaba-inc.com", ".aliyun.com", ".taobao.org",
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
		cfg.Save()
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
	if (c.Proxy.User == "") != (c.Proxy.Password == "") {
		return fmt.Errorf("proxy.user and proxy.password must both be set or both be empty")
	}
	if c.Proxy.Tunnel && c.Proxy.SSHKey == "" {
		return fmt.Errorf("proxy.ssh_key is required when tunnel is enabled")
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

func (c *Config) HasAuth() bool {
	return c.Proxy.User != "" && c.Proxy.Password != ""
}

// ProxyURL returns the proxy URL with proper URL encoding for credentials.
func (c *Config) ProxyURL() string {
	host := c.Proxy.EffectiveHost()
	port := c.Proxy.LocalPort()
	if c.HasAuth() {
		u := url.URL{
			Scheme: "http",
			User:   url.UserPassword(c.Proxy.User, c.Proxy.Password),
			Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		}
		return u.String()
	}
	return c.ProxyURLNoAuth()
}

func (c *Config) ProxyURLNoAuth() string {
	return "http://" + net.JoinHostPort(c.Proxy.EffectiveHost(), strconv.Itoa(c.Proxy.LocalPort()))
}

func (c *Config) NoProxyString() string {
	return strings.Join(c.NoProxy, ",")
}

// ShellQuote wraps a string in single quotes for safe POSIX shell embedding.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
