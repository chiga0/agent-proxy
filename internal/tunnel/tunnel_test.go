package tunnel

import (
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestRunningNoPIDFile(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Port: 19999, // unlikely to be in use
		},
	}
	// With no PID file and no port listening, Running should return false
	if Running(cfg) {
		t.Error("expected Running=false with no PID file and no listener")
	}
}

func TestPIDFilePath(t *testing.T) {
	p := pidFile()
	if p == "" {
		t.Error("pidFile() returned empty string")
	}
}
