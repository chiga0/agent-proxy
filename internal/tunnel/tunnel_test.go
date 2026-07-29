package tunnel

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestControlSocketExpansion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	tests := []struct {
		name     string
		cfg      config.ProxyConfig
		wantUser string
		wantHost string
		wantPort string
	}{
		{
			name: "root default user",
			cfg: config.ProxyConfig{
				Host: "10.0.0.1",
				Port: 18443,
			},
			wantUser: "root",
			wantHost: "10.0.0.1",
			wantPort: "22",
		},
		{
			name: "custom user",
			cfg: config.ProxyConfig{
				Host:    "192.168.1.100",
				Port:    18443,
				SSHUser: "admin",
			},
			wantUser: "admin",
			wantHost: "192.168.1.100",
			wantPort: "22",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Proxy: tt.cfg}
			sock := controlSocket(cfg)

			// The socket path should have %r, %h, %p expanded
			if strings.Contains(sock, "%r") {
				t.Error("controlSocket still contains %r placeholder")
			}
			if strings.Contains(sock, "%h") {
				t.Error("controlSocket still contains %h placeholder")
			}
			if strings.Contains(sock, "%"+"p") {
				t.Error("controlSocket still contains port placeholder")
			}

			// Verify expanded values appear in path
			if !strings.Contains(sock, tt.wantUser) {
				t.Errorf("socket path %q missing user %q", sock, tt.wantUser)
			}
			if !strings.Contains(sock, tt.wantHost) {
				t.Errorf("socket path %q missing host %q", sock, tt.wantHost)
			}
			if !strings.Contains(sock, tt.wantPort) {
				t.Errorf("socket path %q missing port %q", sock, tt.wantPort)
			}
		})
	}
}

func TestControlSocketUnderDataDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:    "1.2.3.4",
			SSHUser: "root",
		},
	}
	sock := controlSocket(cfg)

	expectedDir := filepath.Join(dir, config.ConfigDir)
	if !strings.HasPrefix(sock, expectedDir) {
		t.Errorf("controlSocket %q not under data dir %q", sock, expectedDir)
	}
}

func TestRunningWithStaleSocket(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:    "192.0.2.1", // TEST-NET, unreachable
			Port:    18443,
			SSHUser: "root",
		},
	}

	// Create a fake socket file (not a real SSH control socket)
	sock := controlSocket(cfg)
	os.MkdirAll(filepath.Dir(sock), 0700)
	os.WriteFile(sock, []byte("fake"), 0600)

	// Running should return false because ssh -O check will fail
	if Running(cfg) {
		t.Error("expected Running=false with stale/fake control socket")
	}
}

func TestPortOccupiedFalse(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Port: 19876, // unlikely to be in use
		},
	}
	if PortOccupied(cfg) {
		t.Error("expected PortOccupied=false for unused port")
	}
}

func TestPortOccupiedTrue(t *testing.T) {
	// Start a listener on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	// Extract the port
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Port: port,
		},
	}
	if !PortOccupied(cfg) {
		t.Error("expected PortOccupied=true for occupied port")
	}
}

func TestPortOccupiedTunnelLocalPort(t *testing.T) {
	// When tunnel mode is enabled, PortOccupied should check TunnelLocalPort
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			Host:            "1.2.3.4",
			Port:            18443,
			Tunnel:          true,
			TunnelLocalPort: port,
		},
	}
	if !PortOccupied(cfg) {
		t.Error("expected PortOccupied=true for tunnel local port")
	}
}

func TestKillIfSSHNonexistentPID(t *testing.T) {
	// Should not panic for a PID that doesn't exist
	killIfSSH(99999999)
}
