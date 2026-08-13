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

// sshCommand builds an ssh subprocess with a sanitized environment.
// Warp terminal exports WARP_USE_SSH_WRAPPER=1, making its ssh wrapper
// intercept connections; that breaks tunnel startup and control-socket
// operations (-O check/exit) with "Bad file descriptor". Strip the wrapper
// variables so subprocess ssh always behaves like plain OpenSSH.
func sshCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("ssh", args...)
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "WARP_USE_SSH_WRAPPER=") ||
			strings.HasPrefix(kv, "WARP_SSH_REUSE_CONTROL_MASTER=") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = env
	return cmd
}

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

// CleanStaleSockets removes ControlMaster socket files whose master is no
// longer alive. A stale socket is poisonous: new ssh processes refuse to
// become a master ("ControlSocket ... already exists, disabling multiplexing"),
// so the tunnel may forward traffic but never recreates the socket — leaving
// `ssh -O check/exit` and Running() permanently broken.
func CleanStaleSockets(cfg *config.Config) {
	for _, sock := range allControlSockets(cfg) {
		if _, err := os.Stat(sock); err != nil {
			continue
		}
		if sshCommand("-o", "ControlPath="+sock, "-O", "check", cfg.Proxy.Host).Run() != nil {
			os.Remove(sock)
		}
	}
}

// Start ensures the SSH tunnel is running.
// Tries primary host first; if it fails and a fallback is configured, tries fallback.
// Returns (true, nil) if started, (false, nil) if already running.
func Start(cfg *config.Config) (bool, error) {
	if Running(cfg) {
		return false, nil
	}

	// A dead master leaves a stale socket behind; without cleanup the new ssh
	// would disable multiplexing and Running() would never observe success.
	CleanStaleSockets(cfg)

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
		args = cfg.Proxy.FallbackSSHTunnelBaseArgs()
		target = cfg.Proxy.FallbackSSHTarget()
	} else {
		args = cfg.Proxy.SSHTunnelBaseArgs()
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

	cmd := sshCommand(args...)
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
			cmd := sshCommand(
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
	killTunnelProcesses(cfg)
	// Remove socket files so the next ssh can become ControlMaster again.
	for _, sock := range allControlSockets(cfg) {
		os.Remove(sock)
	}
}

// KillForRestart terminates a broken tunnel process without starting a new
// one, for use when an OS supervisor (launchd / systemd / scheduled task)
// owns the tunnel lifecycle and will respawn it. Stale control sockets are
// cleaned first so the respawned ssh can become ControlMaster again.
func KillForRestart(cfg *config.Config) {
	CleanStaleSockets(cfg)
	killTunnelProcesses(cfg)
}

// killTunnelProcesses kills tunnel ssh processes by command-line pattern
// (covers both primary and fallback hosts).
func killTunnelProcesses(cfg *config.Config) {
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
			err := sshCommand(
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
