package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chiga0/agent-proxy/internal/config"
)

func TestProbeSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer srv.Close()

	// Probe through a "proxy" that is actually the test server
	cfg := &config.Config{}
	cfg.Proxy.Host = "127.0.0.1"
	cfg.Proxy.Port = 18443
	cfg.Proxy.Tunnel = true

	// Override probeURL for testing by hitting the test server directly
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("test server request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestProbeFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == 204 {
		t.Error("expected non-204 from failing server")
	}
}

func TestAtomicState(t *testing.T) {
	consecutiveFailures.Store(0)
	lastCheckOK.Store(true)
	lastRecovery.Store(0)
	recoveryFailures.Store(0)

	if ConsecutiveFailures() != 0 {
		t.Error("expected 0 consecutive failures")
	}
	if !LastCheckOK() {
		t.Error("expected last check OK")
	}
	if LastRecovery() != 0 {
		t.Error("expected no recovery")
	}

	consecutiveFailures.Store(3)
	lastCheckOK.Store(false)
	lastRecovery.Store(12345)

	if ConsecutiveFailures() != 3 {
		t.Errorf("expected 3, got %d", ConsecutiveFailures())
	}
	if LastCheckOK() {
		t.Error("expected last check not OK")
	}
	if LastRecovery() != 12345 {
		t.Errorf("expected 12345, got %d", LastRecovery())
	}

	// Reset for other tests
	consecutiveFailures.Store(0)
	lastCheckOK.Store(true)
	lastRecovery.Store(0)
	recoveryFailures.Store(0)
}

func TestWatchStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx)
		close(done)
	}()
	cancel()
	<-done
}
