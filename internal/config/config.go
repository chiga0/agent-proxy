package config

import (
	"fmt"
	"os"
	"path/filepath"
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

type Config struct {
	Proxy         ProxyConfig `yaml:"proxy"`
	Presets       []string    `yaml:"presets"`
	CustomDomains []string    `yaml:"custom_domains,omitempty"`
	NoProxy       []string    `yaml:"no_proxy"`
	// Legacy: migrated to CustomDomains on load
	Whitelist []string `yaml:"whitelist,omitempty"`
}

type ProxyConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
	SSHKey   string `yaml:"ssh_key,omitempty"`
	SSHUser  string `yaml:"ssh_user,omitempty"`
	Tunnel   bool   `yaml:"tunnel,omitempty"`
}

// EffectiveHost returns the proxy host for client connections.
// When tunnel is enabled, traffic goes through 127.0.0.1 (local SSH tunnel end).
func (p *ProxyConfig) EffectiveHost() string {
	if p.Tunnel {
		return "127.0.0.1"
	}
	return p.Host
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ConfigDir, ConfigFile)
}

func DataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ConfigDir)
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

	return cfg, nil
}

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
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c *Config) AddDomain(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
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

func (c *Config) ProxyURL() string {
	host := c.Proxy.EffectiveHost()
	if c.HasAuth() {
		return fmt.Sprintf("http://%s:%s@%s:%d",
			c.Proxy.User, c.Proxy.Password, host, c.Proxy.Port)
	}
	return c.ProxyURLNoAuth()
}

func (c *Config) ProxyURLNoAuth() string {
	return fmt.Sprintf("http://%s:%d", c.Proxy.EffectiveHost(), c.Proxy.Port)
}

func (c *Config) NoProxyString() string {
	return strings.Join(c.NoProxy, ",")
}
