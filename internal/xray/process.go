package xray

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
)

func binaryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "agent-proxy", "xray")
}

func pidFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "agent-proxy", "xray.pid")
}

func clientConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "agent-proxy", "xray-client.json")
}

// EnsureBinary checks if xray is installed locally, downloads if not.
func EnsureBinary() error {
	bin := binaryPath()
	if _, err := os.Stat(bin); err == nil {
		return nil
	}
	return downloadBinary(bin)
}

func downloadBinary(dest string) error {
	fmt.Print("  → Downloading xray... ")

	os.MkdirAll(filepath.Dir(dest), 0755)

	// Detect OS/arch
	goos, goarch := detectPlatform()
	url := fmt.Sprintf("https://github.com/XTLS/Xray-core/releases/latest/download/Xray-%s-%s.zip", goos, goarch)

	tmpZip := dest + ".zip"
	out, err := exec.Command("curl", "-sL", "--max-time", "60", "-o", tmpZip, url).CombinedOutput()
	if err != nil {
		fmt.Println("✗")
		return fmt.Errorf("download xray: %s: %w", string(out), err)
	}
	defer os.Remove(tmpZip)

	// Extract
	tmpDir := dest + "-extract"
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	if out, err := exec.Command("unzip", "-o", tmpZip, "-d", tmpDir).CombinedOutput(); err != nil {
		fmt.Println("✗")
		return fmt.Errorf("extract xray: %s: %w", string(out), err)
	}

	// Find binary
	extracted := filepath.Join(tmpDir, "xray")
	if _, err := os.Stat(extracted); err != nil {
		fmt.Println("✗")
		return fmt.Errorf("xray binary not found in archive")
	}

	if err := os.Rename(extracted, dest); err != nil {
		fmt.Println("✗")
		return fmt.Errorf("install xray: %w", err)
	}
	os.Chmod(dest, 0755)

	fmt.Println("✓")
	return nil
}

func detectPlatform() (string, string) {
	goos := "linux"
	goarch := "64"

	out, _ := exec.Command("uname", "-s").Output()
	switch strings.TrimSpace(string(out)) {
	case "Darwin":
		goos = "macos"
	case "Linux":
		goos = "linux"
	}

	out, _ = exec.Command("uname", "-m").Output()
	switch strings.TrimSpace(string(out)) {
	case "arm64", "aarch64":
		goarch = "arm64-v8a"
	default:
		goarch = "64"
	}

	return goos, goarch
}

// Start launches the local xray client process.
// Returns (true, nil) if started, (false, nil) if already running.
func Start(cfg *config.Config) (bool, error) {
	if Running(cfg) {
		return false, nil
	}

	if err := EnsureBinary(); err != nil {
		return false, err
	}

	// Write client config
	confData, err := ClientConfig(cfg)
	if err != nil {
		return false, fmt.Errorf("generate xray config: %w", err)
	}
	confPath := clientConfigPath()
	if err := os.WriteFile(confPath, confData, 0600); err != nil {
		return false, fmt.Errorf("write xray config: %w", err)
	}

	bin := binaryPath()
	cmd := exec.Command(bin, "run", "-c", confPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("start xray: %w", err)
	}

	pid := cmd.Process.Pid
	os.WriteFile(pidFile(), []byte(strconv.Itoa(pid)), 0644)
	go cmd.Wait()

	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if Running(cfg) {
			return true, nil
		}
	}

	// Cleanup on failure
	if proc, err := os.FindProcess(pid); err == nil {
		proc.Kill()
	}
	os.Remove(pidFile())
	return false, fmt.Errorf("xray did not start within 3s")
}

// Stop kills the local xray process.
func Stop(cfg *config.Config) {
	if data, err := os.ReadFile(pidFile()); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			killIfXray(pid)
		}
		os.Remove(pidFile())
	}
}

// Running checks if xray is listening on the expected port.
func Running(cfg *config.Config) bool {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Proxy.LocalPort()))
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func killIfXray(pid int) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return
	}
	comm := strings.TrimSpace(string(out))
	if !strings.Contains(comm, "xray") {
		return
	}
	if proc, err := os.FindProcess(pid); err == nil {
		proc.Kill()
	}
}
