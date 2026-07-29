# Commands

## Overview

| Command | Description |
|---------|-------------|
| `agent-proxy init` | Interactive first-time setup |
| `agent-proxy on` | Enable proxy (tunnel + PAC + env vars) |
| `agent-proxy off` | Disable proxy (restores original PAC) |
| `agent-proxy status` | Quick health check (6 checks) |
| `agent-proxy doctor` | Full diagnostics + no_proxy coverage analysis |
| `agent-proxy stats` | Traffic statistics: top domains, bandwidth, Chinese traffic % |
| `agent-proxy setup` | Deploy/redeploy Squid on ECS (idempotent) |
| `agent-proxy trust-host` | Verify and trust ECS host key (SSH fingerprint) |
| `agent-proxy ip` | Refresh Squid IP whitelist (direct mode only) |
| `agent-proxy bench` | Benchmark proxy vs direct latency |
| `agent-proxy trace` | Network path trace: local → ECS → target |
| `agent-proxy update` | Self-update to latest release |
| `agent-proxy whitelist add/rm` | Manage custom domains |
| `agent-proxy preset enable/disable` | Toggle preset groups |
| `agent-proxy config-validate` | Validate config file syntax and values |

---

## init

Interactive first-time setup. Walks through SSH key selection, Squid deployment, tunnel setup, PAC configuration, and verification.

```bash
agent-proxy init
```

## on

Enable the proxy: starts the SSH tunnel, PAC server, and exports environment variables.

```bash
agent-proxy on
```

## off

Disable the proxy: stops the tunnel, PAC server, and restores original system PAC settings. Works even with a corrupted config file.

```bash
agent-proxy off
```

## status

Quick health check with 6 checks (tunnel, proxy forwarding, PAC, env vars, etc.).

```bash
$ agent-proxy status
  ✓ SSH tunnel active
  ✓ Proxy forwarding
  ✓ System PAC configured
  ✓ PAC server running
  ✓ Env vars set
  ✓ All checks passed
```

## doctor

Full diagnostics including no_proxy coverage analysis. Identifies Chinese/domestic domains that may be incorrectly routed through the proxy.

```bash
$ agent-proxy doctor
  ✓ SSH tunnel (127.0.0.1:18443 → 1.2.3.4:18443)
  ✓ Proxy forwarding (exit IP: 1.2.3.4)
  ✓ System PAC (http://127.0.0.1:18080/proxy.pac)
  ✓ PAC file (61 domains)
  ✓ PAC HTTP server (127.0.0.1:18080)
  ✓ ECS Squid loopback-only
  ✓ no_proxy coverage
  ✓ Everything looks good!
```

## stats

Traffic statistics from Squid access logs: top domains by traffic, bandwidth usage, and percentage of Chinese traffic.

```bash
$ agent-proxy stats
  Requests: 500
  Total traffic: 42.3 MB

  Top 10 domains by traffic:
  Domain                                        Requests  Traffic
  chatgpt.com                                        312   38.1 MB
  api.openai.com                                      45    3.2 MB
  github.com                                          28    0.8 MB
  ...

  🇨 Chinese traffic: 3 requests, 12.1 KB (0% of total)
```

## setup

Deploy or redeploy Squid on your ECS. Idempotent — safe to run multiple times. Configures Squid with loopback-only binding and deny-first ACLs.

```bash
agent-proxy setup
```

## trust-host

Verify and trust the ECS SSH host key. Stores the fingerprint in a project-specific `known_hosts` file with `StrictHostKeyChecking=yes`.

```bash
agent-proxy trust-host
```

Displays the SHA256 fingerprint for manual verification against your ECS console.

## bench

Benchmark proxy vs direct latency for configured domains. Shows TTFB and total time for both paths.

```bash
$ agent-proxy bench
  Domain                    Mode       TTFB    Total     Status
  chatgpt.com               proxy      350ms    352ms       ✓
  chatgpt.com               direct       -        -         ✗
  github.com                proxy      320ms    330ms       ✓
  github.com                direct     235ms    240ms       ✓
```

## trace

Network path trace from your machine through the ECS to a target domain. Useful for diagnosing routing issues.

```bash
agent-proxy trace chatgpt.com
```

## config-validate

Validate the config file for syntax errors and invalid values without making any changes.

```bash
agent-proxy config-validate
```

## whitelist

Manage custom domains in the proxy whitelist.

```bash
agent-proxy whitelist add foo.com     # Add a domain
agent-proxy whitelist rm foo.com      # Remove a domain
agent-proxy whitelist ls              # List custom domains
```

## preset

Toggle preset domain groups on or off.

```bash
agent-proxy preset ls              # Show all presets and their status
agent-proxy preset disable cloud   # Disable the cloud preset
agent-proxy preset enable cloud    # Re-enable it
```

## update

Self-update to the latest release with SHA-256 verification.

```bash
agent-proxy update
```
