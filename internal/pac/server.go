package pac

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
)

var (
	serverMu sync.Mutex
	srv      *http.Server
)

func noncePath() string {
	return filepath.Join(config.DataDir(), "pac-nonce")
}

// generateNonce creates a random nonce and persists it to disk.
func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(b)
	os.MkdirAll(config.DataDir(), 0700)
	if err := os.WriteFile(noncePath(), []byte(nonce), 0600); err != nil {
		return "", err
	}
	return nonce, nil
}

// readNonce reads the persisted nonce.
func readNonce() string {
	data, err := os.ReadFile(noncePath())
	if err != nil {
		return ""
	}
	return string(data)
}

func ServerRunning() bool {
	nonce := readNonce()
	if nonce == "" {
		return false
	}
	client := &http.Client{
		Timeout:   500 * time.Millisecond,
		Transport: &http.Transport{Proxy: nil},
	}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/proxy.pac", config.PACPort))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200 && resp.Header.Get("X-Agent-Proxy") == nonce
}

// PortOccupied checks if something is listening on the PAC port.
func PortOccupied() bool {
	client := &http.Client{
		Timeout:   300 * time.Millisecond,
		Transport: &http.Transport{Proxy: nil},
	}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/proxy.pac", config.PACPort))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func pacHandler(nonce string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(config.PACPath())
		if err != nil {
			http.Error(w, "PAC not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Agent-Proxy", nonce)
		w.Write(data)
	}
}

func StartServer() error {
	serverMu.Lock()
	defer serverMu.Unlock()

	if srv != nil {
		return nil
	}

	nonce, err := generateNonce()
	if err != nil {
		return fmt.Errorf("generate PAC nonce: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy.pac", pacHandler(nonce))

	srv = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", config.PACPort),
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			serverMu.Lock()
			srv = nil
			serverMu.Unlock()
		}
	}()

	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		if ServerRunning() {
			return nil
		}
	}
	return fmt.Errorf("PAC HTTP server did not start within 1s")
}

func StopServer() {
	serverMu.Lock()
	defer serverMu.Unlock()

	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	srv = nil
	os.Remove(noncePath())
}

// ServeForeground runs the PAC HTTP server in the foreground (blocking).
// Used by the hidden "serve-pac" command for daemon mode.
// Nonce is written only after successful listener bind.
// A config watcher goroutine regenerates the PAC file when config.yaml changes.
func ServeForeground() error {
	addr := fmt.Sprintf("127.0.0.1:%d", config.PACPort)

	// Bind first to guarantee single instance
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("bind PAC port: %w", err)
	}

	// Generate and persist nonce only after successful bind
	nonce, err := generateNonce()
	if err != nil {
		ln.Close()
		return fmt.Errorf("generate PAC nonce: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy.pac", pacHandler(nonce))

	// Start config watcher for hot-reload
	go watchConfigAndReloadPAC()

	s := &http.Server{Handler: mux}
	return s.Serve(ln)
}

// watchConfigAndReloadPAC polls config.yaml mtime and regenerates the PAC file
// when the config changes. The PAC HTTP handler reads the file on each request,
// so the new PAC is served immediately without restart.
func watchConfigAndReloadPAC() {
	configPath := config.ConfigPath()
	var lastMod time.Time

	for {
		time.Sleep(5 * time.Second)

		info, err := os.Stat(configPath)
		if err != nil {
			continue
		}
		if info.ModTime().Equal(lastMod) {
			continue
		}
		lastMod = info.ModTime()

		// Config changed — reload and regenerate PAC + env.sh
		cfg, err := config.Load()
		if err != nil {
			continue
		}
		if err := Write(cfg); err != nil {
			continue
		}
		cfg.WriteEnvFile() // best-effort env.sh hot-reload
	}
}
