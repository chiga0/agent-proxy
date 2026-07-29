package pac

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardReturnsHTML(t *testing.T) {
	mux := http.NewServeMux()
	registerDashboard(mux)

	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %q", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("dashboard HTML missing DOCTYPE")
	}
	if !strings.Contains(body, "Agent Proxy Dashboard") {
		t.Error("dashboard HTML missing title")
	}
	if !strings.Contains(body, "/api/status") {
		t.Error("dashboard HTML missing /api/status fetch")
	}
	if !strings.Contains(body, "/api/stats") {
		t.Error("dashboard HTML missing /api/stats fetch")
	}
}

func TestAPIStatusReturnsValidJSON(t *testing.T) {
	mux := http.NewServeMux()
	registerDashboard(mux)

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("expected application/json content type, got %q", ct)
	}

	var status statusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	// Verify expected fields exist (port should be non-zero from default config)
	if status.Port == 0 {
		t.Error("expected non-zero port in status response")
	}
	if status.Presets == nil {
		t.Error("expected presets to be non-nil")
	}
}
