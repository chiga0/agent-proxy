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

func controlSocketFor(cfg *config.Config, host, user string) string {
	path := cfg.Proxy.SSHControlPath()
	path = strings.ReplaceAll(path, "%r", user)
	path = strings.ReplaceAll(path, "%h", host)
	path = strings.ReplaceAll(path, "%p", strconv.Itoa(22))
	return path
}

func controlSocket(cfg *config.Config) string {
	return controlSocketFor(cfg, cfg.Proxy.Host, cfg.Proxy.SSHUserOrRoot())
}

// allControlSockets returns sockets for both primary and fallback hosts.
func allControlSockets(cfg *config.Config) []string {
	sockets := []string{controlSocket(cfg)}
	if cfg.Proxy.HasFallback() {
		sockets = append(sockets, controlSocketFor(cfg, cfg.Proxy.FallbackHost, cfg.Proxy.FallbackSSHUserOrRoot()))
	}
	return sockets
}

// Start ensures the SSH tunnel is running.
// Tries primary host first; if it fails and a fallback is configured, tries fallback.
// Returns (true, nil) if started, (false, nil) if already running.
func Start(cfg *config.Config) (bool, error) {
	if Running(cfg) {
		return false, nil
	}

	// Try primary
	started, err := startTunnel(cfg, false)
	if err == nil {
		return started, nil
	}

	// Try fallback if configured
	if cfg.Proxy.HasFallback() {
		fmt.Printf("  ⚠ Primary %s failed (%v), trying fallback %s...\n",
			cfg.Proxy.Host, err, cfg.Proxy.FallbackHost)
		return startTunnel(cfg, true)
	}

	return false, err
}

func startTunnel(cfg *config.Config, useFallback bool) (bool, error) {
	var args []string
	var target string

	if useFallback {
		args = cfg.Proxy.FallbackSSHBaseArgs()
		target = cfg.Proxy.FallbackSSHTarget()
	} else {
		args = cfg.Proxy.SSHBaseArgs()
		target = cfg.Proxy.SSHTarget()
	}

	args = append(args,
		"-f", "-N",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=2",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "Ciphers=aes128-gcm@openssh.com,aes256-gcm@openssh.com,chacha20-poly1305@openssh.com",
		"-o", "Compression=no",
		"-o", "IPQoS=none",
		"-o", "TCPKeepAlive=yes",
		"-o", "BatchMode=yes",
	)
	args = append(args, cfg.Proxy.TunnelListenArgs()...)
	args = append(args, target)

	cmd := exec.Command("ssh", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("start SSH tunnel to %s: %s: %w", target, strings.TrimSpace(string(out)), err)
	}

	for i := 0; i < 100; i++ {
		time.Sleep(100 * time.Millisecond)
		if Running(cfg) {
			return true, nil
		}
	}
	return false, fmt.Errorf("SSH tunnel to %s did not start within 10s", target)
}

// Stop terminates the SSH tunnel via the ControlMaster socket.
// Tries both primary and fallback sockets.
func Stop(cfg *config.Config) {
	for _, sock := range allControlSockets(cfg) {
		if _, err := os.Stat(sock); err == nil {
			host := cfg.Proxy.Host // for -O exit target
			cmd := exec.Command("ssh",
				"-o", "ControlPath="+sock,
				"-O", "exit",
				host,
			)
			// Timeout to prevent hanging on dead connections
			done := make(chan error, 1)
			go func() { done <- cmd.Run() }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				cmd.Process.Kill()
			}
		}
	}
	// Fallback: find by pattern (covers both primary and fallback)
	user := cfg.Proxy.SSHUserOrRoot()
	patterns := []string{
		fmt.Sprintf("ssh.*-L.*%d:127.0.0.1:%d.*%s@%s",
			cfg.Proxy.LocalPort(), cfg.Proxy.Port, user, cfg.Proxy.Host),
		// IPv6 listen format
		fmt.Sprintf("ssh.*-L.*\\[::1\\]:%d:127.0.0.1:%d.*%s@%s",
			cfg.Proxy.LocalPort(), cfg.Proxy.Port, user, cfg.Proxy.Host),
	}
	if cfg.Proxy.HasFallback() {
		patterns = append(patterns,
			fmt.Sprintf("ssh.*-L.*%d:127.0.0.1:%d.*%s@%s",
				cfg.Proxy.LocalPort(), cfg.Proxy.Port, cfg.Proxy.FallbackSSHUserOrRoot(), cfg.Proxy.FallbackHost),
			fmt.Sprintf("ssh.*-L.*\\[::1\\]:%d:127.0.0.1:%d.*%s@%s",
				cfg.Proxy.LocalPort(), cfg.Proxy.Port, cfg.Proxy.FallbackSSHUserOrRoot(), cfg.Proxy.FallbackHost),
		)
	}
	for _, pattern := range patterns {
		for _, pid := range platform.FindPIDsByPattern(pattern) {
			killIfSSH(pid)
		}
	}
}

// Running checks whether the tunnel is active via the ControlMaster socket.
// Checks both primary and fallback sockets.
func Running(cfg *config.Config) bool {
	for _, sock := range allControlSockets(cfg) {
		if _, err := os.Stat(sock); err == nil {
			err := exec.Command("ssh",
				"-o", "ControlPath="+sock,
				"-O", "check",
				cfg.Proxy.Host,
			).Run()
			if err == nil {
				return true
			}
		}
	}
	return false
}

// PortOccupied checks whether something (possibly not our tunnel) is listening
// on the local tunnel port. Used for diagnostics only.
func PortOccupied(cfg *config.Config) bool {
	conn, err := net.DialTimeout("tcp",
		net.JoinHostPort(cfg.Proxy.EffectiveHost(), strconv.Itoa(cfg.Proxy.LocalPort())), 500*time.Millisecond)
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
	if comm != "ssh" && !strings.HasSuffix(comm, "/ssh") &&
		!strings.HasSuffix(comm, "\\ssh.exe") && !strings.EqualFold(comm, "ssh.exe") {
		return
	}
	if proc, err := os.FindProcess(pid); err == nil {
		proc.Kill()
	}
}
