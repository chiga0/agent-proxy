package pac

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/chiga0/agent-proxy/internal/config"
	"github.com/chiga0/agent-proxy/internal/ecs"
)

// statsCache avoids triggering an SSH connection on every /api/stats request.
var statsCache struct {
	mu      sync.Mutex
	body    []byte
	expires time.Time
}

const statsCacheTTL = 60 * time.Second

// registerDashboard adds the dashboard and API routes to the mux.
// These endpoints do NOT require the nonce header.
func registerDashboard(mux *http.ServeMux) {
	mux.HandleFunc("/dashboard", dashboardHandler)
	mux.HandleFunc("/api/status", apiStatusHandler)
	mux.HandleFunc("/api/stats", apiStatsHandler)
}

// statusResponse is the JSON shape for /api/status.
type statusResponse struct {
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	Tunnel         bool     `json:"tunnel"`
	FallbackHost   string   `json:"fallback_host,omitempty"`
	Presets        []string `json:"presets"`
	WhitelistCount int      `json:"whitelist_count"`
	NoProxyCount   int      `json:"noproxy_count"`
}

// domainTraffic is one entry in the /api/stats response.
type domainTraffic struct {
	Domain   string `json:"domain"`
	Requests int    `json:"requests"`
	Bytes    int64  `json:"bytes"`
}

// statsResponse is the JSON shape for /api/stats.
type statsResponse struct {
	Domains []domainTraffic `json:"domains"`
	Error   string          `json:"error,omitempty"`
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(dashboardHTML))
}

func apiStatusHandler(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, `{"error":"failed to load config"}`, http.StatusInternalServerError)
		return
	}

	resp := statusResponse{
		Host:           cfg.Proxy.Host,
		Port:           cfg.Proxy.Port,
		Tunnel:         cfg.Proxy.Tunnel,
		FallbackHost:   cfg.Proxy.FallbackHost,
		Presets:        cfg.Presets,
		WhitelistCount: len(cfg.EffectiveWhitelist()),
		NoProxyCount:   len(cfg.NoProxy),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func apiStatsHandler(w http.ResponseWriter, r *http.Request) {
	statsCache.mu.Lock()
	if statsCache.body != nil && time.Now().Before(statsCache.expires) {
		body := statsCache.body
		statsCache.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
		return
	}
	statsCache.mu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statsResponse{Error: "failed to load config"})
		return
	}

	logText, err := ecs.FetchRecentLogs(cfg, 200)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statsResponse{Error: "failed to fetch logs: " + err.Error()})
		return
	}

	entries := ecs.ParseLogLines(logText)
	agg := ecs.AggregateByDomain(entries)

	// Top 10
	if len(agg) > 10 {
		agg = agg[:10]
	}

	domains := make([]domainTraffic, 0, len(agg))
	for _, s := range agg {
		domains = append(domains, domainTraffic{
			Domain:   s.Domain,
			Requests: s.Requests,
			Bytes:    s.Bytes,
		})
	}

	body, _ := json.Marshal(statsResponse{Domains: domains})

	statsCache.mu.Lock()
	statsCache.body = body
	statsCache.expires = time.Now().Add(statsCacheTTL)
	statsCache.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// dashboardHTML is the self-contained single-page dashboard (inline CSS + JS).
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Agent Proxy Dashboard</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: 'SF Mono', 'Fira Code', 'JetBrains Mono', monospace;
  background: #0d1117;
  color: #c9d1d9;
  padding: 24px;
  line-height: 1.6;
}
h1 { color: #58a6ff; font-size: 1.4em; margin-bottom: 16px; }
h2 { color: #79c0ff; font-size: 1.1em; margin: 20px 0 10px; border-bottom: 1px solid #21262d; padding-bottom: 6px; }
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 16px; margin-bottom: 20px; }
.card {
  background: #161b22;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 16px;
}
.card .label { color: #8b949e; font-size: 0.8em; text-transform: uppercase; }
.card .value { color: #f0f6fc; font-size: 1.2em; margin-top: 4px; }
.badge {
  display: inline-block;
  background: #1f6feb33;
  color: #58a6ff;
  border: 1px solid #1f6feb;
  border-radius: 12px;
  padding: 2px 10px;
  font-size: 0.8em;
  margin: 2px;
}
table { width: 100%; border-collapse: collapse; font-size: 0.9em; }
th, td { text-align: left; padding: 8px 12px; border-bottom: 1px solid #21262d; }
th { color: #8b949e; font-weight: normal; font-size: 0.85em; text-transform: uppercase; }
td { color: #c9d1d9; }
tr:hover td { background: #161b2288; }
.error { color: #f85149; }
.loading { color: #8b949e; font-style: italic; }
.bytes { color: #7ee787; text-align: right; }
.req { color: #d2a8ff; text-align: right; }
#traffic-body .loading-row td { text-align: center; padding: 20px; }
</style>
</head>
<body>
<h1>&#x1f6e1; Agent Proxy Dashboard</h1>

<h2>Proxy Status</h2>
<div class="grid" id="status-grid">
  <div class="card"><div class="label">Loading...</div></div>
</div>

<h2>Whitelist &amp; No-Proxy</h2>
<div class="grid" id="wl-grid">
  <div class="card"><div class="label">Loading...</div></div>
</div>

<h2>Recent Traffic <small style="color:#8b949e">(top 10 domains from last 200 log lines)</small></h2>
<div class="card">
<table>
  <thead><tr><th>Domain</th><th class="req">Requests</th><th class="bytes">Bytes</th></tr></thead>
  <tbody id="traffic-body">
    <tr class="loading-row"><td colspan="3" class="loading">Fetching traffic data...</td></tr>
  </tbody>
</table>
</div>

<script>
function formatBytes(b) {
  if (b >= 1073741824) return (b/1073741824).toFixed(1) + ' GB';
  if (b >= 1048576) return (b/1048576).toFixed(1) + ' MB';
  if (b >= 1024) return (b/1024).toFixed(1) + ' KB';
  return b + ' B';
}

fetch('/api/status').then(r => r.json()).then(d => {
  document.getElementById('status-grid').innerHTML =
    '<div class="card"><div class="label">Host</div><div class="value">' + (d.host || '-') + '</div></div>' +
    '<div class="card"><div class="label">Port</div><div class="value">' + d.port + '</div></div>' +
    '<div class="card"><div class="label">Tunnel Mode</div><div class="value">' + (d.tunnel ? 'Enabled' : 'Disabled') + '</div></div>' +
    '<div class="card"><div class="label">Fallback Host</div><div class="value">' + (d.fallback_host || 'None') + '</div></div>';

  var presets = (d.presets || []).map(function(p) { return '<span class="badge">' + p + '</span>'; }).join(' ');
  document.getElementById('wl-grid').innerHTML =
    '<div class="card"><div class="label">Active Presets</div><div class="value">' + (presets || 'None') + '</div></div>' +
    '<div class="card"><div class="label">Whitelist Domains</div><div class="value">' + d.whitelist_count + '</div></div>' +
    '<div class="card"><div class="label">No-Proxy Entries</div><div class="value">' + d.noproxy_count + '</div></div>';
}).catch(function(e) {
  document.getElementById('status-grid').innerHTML = '<div class="card"><div class="label error">Error loading status</div></div>';
});

fetch('/api/stats').then(r => r.json()).then(d => {
  var tbody = document.getElementById('traffic-body');
  if (d.error) {
    tbody.innerHTML = '<tr><td colspan="3" class="error">' + d.error + '</td></tr>';
    return;
  }
  if (!d.domains || d.domains.length === 0) {
    tbody.innerHTML = '<tr><td colspan="3" class="loading">No traffic data available</td></tr>';
    return;
  }
  tbody.innerHTML = d.domains.map(function(row) {
    return '<tr><td>' + row.domain + '</td><td class="req">' + row.requests + '</td><td class="bytes">' + formatBytes(row.bytes) + '</td></tr>';
  }).join('');
}).catch(function(e) {
  document.getElementById('traffic-body').innerHTML = '<tr><td colspan="3" class="error">Failed to fetch stats</td></tr>';
});
</script>
</body>
</html>`
