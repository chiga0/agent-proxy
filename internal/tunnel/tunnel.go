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
)

func Start(cfg *config.Config) error {
	if Running(cfg) {
		return nil
	}

	args := []string{
		"-f", "-N",
		"-o", "StrictHostKeyChecking=no",
		"-o", "ServerAliveInterval=60",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
	}
	if cfg.Proxy.SSHKey != "" {
		args = append(args, "-i", cfg.Proxy.SSHKey)
	}
	user := cfg.Proxy.SSHUser
	if user == "" {
		user = "root"
	}
	localPort := cfg.Proxy.Port
	args = append(args,
		"-L", fmt.Sprintf("%d:127.0.0.1:%d", localPort, cfg.Proxy.Port),
		fmt.Sprintf("%s@%s", user, cfg.Proxy.Host),
	)

	cmd := exec.Command("ssh", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start SSH tunnel: %s: %w", strings.TrimSpace(string(out)), err)
	}

	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if Running(cfg) {
			return nil
		}
	}
	return fmt.Errorf("SSH tunnel did not start within 2s")
}

func Stop(cfg *config.Config) {
	localPort := cfg.Proxy.Port
	user := cfg.Proxy.SSHUser
	if user == "" {
		user = "root"
	}
	pattern := fmt.Sprintf("ssh.*-L.*%d:127.0.0.1:%d.*%s@%s",
		localPort, cfg.Proxy.Port, user, cfg.Proxy.Host)

	out, err := exec.Command("pgrep", "-f", pattern).Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err == nil {
			proc.Kill()
		}
	}
}

func Running(cfg *config.Config) bool {
	conn, err := net.DialTimeout("tcp",
		fmt.Sprintf("127.0.0.1:%d", cfg.Proxy.Port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
