# Network Optimization & Bug Fix Design

**Branch:** `feat/network-optimization`
**Date:** 2026-07-28
**Status:** Draft

---

## 1. Problem Analysis

### 1.1 PAC Server Daemon Instability (P0)

**Symptom:** PAC HTTP server (port 18080) dies shortly after `agent-proxy on` exits.
Browser proxy silently stops working.

**Root Cause:** `startPACDaemon()` in `internal/proxy/manager.go` spawns the
PAC server via `exec.Command(self, "serve-pac")` without setting
`SysProcAttr.Setsid = true`. The child process shares the parent's session
and process group. When the parent's controlling terminal closes (or the
shell process that launched `agent-proxy on` exits), the kernel sends SIGHUP
to all processes in the session, killing the PAC server.

**Evidence:**
- Running `agent-proxy serve-pac &` directly from shell → survives indefinitely
- Running via `startPACDaemon()` → dies within seconds of parent exit
- LaunchAgent with `KeepAlive: true` works around the issue (launchd restarts it)

**Impact:** All users who run `agent-proxy on` and then close their terminal.

### 1.2 Upgrade Migration: Orphaned `__pac-server` (P0)

**Symptom:** After upgrading from v0.3.x to v0.4.x, `agent-proxy on` fails to
start the PAC server because port 18080 is held by an orphaned old process.

**Root Cause:** v0.3.x used `__pac-server` as the hidden command name; v0.4.x
renamed it to `serve-pac`. `stopPACDaemon()` only pgreps for `serve-pac`,
leaving old processes as orphans.

**Impact:** All users upgrading from v0.3.x.

### 1.3 `ServerRunning()` Proxy Interference (P1)

**Symptom:** `pac.ServerRunning()` returns false even when the PAC server is
alive and serving correctly.

**Root Cause:** The `http.Client` in `ServerRunning()` uses
`http.DefaultTransport` which has `Proxy: http.ProxyFromEnvironment`. If the
user's `NO_PROXY` doesn't include `127.0.0.1` (or the env vars are in an
unexpected state), the health check request loops through the proxy.

**Impact:** False negatives in `doctor` and `status` checks.

### 1.4 `checkSSH` Redundant in Tunnel Mode (P1)

**Symptom:** `agent-proxy doctor` shows `✗ SSH (22) unreachable` even when the
SSH tunnel is working perfectly.

**Root Cause:** `checkSSH()` uses Go's `net.DialTimeout` to connect to the
external IP on port 22. In tunnel mode, this check is redundant (the tunnel
check already proves SSH works) and misleading when it fails due to
environment-specific issues (firewalls, sandboxes, Go runtime quirks).

**Impact:** Confusing doctor output; users think something is broken.

### 1.5 SSH Tunnel Performance (P2)

**Measured baseline (Hangzhou → Singapore ECS → Cloudflare):**

| Metric | Value |
|--------|-------|
| SSH tunnel throughput | 3.0 MB/s |
| SSH handshake latency | ~1200ms |
| Tunnel curl TTFB (avg) | ~2.5s |
| ECS direct curl TTFB (avg) | ~0.94s |
| Tunnel overhead | ~1.5s |
| Tunnel TTFB outlier | 5.3s (TCP-over-TCP retransmission cascade) |
| ECS → Cloudflare RTT | 1.6ms |

**Key issues:**
1. **TCP-over-TCP:** SSH tunnel wraps TCP inside TCP. Inner TCP loss triggers
   retransmission; outer SSH TCP also sees the delay and retransmits, causing
   cascading delays (the 5.3s outlier).
2. **No SSH connection multiplexing:** Each `agent-proxy on` creates a fresh
   SSH connection. If it drops, all proxy traffic stalls until reconnect.
3. **Default cipher:** SSH negotiates the default cipher, which may not be
   optimal for ARM64 (Apple Silicon / AWS Graviton).
4. **Compression enabled by default:** HTTPS traffic is already compressed;
   SSH compression wastes CPU and adds latency.

---

## 2. Optimization Design

### 2.1 Fix: PAC Server `Setsid` Isolation

**File:** `internal/proxy/manager.go` — `startPACDaemon()`

Add platform-specific process group isolation:

```go
// On Unix (darwin, linux):
cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
```

This creates a new session for the child process, detaching it from the
parent's terminal. The PAC server will survive terminal close and shell exit.

For Windows, `Setsid` is not available; use `CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP`
via a build-tagged helper.

### 2.2 Fix: Migration Cleanup for Old `__pac-server`

**File:** `internal/proxy/manager.go` — `stopPACDaemon()`

Add fallback pgrep for the old command name:

```go
func stopPACDaemon() {
    // ... existing PID file logic ...

    // Fallback: pgrep both old and new command names
    for _, pattern := range []string{"serve-pac", "__pac-server"} {
        out, err := exec.Command("pgrep", "-f", pattern).Output()
        // ... kill matching PIDs ...
    }
}
```

### 2.3 Fix: `ServerRunning()` Bypass Proxy

**File:** `internal/pac/server.go` — `ServerRunning()`

```go
func ServerRunning() bool {
    client := &http.Client{
        Timeout: 500 * time.Millisecond,
        Transport: &http.Transport{Proxy: nil},
    }
    // ...
}
```

### 2.4 Fix: `checkSSH` Skip in Tunnel Mode

**File:** `internal/proxy/status.go` — `Status()`

When `cfg.Proxy.Tunnel` is true, replace the raw SSH port check with an
informational note:

```go
if cfg.Proxy.Tunnel {
    results = append(results,
        CheckResult{"SSH (22)", true, "via tunnel — skipped direct check"},
        checkTunnel(cfg),
    )
} else {
    results = append(results, checkSSH(cfg), checkPort(cfg))
}
```

### 2.5 Optimization: SSH Tunnel Hardening

**File:** `internal/tunnel/tunnel.go` — `Start()`

Add performance and reliability flags to the SSH command:

```go
args := []string{
    "-f", "-N",
    "-o", "StrictHostKeyChecking=accept-new",
    "-o", "ServerAliveInterval=30",      // was 60; faster dead-peer detection
    "-o", "ServerAliveCountMax=3",
    "-o", "ExitOnForwardFailure=yes",
    "-o", "Ciphers=aes128-gcm@openssh.com,aes256-gcm@openssh.com,chacha20-poly1305@openssh.com",
    "-o", "Compression=no",              // HTTPS already compressed
    "-o", "IPQoS=throughput",            // DSCP marking for QoS
    "-o", "TCPKeepAlive=yes",
}
```

**Rationale:**
- `aes128-gcm` first: hardware-accelerated on ARM64 (AES-NI / ARM CE),
  ~20% faster than ChaCha20 on supported hardware.
- `Compression=no`: avoids double-compression of already-compressed HTTPS
  payloads; saves CPU and reduces latency.
- `IPQoS=throughput`: sets DSCP CS0/AF21 marking; some ISP routers
  prioritize marked packets.
- `ServerAliveInterval=30`: detect dead connections faster (was 60s).

### 2.6 Optimization: Squid Connection Pool Tuning

**File:** `internal/ecs/deploy.go` — `writeSquidConfig()`

Add to the Squid config template:

```squid
# Connection pool tuning
server_persistent_connections on
client_persistent_connections on
persistent_request_timeout 60 seconds    # was 30; keep idle conns longer
pconn_timeout 2 minutes                  # was 1; reduce reconnect overhead
half_closed_clients off                  # avoid stale half-closed conns
read_timeout 5 minutes
connect_timeout 10 seconds

# DNS cache (reduce lookup latency)
positive_dns_ttl 1 hours
negative_dns_ttl 30 seconds

# File descriptor limit
max_filedescriptors 65536
```

### 2.7 Future: WireGuard Tunnel Option (Not in This PR)

WireGuard would eliminate TCP-over-TCP entirely (UDP encapsulation), reducing
latency variance by 30-50%. However, it requires:
- Kernel module or userspace (`wireguard-go`) on ECS
- Client-side WireGuard config
- Firewall rule changes (UDP port)

This is tracked as a separate feature request and not included in this PR.

---

## 3. Test Plan

| Test | Method |
|------|--------|
| PAC server survives parent exit | `agent-proxy on` → close shell → check port 18080 |
| Old `__pac-server` cleanup | Start old binary → `agent-proxy off` → verify port freed |
| `ServerRunning()` with proxy env | Set `http_proxy` without `NO_PROXY` → verify check passes |
| Tunnel mode doctor | `agent-proxy doctor` with `tunnel: true` → SSH check shows ✓ |
| SSH cipher negotiation | `ssh -vvv` → verify `aes128-gcm` selected |
| Squid config deploy | `agent-proxy setup` → verify new params in squid.conf |
| End-to-end proxy | `curl -x http://127.0.0.1:18443 https://ipinfo.io/ip` → Singapore IP |

---

## 4. Files Changed

| File | Change |
|------|--------|
| `internal/proxy/manager.go` | Setsid, migration cleanup |
| `internal/pac/server.go` | Proxy-free health check |
| `internal/proxy/status.go` | Skip SSH check in tunnel mode |
| `internal/tunnel/tunnel.go` | Cipher, compression, keepalive tuning |
| `internal/ecs/deploy.go` | Squid connection pool + DNS cache |
| `docs/known-issues.md` | This document (moved from root) |
