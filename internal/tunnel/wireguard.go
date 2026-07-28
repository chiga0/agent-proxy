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
)

func wgPIDFile() string {
	return filepath.Join(config.DataDir(), "wireguard.pid")
}

func udp2rawPIDFile() string {
	return filepath.Join(config.DataDir(), "udp2raw.pid")
}

// StartWireGuard starts the WireGuard tunnel.
// Returns (true, nil) if started, (false, nil) if already running, (false, err) on failure.
func StartWireGuard(cfg *config.Config) (bool, error) {
	if WireGuardRunning(cfg) {
		config.ActiveTunnel = "wireguard"
		return false, nil
	}

	wg := cfg.Proxy.WireGuard
	if wg.ConfigPath == "" || wg.ServerIP == "" || wg.ClientIP == "" {
		return false, fmt.Errorf("wireguard config incomplete (need config_path, server_ip, client_ip)")
	}

	// Expand ~ in paths
	wgConfigPath := expandHome(wg.ConfigPath)
	wgGoPath := wg.WireGuardGo
	if wgGoPath == "" {
		wgGoPath = findBinary("wireguard-go")
	}
	if wgGoPath == "" {
		return false, fmt.Errorf("wireguard-go not found (set proxy.wireguard.wireguard_go_path)")
	}

	// If udp2raw is enabled, start it first
	if wg.Udp2Raw {
		if err := startUdp2Raw(cfg); err != nil {
			return false, fmt.Errorf("start udp2raw: %w", err)
		}
	}

	// Start wireguard-go (needs root on macOS for utun creation)
	cmd := exec.Command("sudo", "-n", wgGoPath, "utun")
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		// Try without sudo (might work on Linux or with capabilities)
		cmd = exec.Command(wgGoPath, "utun")
		cmd.Stderr = nil
		out, err = cmd.Output()
		if err != nil {
			stopUdp2Raw()
			return false, fmt.Errorf("start wireguard-go (try sudo agent-proxy on): %w", err)
		}
	}

	// Parse utun device name from output (e.g. "utun6")
	utun := strings.TrimSpace(string(out))
	if utun == "" {
		// Try to find it from wg show
		utunOut, _ := exec.Command("wg", "show", "interfaces").Output()
		parts := strings.Fields(string(utunOut))
		if len(parts) > 0 {
			utun = parts[len(parts)-1]
		}
	}
	if utun == "" {
		stopUdp2Raw()
		return false, fmt.Errorf("could not determine utun device name")
	}

	// Configure WireGuard interface
	if err := exec.Command("sudo", "-n", "wg", "setconf", utun, wgConfigPath).Run(); err != nil {
		// Try without sudo
		if err2 := exec.Command("wg", "setconf", utun, wgConfigPath).Run(); err2 != nil {
			stopUdp2Raw()
			return false, fmt.Errorf("wg setconf: %w", err)
		}
	}

	// Set IP address
	if err := exec.Command("sudo", "-n", "ifconfig", utun, wg.ClientIP, wg.ServerIP, "up").Run(); err != nil {
		exec.Command("ifconfig", utun, wg.ClientIP, wg.ServerIP, "up").Run()
	}

	// Save PID (wireguard-go runs in foreground, we need to find it)
	time.Sleep(500 * time.Millisecond)
	saveWgPID(utun)

	// Wait for handshake
	for i := 0; i < 30; i++ {
		time.Sleep(200 * time.Millisecond)
		if pingHost(wg.ServerIP, 500*time.Millisecond) {
			config.ActiveTunnel = "wireguard"
			return true, nil
		}
	}

	// Handshake failed — clean up
	StopWireGuard(cfg)
	return false, fmt.Errorf("WireGuard handshake timeout (UDP blocked? try udp2raw or SSH)")
}

func StopWireGuard(cfg *config.Config) {
	// Kill wireguard-go process
	if data, err := os.ReadFile(wgPIDFile()); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			exec.Command("sudo", "-n", "kill", strconv.Itoa(pid)).Run()
			exec.Command("kill", strconv.Itoa(pid)).Run()
		}
		os.Remove(wgPIDFile())
	}
	// Also kill by name
	exec.Command("sudo", "-n", "pkill", "-f", "wireguard-go").Run()
	exec.Command("pkill", "-f", "wireguard-go").Run()

	// Bring down utun interfaces
	out, _ := exec.Command("wg", "show", "interfaces").Output()
	for _, iface := range strings.Fields(string(out)) {
		exec.Command("sudo", "-n", "ifconfig", iface, "down").Run()
		exec.Command("ifconfig", iface, "down").Run()
	}

	stopUdp2Raw()
	config.ActiveTunnel = ""
}

func WireGuardRunning(cfg *config.Config) bool {
	wg := cfg.Proxy.WireGuard
	if wg.ServerIP == "" {
		return false
	}
	return pingHost(wg.ServerIP, 500*time.Millisecond)
}

// --- udp2raw client ---

func startUdp2Raw(cfg *config.Config) error {
	wg := cfg.Proxy.WireGuard
	if !wg.Udp2Raw || wg.Udp2RawRemote == "" || wg.Udp2RawKey == "" {
		return nil
	}

	localPort := wg.Udp2RawPort
	if localPort == 0 {
		localPort = 4097
	}

	bin := findBinary("udp2raw")
	if bin == "" {
		return fmt.Errorf("udp2raw binary not found")
	}

	args := []string{
		"-c",
		"-l", fmt.Sprintf("127.0.0.1:%d", localPort),
		"-r", wg.Udp2RawRemote,
		"--raw-mode", "faketcp",
		"-k", wg.Udp2RawKey,
		"--disable-color",
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start udp2raw: %w", err)
	}

	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(udp2rawPIDFile(), []byte(strconv.Itoa(cmd.Process.Pid)), 0644)
	go cmd.Wait()

	time.Sleep(500 * time.Millisecond)
	return nil
}

func stopUdp2Raw() {
	if data, err := os.ReadFile(udp2rawPIDFile()); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			exec.Command("kill", strconv.Itoa(pid)).Run()
		}
		os.Remove(udp2rawPIDFile())
	}
	exec.Command("pkill", "-f", "udp2raw.*-c").Run()
}

// --- helpers ---

func pingHost(host string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "18443"), timeout)
	if err != nil {
		// Try ICMP-like check via wg show handshake
		out, _ := exec.Command("wg", "show", "all", "latest-handshakes").Output()
		return strings.Contains(string(out), "second") || strings.Contains(string(out), "minute")
	}
	conn.Close()
	return true
}

func saveWgPID(utun string) {
	// wireguard-go doesn't print its PID; find it via pgrep
	out, err := exec.Command("pgrep", "-f", "wireguard-go.*utun").Output()
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 && lines[0] != "" {
		os.MkdirAll(config.DataDir(), 0700)
		os.WriteFile(wgPIDFile(), []byte(strings.TrimSpace(lines[0])), 0644)
	}
}

func findBinary(name string) string {
	// Check common locations
	candidates := []string{
		"/usr/local/bin/" + name,
		"/opt/homebrew/bin/" + name,
		"/tmp/wireguard-go/" + name,
	}
	home, _ := os.UserHomeDir()
	candidates = append(candidates,
		filepath.Join(home, "go", "bin", name),
	)
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	// Check PATH
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return home + path[1:]
	}
	return path
}
