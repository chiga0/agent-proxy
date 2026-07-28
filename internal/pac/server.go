package pac

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
func ServeForeground() error {
	nonce, err := generateNonce()
	if err != nil {
		return fmt.Errorf("generate PAC nonce: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy.pac", pacHandler(nonce))

	s := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", config.PACPort),
		Handler: mux,
	}
	return s.ListenAndServe()
}
