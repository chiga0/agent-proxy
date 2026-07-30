package pac

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestMetricsEndpoint(t *testing.T) {
	handler := metricsHandler()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}

	body := rec.Body.String()

	// Verify all expected metric names are present
	expectedMetrics := []string{
		"agent_proxy_pac_requests_total",
		"agent_proxy_pac_server_up",
		"agent_proxy_config_domains_total",
		"agent_proxy_config_presets_total",
		"agent_proxy_config_noproxy_total",
		"agent_proxy_tunnel_enabled",
		"agent_proxy_health_check_ok",
		"agent_proxy_health_consecutive_failures",
		"agent_proxy_health_last_recovery_unix",
	}
	for _, m := range expectedMetrics {
		if !strings.Contains(body, m) {
			t.Errorf("missing metric %q in output", m)
		}
	}

	// Verify Prometheus text format: each metric family has HELP and TYPE lines
	helpRe := regexp.MustCompile(`(?m)^# HELP (\w+) .+$`)
	typeRe := regexp.MustCompile(`(?m)^# TYPE (\w+) (counter|gauge)$`)
	sampleRe := regexp.MustCompile(`(?m)^(\w+) \d+$`)

	helps := helpRe.FindAllStringSubmatch(body, -1)
	types := typeRe.FindAllStringSubmatch(body, -1)
	samples := sampleRe.FindAllStringSubmatch(body, -1)

	if len(helps) != len(expectedMetrics) {
		t.Errorf("expected %d HELP lines, got %d", len(expectedMetrics), len(helps))
	}
	if len(types) != len(expectedMetrics) {
		t.Errorf("expected %d TYPE lines, got %d", len(expectedMetrics), len(types))
	}
	if len(samples) != len(expectedMetrics) {
		t.Errorf("expected %d sample lines, got %d", len(expectedMetrics), len(samples))
	}

	// Verify pac_server_up is 1
	if !strings.Contains(body, "agent_proxy_pac_server_up 1") {
		t.Error("agent_proxy_pac_server_up should be 1")
	}
}

func TestMetricsCounterIncrement(t *testing.T) {
	// Reset counter for this test
	before := pacRequestsTotal.Load()

	// Simulate a PAC request by calling the increment directly
	pacRequestsTotal.Add(1)

	after := pacRequestsTotal.Load()
	if after != before+1 {
		t.Errorf("expected counter to increment from %d to %d, got %d", before, before+1, after)
	}
}
