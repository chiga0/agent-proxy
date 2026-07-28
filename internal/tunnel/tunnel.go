package tunnel

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/platform"
)

func pidFile() string {
	return filepath.Join(config.DataDir(), "ssh-tunnel.pid")
}

// Start ensures the SSH tunnel is running.
// Returns (true, nil) if it was started by this call, (false, nil) if already running.
func Start(cfg *config.Config) (bool, error) {
	if Running(cfg) {
		return false, nil
	}

	args := []string{
		"-f", "-N",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "Ciphers=aes128-gcm@openssh.com,aes256-gcm@openssh.com,chacha20-poly1305@openssh.com",
		"-o", "Compression=no",
		"-o", "IPQoS=throughput",
		"-o", "TCPKeepAlive=yes",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + filepath.Join(config.DataDir(), "ssh-ctrl-%r@%h:%p"),
		"-o", "ControlPersist=600",
	}
	if cfg.Proxy.SSHKey != "" {
		args = append(args, "-i", cfg.Proxy.SSHKey)
	}
	user := cfg.Proxy.SSHUser
	if user == "" {
		user = "root"
	}
	args = append(args,
		"-L", fmt.Sprintf("%d:127.0.0.1:%d", cfg.Proxy.LocalPort(), cfg.Proxy.Port),
		fmt.Sprintf("%s@%s", user, cfg.Proxy.Host),
	)

	cmd := exec.Command("ssh", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("start SSH tunnel: %s: %w", strings.TrimSpace(string(out)), err)
	}

	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if Running(cfg) {
			savePID(cfg)
			return true, nil
		}
	}
	return false, fmt.Errorf("SSH tunnel did not start within 2s")
}

func Stop(cfg *config.Config) {
	if data, err := os.ReadFile(pidFile()); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			killIfSSH(pid)
		}
		os.Remove(pidFile())
		return
	}
	// Fallback: find by pattern
	user := cfg.Proxy.SSHUser
	if user == "" {
		user = "root"
	}
	pattern := fmt.Sprintf("ssh.*-L.*%d:127.0.0.1:%d.*%s@%s",
		cfg.Proxy.LocalPort(), cfg.Proxy.Port, user, cfg.Proxy.Host)
	for _, pid := range platform.FindPIDsByPattern(pattern) {
		killIfSSH(pid)
	}
}

// Running checks PID file + process liveness + port.
func Running(cfg *config.Config) bool {
	if data, err := os.ReadFile(pidFile()); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			if !platform.IsProcessAlive(pid) {
				os.Remove(pidFile())
				return false
			}
		}
	}
	conn, err := net.DialTimeout("tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Proxy.LocalPort())), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// killIfSSH kills a PID only if it looks like an ssh process.
func killIfSSH(pid int) {
	if !platform.IsProcessAlive(pid) {
		return
	}
	comm := platform.GetProcessName(pid)
	if comm != "ssh" && !strings.HasSuffix(comm, "/ssh") && !strings.HasSuffix(comm, "\\ssh.exe") {
		return // PID was reused by a different process
	}
	if proc, err := os.FindProcess(pid); err == nil {
		proc.Kill()
	}
}

func savePID(cfg *config.Config) {
	user := cfg.Proxy.SSHUser
	if user == "" {
		user = "root"
	}
	pattern := fmt.Sprintf("ssh.*-L.*%d:127.0.0.1:%d.*%s@%s",
		cfg.Proxy.LocalPort(), cfg.Proxy.Port, user, cfg.Proxy.Host)
	pids := platform.FindPIDsByPattern(pattern)
	if len(pids) > 0 {
		os.MkdirAll(config.DataDir(), 0700)
		os.WriteFile(pidFile(), []byte(strconv.Itoa(pids[0])), 0644)
	}
}
