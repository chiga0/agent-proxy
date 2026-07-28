package pac

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
)

var (
	serverMu sync.Mutex
	srv      *http.Server
)

func ServerRunning() bool {
	client := &http.Client{
		Timeout:   500 * time.Millisecond,
		Transport: &http.Transport{Proxy: nil},
	}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/proxy.pac", config.PACPort))
	if err != nil {
		return false
	}
	resp.Body.Close()
	// Verify this is our PAC server, not another HTTP service on the same port
	return resp.StatusCode == 200 && resp.Header.Get("X-Agent-Proxy") == "pac"
}

func StartServer() error {
	serverMu.Lock()
	defer serverMu.Unlock()

	if srv != nil {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy.pac", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(config.PACPath())
		if err != nil {
			http.Error(w, "PAC not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Agent-Proxy", "pac")
		w.Write(data)
	})

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
}

// ServeForeground runs the PAC HTTP server in the foreground (blocking).
// Used by the hidden "serve-pac" command for daemon mode.
func ServeForeground() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy.pac", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(config.PACPath())
		if err != nil {
			http.Error(w, "PAC not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Agent-Proxy", "pac")
		w.Write(data)
	})

	s := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", config.PACPort),
		Handler: mux,
	}
	return s.ListenAndServe()
}
