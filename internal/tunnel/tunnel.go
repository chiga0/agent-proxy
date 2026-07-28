package tunnel

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/platform"
)

func controlSocket(cfg *config.Config) string {
	// Expand the ControlPath template the same way OpenSSH does.
	user := cfg.Proxy.SSHUserOrRoot()
	path := cfg.Proxy.SSHControlPath()
	path = strings.ReplaceAll(path, "%r", user)
	path = strings.ReplaceAll(path, "%h", cfg.Proxy.Host)
	path = strings.ReplaceAll(path, "%p", strconv.Itoa(22))
	return path
}

// Start ensures the SSH tunnel is running.
// Returns (true, nil) if it was started by this call, (false, nil) if already running.
func Start(cfg *config.Config) (bool, error) {
	if Running(cfg) {
		return false, nil
	}

	args := cfg.Proxy.SSHBaseArgs()
	args = append(args,
		"-f", "-N",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "Ciphers=aes128-gcm@openssh.com,aes256-gcm@openssh.com,chacha20-poly1305@openssh.com",
		"-o", "Compression=no",
		"-o", "IPQoS=throughput",
		"-o", "TCPKeepAlive=yes",
		"-o", "BatchMode=yes",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath="+cfg.Proxy.SSHControlPath(),
		"-o", "ControlPersist=600",
		"-L", fmt.Sprintf("%d:127.0.0.1:%d", cfg.Proxy.LocalPort(), cfg.Proxy.Port),
		cfg.Proxy.SSHTarget(),
	)

	cmd := exec.Command("ssh", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("start SSH tunnel: %s: %w", strings.TrimSpace(string(out)), err)
	}

	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if Running(cfg) {
			return true, nil
		}
	}
	return false, fmt.Errorf("SSH tunnel did not start within 2s")
}

// Stop terminates the SSH tunnel via the ControlMaster socket.
func Stop(cfg *config.Config) {
	sock := controlSocket(cfg)
	// Prefer graceful shutdown via ControlPath
	if _, err := os.Stat(sock); err == nil {
		exec.Command("ssh",
			"-o", "ControlPath="+sock,
			"-O", "exit",
			cfg.Proxy.SSHTarget(),
		).Run()
		return
	}
	// Fallback: find by pattern
	user := cfg.Proxy.SSHUserOrRoot()
	pattern := fmt.Sprintf("ssh.*-L.*%d:127.0.0.1:%d.*%s@%s",
		cfg.Proxy.LocalPort(), cfg.Proxy.Port, user, cfg.Proxy.Host)
	for _, pid := range platform.FindPIDsByPattern(pattern) {
		killIfSSH(pid)
	}
}

// Running checks whether the tunnel is active via the ControlMaster socket.
// A port listener alone is NOT sufficient — it may be another process.
func Running(cfg *config.Config) bool {
	sock := controlSocket(cfg)
	if _, err := os.Stat(sock); err == nil {
		err := exec.Command("ssh",
			"-o", "ControlPath="+sock,
			"-O", "check",
			cfg.Proxy.SSHTarget(),
		).Run()
		if err == nil {
			return true
		}
	}
	return false
}

// PortOccupied checks whether something (possibly not our tunnel) is listening
// on the local tunnel port. Used for diagnostics only.
func PortOccupied(cfg *config.Config) bool {
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
		return
	}
	if proc, err := os.FindProcess(pid); err == nil {
		proc.Kill()
	}
}
