package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/pac"
	"github.com/chiga0/agent-proxy/internal/platform"
	"github.com/chiga0/agent-proxy/internal/tunnel"
)

// pacState records the system PAC state before agent-proxy modifies it,
// so that Off() can restore the original configuration.
type pacState struct {
	Service     string `json:"service"`
	OriginalURL string `json:"original_url"`
	WasEnabled  bool   `json:"was_enabled"`
}

func pacStatePath() string {
	return filepath.Join(config.DataDir(), "pac-state.json")
}

func savePACState(service string) {
	pacURL, enabled, err := platform.GetAutoProxy(service)
	if err != nil {
		pacURL = ""
		enabled = false
	}
	state := pacState{
		Service:     service,
		OriginalURL: pacURL,
		WasEnabled:  enabled,
	}
	data, err := json.Marshal(state)
	if err != nil {
		return
	}
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(pacStatePath(), data, 0600)
}

func loadPACState() (*pacState, error) {
	data, err := os.ReadFile(pacStatePath())
	if err != nil {
		return nil, err
	}
	var state pacState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func On(cfg *config.Config) error {
	var undo []func()

	fail := func(err error) error {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
		return err
	}

	// 1. Start SSH tunnel if enabled
	if cfg.Proxy.Tunnel {
		fmt.Print("  → SSH tunnel... ")
		started, err := tunnel.Start(cfg)
		if err != nil {
			fmt.Println("✗")
			return fmt.Errorf("start SSH tunnel: %w", err)
		}
		fmt.Println("✓")
		if started {
			undo = append(undo, func() { tunnel.Stop(cfg) })
		}
	}

	// 2. Generate PAC
	if err := pac.Write(cfg); err != nil {
		return fail(fmt.Errorf("generate PAC: %w", err))
	}

	// 3. Start PAC HTTP server (background daemon with PID file)
	fmt.Print("  → PAC server... ")
	pacStarted, err := startPACDaemon()
	if err != nil {
		fmt.Println("✗")
		return fail(fmt.Errorf("start PAC server: %w", err))
	}
	fmt.Println("✓")
	if pacStarted {
		undo = append(undo, stopPACDaemon)
	}

	// 4. Save original PAC state, then set system PAC proxy
	service, err := platform.DetectNetworkService()
	if err != nil {
		return fail(fmt.Errorf("detect network: %w", err))
	}
	savePACState(service)

	pacURL := fmt.Sprintf("http://127.0.0.1:%d/proxy.pac", config.PACPort)
	if err := platform.SetAutoProxy(service, pacURL); err != nil {
		return fail(fmt.Errorf("set PAC proxy: %w", err))
	}
	undo = append(undo, func() { restorePACState(service) })

	// 5. Write CLI env file
	if err := writeEnvFile(cfg); err != nil {
		return fail(fmt.Errorf("write env file: %w", err))
	}
	undo = append(undo, func() { os.Remove(config.EnvPath()) })

	// 6. Register auto-start (write config files only, don't start services)
	if err := platform.InstallAutoStart(cfg); err != nil {
		fmt.Printf("  ⚠ Auto-start registration failed: %v\n", err)
	}

	fmt.Printf("\n✓ Proxy enabled\n")
	fmt.Printf("  PAC (browser/desktop): %s\n", pacURL)
	fmt.Printf("  CLI env:               %s\n", config.EnvPath())
	fmt.Printf("  Proxy:                 %s:%d", cfg.Proxy.EffectiveHost(), cfg.Proxy.LocalPort())
	if cfg.Proxy.Tunnel {
		fmt.Printf(" (via SSH tunnel → %s:%d)", cfg.Proxy.Host, cfg.Proxy.Port)
	}
	fmt.Println()
	fmt.Printf("  Network service:       %s\n", service)
	fmt.Printf("  Whitelist:             %d domains (presets: %v)\n", len(cfg.EffectiveWhitelist()), cfg.Presets)
	fmt.Printf("\nCurrent terminal: source %s\n", config.EnvPath())
	return nil
}

func Off(cfg *config.Config) error {
	var errs []string

	// 1. Restore original system PAC state
	service, err := platform.DetectNetworkService()
	if err == nil {
		restorePACState(service)
	}

	// 2. Stop PAC HTTP server daemon
	stopPACDaemon()

	// 3. Stop SSH tunnel
	if cfg.Proxy.Tunnel {
		tunnel.Stop(cfg)
	}

	// 4. Remove auto-start
	platform.UninstallAutoStart()

	// 5. Remove CLI env file
	if err := os.Remove(config.EnvPath()); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("remove env.sh: %v", err))
	}

	// 6. Remove PAC state file
	os.Remove(pacStatePath())

	if len(errs) > 0 {
		fmt.Printf("✓ Proxy disabled (with warnings)\n")
		for _, e := range errs {
			fmt.Printf("  ⚠ %s\n", e)
		}
	} else {
		fmt.Println("✓ Proxy disabled")
	}
	fmt.Println("  Current terminal: unset https_proxy http_proxy no_proxy NO_PROXY")
	return nil
}

// restorePACState restores the original PAC configuration.
// If the current PAC URL does not belong to agent-proxy, it is left untouched.
func restorePACState(currentService string) {
	state, err := loadPACState()
	if err != nil {
		// No saved state — just clear
		platform.ClearAutoProxy(currentService)
		return
	}

	// Only modify if the current PAC still belongs to agent-proxy
	pacURL, enabled, err := platform.GetAutoProxy(currentService)
	if err == nil && enabled && !strings.Contains(pacURL, "127.0.0.1:"+strconv.Itoa(config.PACPort)) {
		// PAC was changed by something else — don't touch it
		return
	}

	if state.WasEnabled && state.OriginalURL != "" {
		platform.SetAutoProxy(currentService, state.OriginalURL)
	} else {
		platform.ClearAutoProxy(currentService)
	}
}

func pacPIDFile() string {
	return filepath.Join(config.DataDir(), "pac-server.pid")
}

// startPACDaemon starts the PAC server if not already running.
// Returns (true, nil) if started by this call, (false, nil) if already running.
func startPACDaemon() (bool, error) {
	if pac.ServerRunning() {
		return false, nil
	}

	self, err := os.Executable()
	if err != nil {
		return false, err
	}

	cmd := exec.Command(self, "serve-pac")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	platform.DetachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return false, err
	}

	pid := cmd.Process.Pid
	os.MkdirAll(config.DataDir(), 0700)
	os.WriteFile(pacPIDFile(), []byte(strconv.Itoa(pid)), 0600)

	go cmd.Wait()

	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		if pac.ServerRunning() {
			return true, nil
		}
	}

	// Cleanup on failure: kill the child and remove stale PID
	if proc, err := os.FindProcess(pid); err == nil {
		proc.Kill()
	}
	os.Remove(pacPIDFile())
	return false, fmt.Errorf("PAC server did not start within 1s")
}

func stopPACDaemon() {
	if data, err := os.ReadFile(pacPIDFile()); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			killIfPACServer(pid, "serve-pac", "__pac-server")
		}
		os.Remove(pacPIDFile())
	}
	// Fallback: find both current and legacy command names
	for _, pattern := range []string{"serve-pac", "__pac-server"} {
		for _, pid := range platform.FindPIDsByPattern(pattern) {
			killIfPACServer(pid, pattern)
		}
	}
}

// killIfPACServer kills a PID only if its args contain one of the expected patterns.
func killIfPACServer(pid int, patterns ...string) {
	args := platform.GetProcessArgs(pid)
	if args == "" {
		return
	}
	for _, p := range patterns {
		if strings.Contains(args, p) {
			exec.Command("kill", strconv.Itoa(pid)).Run()
			return
		}
	}
}

func writeEnvFile(cfg *config.Config) error {
	var b strings.Builder
	b.WriteString("# Auto-generated by agent-proxy\n")
	fmt.Fprintf(&b, "export https_proxy=%s\n", config.ShellQuote(cfg.ProxyURL()))
	fmt.Fprintf(&b, "export http_proxy=%s\n", config.ShellQuote(cfg.ProxyURL()))
	fmt.Fprintf(&b, "export no_proxy=%s\n", config.ShellQuote(cfg.NoProxyString()))
	b.WriteString("export NO_PROXY=\"${no_proxy}\"\n")

	return os.WriteFile(config.EnvPath(), []byte(b.String()), 0600)
}
