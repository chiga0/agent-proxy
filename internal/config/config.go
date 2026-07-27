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
	Proxy     ProxyConfig  `yaml:"proxy"`
	Whitelist []string     `yaml:"whitelist"`
	NoProxy   []string     `yaml:"no_proxy"`
}

type ProxyConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSHKey   string `yaml:"ssh_key"`
	SSHUser  string `yaml:"ssh_user"`
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
		Whitelist: []string{
			"chatgpt.com",
			"openai.com",
			"oaistatic.com",
			"oaiusercontent.com",
			"anthropic.com",
			"claude.ai",
			"openrouter.ai",
			"github.com",
			"githubusercontent.com",
			"ipinfo.io",
			"googleapis.com",
			"gemini.google.com",
			"google.com",
		},
		NoProxy: []string{
			"localhost",
			"127.0.0.1",
			"::1",
			"10.*",
			"172.16.*", "172.17.*", "172.18.*", "172.19.*",
			"172.20.*", "172.21.*", "172.22.*", "172.23.*",
			"172.24.*", "172.25.*", "172.26.*", "172.27.*",
			"172.28.*", "172.29.*", "172.30.*", "172.31.*",
			"192.168.*",
			".alibaba-inc.com",
			".aliyun.com",
			".taobao.org",
			".antgroup.com",
			".alipay.com",
			".dingtalk.com",
			".baidu.com",
			".qq.com",
			".tencent.com",
			".bilibili.com",
			".zhihu.com",
			".npmmirror.com",
			".mirrors.aliyun.com",
		},
	}
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
	for _, d := range c.Whitelist {
		if d == domain {
			return false
		}
	}
	c.Whitelist = append(c.Whitelist, domain)
	return true
}

func (c *Config) RemoveDomain(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	for i, d := range c.Whitelist {
		if d == domain {
			c.Whitelist = append(c.Whitelist[:i], c.Whitelist[i+1:]...)
			return true
		}
	}
	return false
}

func (c *Config) ProxyURL() string {
	return fmt.Sprintf("http://%s:%s@%s:%d",
		c.Proxy.User, c.Proxy.Password, c.Proxy.Host, c.Proxy.Port)
}

func (c *Config) ProxyURLNoAuth() string {
	return fmt.Sprintf("http://%s:%d", c.Proxy.Host, c.Proxy.Port)
}

func (c *Config) NoProxyString() string {
	return strings.Join(c.NoProxy, ",")
}
