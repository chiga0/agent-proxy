# Troubleshooting

## Common Issues

| Problem | Fix |
|---------|-----|
| `host not in project known_hosts` | `agent-proxy trust-host` |
| ECS Squid not loopback-only | `agent-proxy setup` |
| Chinese traffic going through proxy | `agent-proxy doctor` → add flagged domains to `no_proxy` |
| Codex/desktop app can't connect | Restart the app (it caches PAC at startup) |
| `go build` fails with proxy | `source ~/.config/agent-proxy/env.sh` (updates no_proxy) |
| macOS "developer cannot be verified" | `xattr -d com.apple.quarantine /usr/local/bin/agent-proxy` |
| Proxy env breaks `go install` | `unset https_proxy http_proxy && go install ...` |
| Config corrupted, need emergency off | `agent-proxy off` works even with broken config |

## Diagnostics Workflow

### 1. Quick health check

```bash
agent-proxy status
```

Runs 6 checks: tunnel, proxy forwarding, system PAC, PAC file, PAC server, and Squid binding.

### 2. Full diagnostics

```bash
agent-proxy doctor
```

Includes everything in `status` plus no_proxy coverage analysis. Identifies domestic domains that may be incorrectly routed through the proxy.

### 3. Benchmark connectivity

```bash
agent-proxy bench
```

Compares proxy vs direct latency for configured domains. Helps identify if the proxy path is degraded.

### 4. Trace the network path

```bash
agent-proxy trace <domain>
```

Shows the full network path from your machine through the ECS to the target domain.

## FAQ

### Why does my desktop app still not work after `agent-proxy on`?

Many Electron/desktop apps cache the system PAC at startup. Restart the application after enabling the proxy.

### How do I completely disable the proxy in an emergency?

```bash
agent-proxy off
```

This works even if the config file is corrupted or missing. It restores original PAC settings and stops all proxy processes.

### How do I check what traffic is going through the proxy?

```bash
agent-proxy stats
```

Shows request counts, bandwidth per domain, and flags any Chinese traffic that may be misrouted.
