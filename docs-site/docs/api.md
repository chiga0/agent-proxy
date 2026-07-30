# API Reference

The PAC HTTP server (running on `127.0.0.1:18080`) exposes several HTTP endpoints for monitoring, management, and integration.

All endpoints are served by the PAC daemon started via `agent-proxy on`. The server binds to **loopback only** — it is not accessible from the network.

## Endpoints Overview

| Endpoint | Method | Content-Type | Auth | Description |
|----------|--------|-------------|------|-------------|
| `/proxy.pac` | GET | `application/x-ns-proxy-autoconfig` | Nonce header | PAC file for browser proxy routing |
| `/dashboard` | GET | `text/html` | None | Web management panel |
| `/api/status` | GET | `application/json` | None | Proxy configuration summary |
| `/api/stats` | GET | `application/json` | None | Traffic statistics from Squid logs |
| `/metrics` | GET | `text/plain` (Prometheus) | None | Prometheus metrics |

---

## /dashboard

A self-contained single-page web dashboard with inline CSS and JavaScript. No external dependencies.

```bash
# Open in browser
open http://127.0.0.1:18080/dashboard    # macOS
xdg-open http://127.0.0.1:18080/dashboard    # Linux
```

**What it shows:**

- **Proxy Status**: Host, port, tunnel mode, fallback host
- **Whitelist & No-Proxy**: Active presets (as badges), domain count, no_proxy count
- **Recent Traffic**: Top 10 domains by traffic from the last 200 Squid access log lines

The dashboard fetches data from `/api/status` and `/api/stats` on page load. It uses a dark theme (GitHub-style) with a monospace font.

**Response headers:**

```
Content-Type: text/html; charset=utf-8
Cache-Control: no-cache
```

---

## /api/status

Returns the current proxy configuration as JSON.

```bash
curl -s http://127.0.0.1:18080/api/status | python3 -m json.tool
```

**Response:**

```json
{
  "host": "1.2.3.4",
  "port": 18443,
  "tunnel": true,
  "fallback_host": "5.6.7.8",
  "presets": ["ai", "dev", "search", "cloud", "media"],
  "whitelist_count": 61,
  "noproxy_count": 32
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `host` | string | ECS public IP or hostname |
| `port` | int | Squid port on ECS |
| `tunnel` | bool | Whether SSH tunnel mode is enabled |
| `fallback_host` | string | Fallback ECS IP (omitted if not configured) |
| `presets` | string[] | List of enabled preset group names |
| `whitelist_count` | int | Total number of whitelisted domains (presets + custom) |
| `noproxy_count` | int | Number of no_proxy entries |

**Error response (config load failure):**

```
HTTP 500
{"error":"failed to load config"}
```

---

## /api/stats

Returns traffic statistics parsed from the last 200 lines of the Squid access log on the ECS. Requires SSH connectivity to the ECS.

```bash
curl -s http://127.0.0.1:18080/api/stats | python3 -m json.tool
```

**Response:**

```json
{
  "domains": [
    {
      "domain": "chatgpt.com",
      "requests": 156,
      "bytes": 45230080
    },
    {
      "domain": "api.openai.com",
      "requests": 89,
      "bytes": 12582912
    },
    {
      "domain": "github.com",
      "requests": 34,
      "bytes": 4194304
    }
  ]
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `domains` | array | Top 10 domains by traffic volume |
| `domains[].domain` | string | The requested domain |
| `domains[].requests` | int | Number of requests in the log sample |
| `domains[].bytes` | int | Total bytes transferred |
| `error` | string | Error message (present only on failure) |

**Error responses:**

```json
{"domains": [], "error": "failed to load config"}
{"domains": [], "error": "failed to fetch logs: ssh: connect to host 1.2.3.4 port 22: Connection timed out"}
```

!!! info "Data source"
    Stats are parsed from `/var/log/squid/access.log` on the ECS via SSH. The endpoint fetches the last 200 lines, so it reflects recent traffic only. For a larger sample, use `agent-proxy stats -n 5000` from the CLI.

---

## /metrics

Prometheus text format metrics for monitoring integration. No authentication required.

```bash
curl -s http://127.0.0.1:18080/metrics
```

**Response headers:**

```
Content-Type: text/plain; version=0.0.4; charset=utf-8
```

### Metric Reference

#### agent_proxy_pac_requests_total

| Property | Value |
|----------|-------|
| Type | Counter |
| Labels | None |
| Description | Total number of PAC file requests served since the PAC server started |

```
# HELP agent_proxy_pac_requests_total Total PAC file requests served.
# TYPE agent_proxy_pac_requests_total counter
agent_proxy_pac_requests_total 1234
```

Increments on every GET to `/proxy.pac`. Useful for monitoring how frequently browsers re-fetch the PAC file.

---

#### agent_proxy_pac_server_up

| Property | Value |
|----------|-------|
| Type | Gauge |
| Labels | None |
| Description | Whether the PAC server is running (always 1 when scraped, since the server must be up to respond) |

```
# HELP agent_proxy_pac_server_up Whether the PAC server is running.
# TYPE agent_proxy_pac_server_up gauge
agent_proxy_pac_server_up 1
```

This metric is always `1` when successfully scraped (the server must be running to serve metrics). Its value is in **scrape failure detection**: if Prometheus can't reach this metric, the PAC server is down.

---

#### agent_proxy_config_domains_total

| Property | Value |
|----------|-------|
| Type | Gauge |
| Labels | None |
| Description | Number of whitelisted domains (presets + custom) |

```
# HELP agent_proxy_config_domains_total Number of whitelisted domains.
# TYPE agent_proxy_config_domains_total gauge
agent_proxy_config_domains_total 61
```

Reflects the current `config.yaml` state. Changes when presets are toggled or custom domains are added/removed.

---

#### agent_proxy_config_presets_total

| Property | Value |
|----------|-------|
| Type | Gauge |
| Labels | None |
| Description | Number of enabled preset groups |

```
# HELP agent_proxy_config_presets_total Number of enabled presets.
# TYPE agent_proxy_config_presets_total gauge
agent_proxy_config_presets_total 5
```

Range: 0–5 (one for each preset group: ai, dev, search, cloud, media).

---

#### agent_proxy_config_noproxy_total

| Property | Value |
|----------|-------|
| Type | Gauge |
| Labels | None |
| Description | Number of no_proxy entries |

```
# HELP agent_proxy_config_noproxy_total Number of no_proxy entries.
# TYPE agent_proxy_config_noproxy_total gauge
agent_proxy_config_noproxy_total 32
```

A sudden drop to 0 may indicate a config file issue.

---

#### agent_proxy_tunnel_enabled

| Property | Value |
|----------|-------|
| Type | Gauge |
| Labels | None |
| Description | Whether SSH tunnel mode is enabled (1 = yes, 0 = no) |

```
# HELP agent_proxy_tunnel_enabled Whether tunnel mode is enabled.
# TYPE agent_proxy_tunnel_enabled gauge
agent_proxy_tunnel_enabled 1
```

Useful for alerting if the proxy accidentally falls back to direct mode.

---

### Prometheus scrape config

```yaml
scrape_configs:
  - job_name: 'agent-proxy'
    scrape_interval: 30s
    static_configs:
      - targets: ['127.0.0.1:18080']
    metrics_path: /metrics
```

See [Tutorial 3: Monitoring with Prometheus + Grafana](tutorials.md#tutorial-3-monitoring-with-prometheus-grafana) for a complete setup guide.

---

## /proxy.pac

The PAC (Proxy Auto-Configuration) file served to browsers and system proxy settings.

```bash
curl -s http://127.0.0.1:18080/proxy.pac
```

**Response headers:**

```
Content-Type: application/x-ns-proxy-autoconfig
Cache-Control: no-cache
X-Agent-Proxy: <nonce>
```

**Response body (truncated):**

```javascript
// Auto-generated by agent-proxy - 2026-07-29 10:30:00
// Effective whitelist: 61 domains (presets: [ai dev search cloud media])
function FindProxyForURL(url, host) {
    if (dnsDomainIs(host, ".chatgpt.com") || host == "chatgpt.com") return "PROXY 127.0.0.1:18443";
    if (dnsDomainIs(host, ".openai.com") || host == "openai.com") return "PROXY 127.0.0.1:18443";
    if (dnsDomainIs(host, ".anthropic.com") || host == "anthropic.com") return "PROXY 127.0.0.1:18443";
    // ... 58 more domains ...
    return "DIRECT";
}
```

**Behavior:**

- Each whitelisted domain matches both the bare domain (`host == "example.com"`) and all subdomains (`dnsDomainIs(host, ".example.com")`)
- Non-matching domains return `"DIRECT"` (no proxy)
- The proxy address is `127.0.0.1:<port>` (the local SSH tunnel endpoint in tunnel mode, or the ECS IP in direct mode)
- The file is regenerated when `config.yaml` changes (hot-reload via config watcher, polled every 5 seconds)

**The `X-Agent-Proxy` header** contains a random nonce generated at server start. The `status` and `doctor` commands use this to verify they're communicating with the current agent-proxy server (not a stale process on the same port).

---

## Server Lifecycle

The PAC server is managed as a background daemon:

1. **Start**: `agent-proxy on` launches `agent-proxy serve-pac` as a detached process, writes the PID to `pac-server.pid`
2. **Identity**: A random nonce is generated and persisted to `pac-nonce` after successful listener bind
3. **Hot-reload**: In foreground mode (`serve-pac`), a watcher polls `config.yaml` mtime every 5 seconds and regenerates the PAC file + `env.sh` on change
4. **Stop**: `agent-proxy off` kills the daemon via PID file, with fallback to process pattern matching
5. **Takeover**: If port 18080 is occupied by a stale server (nonce mismatch), `agent-proxy on` kills the old process and starts a new one
