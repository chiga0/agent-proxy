package proxy

import (
	"encoding/json"
	"errors"
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

// pacURL is the exact PAC URL that agent-proxy sets on the system.
func agentProxyPACURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/proxy.pac", config.PACPort)
}

// isOurPAC returns true if the given URL belongs to agent-proxy.
func isOurPAC(url string) bool {
	return url == agentProxyPACURL()
}

// pacSnapshot records the system PAC state for one network service
// before agent-proxy modifies it.
type pacSnapshot struct {
	OriginalURL string            `json:"original_url"`
	WasEnabled  bool              `json:"was_enabled"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// pacStateFile maps network service name → snapshot.
type pacStateFile map[string]pacSnapshot

func pacStatePath() string {
	return filepath.Join(config.DataDir(), "pac-state.json")
}

func loadPACStateFile() (pacStateFile, error) {
	data, err := os.ReadFile(pacStatePath())
	if err != nil {
		return nil, err
	}
	var m pacStateFile
	if err := json.Unmarshal(data, &m); err != nil {
		// Try legacy single-service format
		var legacy struct {
			Service     string `json:"service"`
			OriginalURL string `json:"original_url"`
			WasEnabled  bool   `json:"was_enabled"`
		}
		if json.Unmarshal(data, &legacy) == nil && legacy.Service != "" {
			return pacStateFile{legacy.Service: {OriginalURL: legacy.OriginalURL, WasEnabled: legacy.WasEnabled}}, nil
		}
		return nil, fmt.Errorf("corrupted PAC state file: %w", err)
	}
	return m, nil
}

func savePACStateFile(m pacStateFile) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal PAC state: %w", err)
	}
	if err := os.MkdirAll(config.DataDir(), 0700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	path := pacStatePath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write PAC state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename PAC state: %w", err)
	}
	return nil
}

// savePACState captures the current PAC state for a service.
func savePACState(service string) error {
	stateMap, err := loadPACStateFile()
	if err != nil {
		if os.IsNotExist(err) {
			stateMap = make(pacStateFile)
		} else {
			return fmt.Errorf("load PAC state (file may be corrupted — inspect or delete %s): %w", pacStatePath(), err)
		}
	}

	pacURL, enabled, err := platform.GetAutoProxy(service)
	if err != nil {
		return fmt.Errorf("query current PAC state: %w", err)
	}

	// Idempotent: if we already have a snapshot and current PAC is ours, keep it.
	if _, exists := stateMap[service]; exists && enabled && isOurPAC(pacURL) {
		return nil
	}

	extra, err := platform.CaptureExtraState(service)
	if err != nil {
		return fmt.Errorf("capture platform state: %w", err)
	}

	stateMap[service] = pacSnapshot{
		OriginalURL: pacURL,
		WasEnabled:  enabled,
		Extra:       extra,
	}
	return savePACStateFile(stateMap)
}

// restorePACState restores the original PAC for a service from its snapshot.
// Order: PAC URL/enabled state first, platform extra state LAST.
func restorePACState(service string, snap pacSnapshot) error {
	// Verify ownership: fail if we can't confirm the current PAC is ours
	pacURL, enabled, err := platform.GetAutoProxy(service)
	if err != nil {
		return fmt.Errorf("verify PAC ownership: %w", err)
	}
	if enabled && !isOurPAC(pacURL) {
		return nil // PAC was changed by something else — don't touch it
	}

	// 1. Restore PAC URL / enabled state
	if snap.WasEnabled && snap.OriginalURL != "" {
		if err := platform.SetAutoProxy(service, snap.OriginalURL); err != nil {
			return fmt.Errorf("restore PAC URL: %w", err)
		}
	} else {
		if err := platform.ClearAutoProxy(service); err != nil {
			return fmt.Errorf("clear PAC: %w", err)
		}
	}

	// 2. Restore platform-specific state LAST (Linux mode, Windows AutoDetect)
	if err := platform.RestoreExtraState(service, snap.Extra); err != nil {
		return fmt.Errorf("restore platform state: %w", err)
	}

	return nil
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

	// 4. System PAC: check capability first, then save/set
	service, err := platform.DetectNetworkService()
	if err != nil {
		return fail(fmt.Errorf("detect network: %w", err))
	}

	pacURL := agentProxyPACURL()
	systemPACEnabled := true

	// Probe capability: if GetAutoProxy returns ErrPACNotSupported,
	// skip system PAC entirely and continue with CLI-only mode.
	_, _, probeErr := platform.GetAutoProxy(service)
	if probeErr != nil && errors.Is(probeErr, platform.ErrPACNotSupported) {
		systemPACEnabled = false
		fmt.Printf("  ⚠ System PAC not supported — CLI env vars only\n")
	}

	if systemPACEnabled {
		if err := savePACState(service); err != nil {
			return fail(fmt.Errorf("save PAC state (required for safe restore): %w", err))
		}

		// Register rollback BEFORE modifying system proxy
		undo = append(undo, func() {
			if m, err := loadPACStateFile(); err == nil {
				if snap, ok := m[service]; ok {
					restorePACState(service, snap)
				}
			}
		})

		if err := platform.SetAutoProxy(service, pacURL); err != nil {
			return fail(fmt.Errorf("set PAC proxy: %w", err))
		}
	}

	// 5. Write CLI env file
	if err := writeEnvFile(cfg); err != nil {
		return fail(fmt.Errorf("write env file: %w", err))
	}
	undo = append(undo, func() { os.Remove(config.EnvPath()) })

	// 6. Register auto-start
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

	// 1. Restore PAC state for all saved services
	stateMap, stateErr := loadPACStateFile()
	if stateErr == nil {
		for service, snap := range stateMap {
			if err := restorePACState(service, snap); err != nil {
				errs = append(errs, fmt.Sprintf("restore PAC on %s: %v", service, err))
			} else {
				delete(stateMap, service)
			}
		}
		if len(stateMap) == 0 {
			os.Remove(pacStatePath())
		} else {
			savePACStateFile(stateMap)
		}
	} else if !os.IsNotExist(stateErr) {
		// Corrupted state file — try to clear only our own PAC
		errs = append(errs, fmt.Sprintf("PAC state file error: %v", stateErr))
		clearOurPACOnly()
	} else {
		// No state file — only clear if PAC is ours
		clearOurPACOnly()
	}

	// 2. Stop PAC HTTP server daemon
	stopPACDaemon()

	// 3. Stop SSH tunnel (best-effort — don't let invalid config block cleanup)
	if cfg.Proxy.Tunnel {
		tunnel.Stop(cfg)
	}

	// 4. Remove auto-start
	if err := platform.UninstallAutoStart(); err != nil {
		errs = append(errs, fmt.Sprintf("autostart: %v", err))
	}

	// 5. Remove CLI env file
	if err := os.Remove(config.EnvPath()); err != nil && !os.IsNotExist(err) {
		errs = append(errs, fmt.Sprintf("remove env.sh: %v", err))
	}

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

// clearOurPACOnly clears the system PAC only if it currently points to agent-proxy.
func clearOurPACOnly() {
	service, err := platform.DetectNetworkService()
	if err != nil {
		return
	}
	pacURL, enabled, err := platform.GetAutoProxy(service)
	if err == nil && enabled && isOurPAC(pacURL) {
		platform.ClearAutoProxy(service)
	}
}

func pacPIDFile() string {
	return filepath.Join(config.DataDir(), "pac-server.pid")
}

func startPACDaemon() (bool, error) {
	if pac.ServerRunning() {
		return false, nil
	}

	// Port occupied but nonce mismatch: likely an old-version daemon.
	// Attempt safe takeover: stop old process, then start new one.
	if pac.PortOccupied() {
		stopPACDaemon() // stops by PID file or pattern match
		// Brief wait for port release
		time.Sleep(200 * time.Millisecond)
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
	for _, pattern := range []string{"serve-pac", "__pac-server"} {
		for _, pid := range platform.FindPIDsByPattern(pattern) {
			killIfPACServer(pid, pattern)
		}
	}
}

func killIfPACServer(pid int, patterns ...string) {
	args := platform.GetProcessArgs(pid)
	if args == "" {
		return
	}
	for _, p := range patterns {
		if strings.Contains(args, p) {
			if proc, err := os.FindProcess(pid); err == nil {
				proc.Kill()
			}
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
