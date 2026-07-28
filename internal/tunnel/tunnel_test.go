package tunnel

import (
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestRunningNoListener(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Port: 19999, // unlikely to be in use
		},
	}
	// With no control socket and no port listening, Running should return false
	if Running(cfg) {
		t.Error("expected Running=false with no control socket and no listener")
	}
}

func TestControlSocketPath(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:    "1.2.3.4",
			SSHUser: "root",
		},
	}
	sock := controlSocket(cfg)
	if sock == "" {
		t.Error("controlSocket() returned empty string")
	}
}
