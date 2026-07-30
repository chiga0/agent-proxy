package pac

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/health"
)

// pacRequestsTotal counts the total number of PAC file requests served.
var pacRequestsTotal atomic.Int64

// metricsHandler serves Prometheus text format metrics without requiring the nonce header.
func metricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		// Load config for gauge values (best-effort; use zero values on error)
		var domainsTotal, presetsTotal, noproxyTotal int
		var tunnelEnabled int
		cfg, err := config.Load()
		if err == nil {
			domainsTotal = len(cfg.EffectiveWhitelist())
			presetsTotal = len(cfg.Presets)
			noproxyTotal = len(cfg.NoProxy)
			if cfg.Proxy.Tunnel {
				tunnelEnabled = 1
			}
		}

		fmt.Fprintf(w, "# HELP agent_proxy_pac_requests_total Total PAC file requests served.\n")
		fmt.Fprintf(w, "# TYPE agent_proxy_pac_requests_total counter\n")
		fmt.Fprintf(w, "agent_proxy_pac_requests_total %d\n", pacRequestsTotal.Load())
		fmt.Fprintf(w, "\n")

		fmt.Fprintf(w, "# HELP agent_proxy_pac_server_up Whether the PAC server is running.\n")
		fmt.Fprintf(w, "# TYPE agent_proxy_pac_server_up gauge\n")
		fmt.Fprintf(w, "agent_proxy_pac_server_up 1\n")
		fmt.Fprintf(w, "\n")

		fmt.Fprintf(w, "# HELP agent_proxy_config_domains_total Number of whitelisted domains.\n")
		fmt.Fprintf(w, "# TYPE agent_proxy_config_domains_total gauge\n")
		fmt.Fprintf(w, "agent_proxy_config_domains_total %d\n", domainsTotal)
		fmt.Fprintf(w, "\n")

		fmt.Fprintf(w, "# HELP agent_proxy_config_presets_total Number of enabled presets.\n")
		fmt.Fprintf(w, "# TYPE agent_proxy_config_presets_total gauge\n")
		fmt.Fprintf(w, "agent_proxy_config_presets_total %d\n", presetsTotal)
		fmt.Fprintf(w, "\n")

		fmt.Fprintf(w, "# HELP agent_proxy_config_noproxy_total Number of no_proxy entries.\n")
		fmt.Fprintf(w, "# TYPE agent_proxy_config_noproxy_total gauge\n")
		fmt.Fprintf(w, "agent_proxy_config_noproxy_total %d\n", noproxyTotal)
		fmt.Fprintf(w, "\n")

		fmt.Fprintf(w, "# HELP agent_proxy_tunnel_enabled Whether tunnel mode is enabled.\n")
		fmt.Fprintf(w, "# TYPE agent_proxy_tunnel_enabled gauge\n")
		fmt.Fprintf(w, "agent_proxy_tunnel_enabled %d\n", tunnelEnabled)
		fmt.Fprintf(w, "\n")

		healthOK := 0
		if health.LastCheckOK() {
			healthOK = 1
		}
		fmt.Fprintf(w, "# HELP agent_proxy_health_check_ok Whether the last health check passed.\n")
		fmt.Fprintf(w, "# TYPE agent_proxy_health_check_ok gauge\n")
		fmt.Fprintf(w, "agent_proxy_health_check_ok %d\n", healthOK)
		fmt.Fprintf(w, "\n")

		fmt.Fprintf(w, "# HELP agent_proxy_health_consecutive_failures Consecutive health check failures.\n")
		fmt.Fprintf(w, "# TYPE agent_proxy_health_consecutive_failures gauge\n")
		fmt.Fprintf(w, "agent_proxy_health_consecutive_failures %d\n", health.ConsecutiveFailures())
		fmt.Fprintf(w, "\n")

		fmt.Fprintf(w, "# HELP agent_proxy_health_last_recovery_unix Unix timestamp of last auto-recovery (0 = never).\n")
		fmt.Fprintf(w, "# TYPE agent_proxy_health_last_recovery_unix gauge\n")
		fmt.Fprintf(w, "agent_proxy_health_last_recovery_unix %d\n", health.LastRecovery())
	}
}
