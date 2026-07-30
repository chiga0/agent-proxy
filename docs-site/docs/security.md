# Security

A deep dive into agent-proxy's security model: what it protects, how, and where the boundaries are.

## Supported Versions

| Version | Supported |
|---------|:---------:|
| 0.7.x   | ✅ |
| 0.6.x   | ✅ |
| < 0.6   | ❌ (Basic auth removed, known_hosts not enforced) |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it by opening a **private** issue or emailing the maintainer directly. Do **not** open a public issue for security vulnerabilities.

We will acknowledge receipt within 48 hours and provide a timeline for a fix.

## Threat Model

### What we protect against

| Threat | Mitigation |
|--------|------------|
| **Eavesdropping on proxy traffic** | SSH tunnel encrypts all data between your machine and the ECS |
| **Unauthorized use of your Squid** | Deny-first ACL: only loopback (tunnel) or your IP (direct) is allowed; final rule is `deny all` |
| **SSRF via Squid** | ACLs block localhost, RFC 1918, link-local, and cloud metadata endpoints |
| **MITM on SSH connection** | `StrictHostKeyChecking=yes` with project-specific `known_hosts`; fingerprint shown for manual verification |
| **Tampered release binaries** | SHA-256 checksums verified by installer (fail-closed); checksums signed with cosign |
| **CI supply chain attacks** | GitHub Actions pinned to commit SHA; GoReleaser version pinned |
| **Config file disclosure** | `0600` permissions on config, env.sh, known_hosts, PID files, PAC state |
| **Legacy credential exposure** | Basic auth (`proxy.user`/`proxy.password`) removed entirely in v0.6.0; fields no longer exist in config schema |

### What we do NOT protect against

| Non-goal | Reason |
|----------|--------|
| **Same-user impersonation** | Any process running as your user can read the config, use the SSH key, and access the proxy. This is a fundamental OS trust boundary, not something agent-proxy can solve. |
| **Compromised ECS** | If your ECS is rooted, all bets are off. Squid ACLs are a defense-in-depth layer, not a sandbox. |
| **DNS-level blocking** | If your ISP blocks DNS resolution for target domains, the proxy can't help (the PAC file uses `dnsDomainIs` on the host string, but DNS must resolve for the connection). |
| **Non-HTTP protocols** | agent-proxy is an HTTP/HTTPS forward proxy. SSH, gaming, VoIP, and other TCP/UDP protocols are not proxied. |
| **GFW protocol fingerprinting** | SSH traffic can be fingerprinted. agent-proxy does not implement obfuscation (e.g., obfs4, VLESS). |

---

## SSH Tunnel Security

### Why tunnel mode?

In direct mode, Squid listens on a public port and is protected only by an IP whitelist ACL. This has weaknesses:

- Your public IP changes (mobile networks, DHCP) → whitelist goes stale
- An attacker who spoofs your IP can use your Squid
- Traffic between your machine and the ECS is unencrypted (Squid receives plaintext HTTP CONNECT)

Tunnel mode eliminates all three: Squid binds to `127.0.0.1` only, and all traffic flows through an encrypted SSH channel.

### Cipher selection

The SSH tunnel is started with an explicit cipher list:

```
Ciphers=aes128-gcm@openssh.com,aes256-gcm@openssh.com,chacha20-poly1305@openssh.com
```

These are all AEAD ciphers (authenticated encryption with associated data), which provide both confidentiality and integrity. Legacy CBC-mode ciphers are excluded.

### ControlMaster multiplexing

The tunnel uses SSH ControlMaster for connection reuse:

```
ControlMaster=auto
ControlPath=~/.config/agent-proxy/ssh-ctrl-%r@%h:%p
ControlPersist=600
```

- **`auto`**: The first SSH connection creates a master socket; subsequent connections reuse it (no re-authentication, faster setup).
- **`ControlPersist=600`**: The master socket stays alive for 10 minutes after the last session closes, so brief disconnects don't require a full reconnect.
- The control socket lives in `~/.config/agent-proxy/` with restricted permissions.

### Keepalive and failure detection

```
ServerAliveInterval=30
ServerAliveCountMax=3
TCPKeepAlive=yes
ExitOnForwardFailure=yes
```

- A keepalive probe is sent every 30 seconds. After 3 missed responses (90 seconds), the tunnel is declared dead.
- `ExitOnForwardFailure=yes` ensures the SSH process exits immediately if the port forward can't be established (rather than hanging with a broken tunnel).

### Host key verification

agent-proxy enforces `StrictHostKeyChecking=yes` with a **project-specific** `known_hosts` file:

```
UserKnownHostsFile=~/.config/agent-proxy/known_hosts
```

This is separate from `~/.ssh/known_hosts` to avoid conflicts with other tools and to give agent-proxy full control over its trust store.

**Key lifecycle:**

1. `agent-proxy init` or `trust-host` fetches the host key via `ssh-keyscan`
2. The SHA256 fingerprint is displayed for **manual verification** against the ECS console
3. The user must type `yes` to accept (not just press Enter)
4. The key is stored with `0600` permissions
5. All subsequent SSH connections (tunnel, deploy, trace) verify against this key

The old `accept-new` strategy (auto-trust on first connection) was removed in the v0.6.0 security audit.

### BatchMode

```
BatchMode=yes
```

Disables interactive password prompts. If key authentication fails, the connection fails immediately rather than hanging waiting for input. This is critical for background tunnel management.

---

## Squid ACL Deep-Dive

The Squid configuration generated by `agent-proxy setup` follows a **deny-first** model. Here's every rule explained:

### Listen address

```squid
# Tunnel mode (recommended)
http_port 127.0.0.1:18443

# Direct mode (not recommended)
http_port 18443
```

In tunnel mode, Squid is unreachable from the network — only processes on the ECS loopback (i.e., the SSH tunnel endpoint) can connect.

### Trusted sources

```squid
acl trusted_ip src 127.0.0.1           # tunnel mode
acl trusted_ip src 127.0.0.1 YOUR_IP   # direct mode
```

Only the SSH tunnel endpoint (loopback) or your verified public IP is allowed to use the proxy.

### Port restrictions

```squid
acl Safe_ports port 80 443 8443
acl SSL_ports port 443 8443
acl CONNECT method CONNECT

http_access deny !Safe_ports
http_access deny CONNECT !SSL_ports
```

- Only ports 80 (HTTP), 443 (HTTPS), and 8443 (alt-HTTPS) can be reached through the proxy.
- The CONNECT method (used for HTTPS tunneling) is restricted to SSL ports only.
- This prevents using your Squid as an open relay to arbitrary ports (e.g., SMTP, SSH).

### Destination blocking

```squid
acl to_localhost dst 127.0.0.0/8 ::1
acl to_linklocal dst 169.254.0.0/16 fe80::/10
acl to_rfc1918 dst 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fc00::/7
acl to_metadata dst 169.254.169.254 100.100.100.200

http_access deny to_localhost
http_access deny to_linklocal
http_access deny to_rfc1918
http_access deny to_metadata
```

These rules prevent **Server-Side Request Forgery (SSRF)** through your Squid:

| ACL | Blocks | Why |
|-----|--------|-----|
| `to_localhost` | `127.0.0.0/8`, `::1` | Prevents access to services running on the ECS itself (databases, admin panels) |
| `to_linklocal` | `169.254.0.0/16`, `fe80::/10` | Blocks link-local addresses used by cloud metadata and zeroconf |
| `to_rfc1918` | `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `fc00::/7` | Blocks private network ranges (VPC internal services) |
| `to_metadata` | `169.254.169.254`, `100.100.100.200` | Blocks AWS and Alibaba Cloud instance metadata endpoints (which expose IAM credentials) |

### Final rules

```squid
http_access allow trusted_ip
http_access deny all
```

After all deny rules, only trusted sources are allowed. The final `deny all` is the safety net — anything not explicitly allowed is blocked.

### Privacy headers

```squid
forwarded_for off
request_header_access Via deny all
```

- `forwarded_for off`: Removes the `X-Forwarded-For` header so target sites don't see your real IP.
- `Via deny all`: Removes the `Via` header so target sites can't identify Squid.

### Other settings

```squid
cache deny all                    # No caching (privacy: don't store responses on ECS)
dns_nameservers 8.8.8.8 8.8.4.4  # Use Google DNS (avoid ISP DNS hijacking)
max_filedescriptors 65536         # Handle many concurrent connections
collapsed_forwarding on           # Deduplicate concurrent requests to the same URL
```

---

## Local Security

### File permissions

| File | Permissions | Why |
|------|:-----------:|-----|
| `config.yaml` | `0600` | Contains ECS IP, SSH key path |
| `env.sh` | `0600` | Contains proxy settings |
| `known_hosts` | `0600` | SSH trust store |
| `pac-state.json` | `0600` | System PAC state |
| `pac-server.pid` | `0600` | Process tracking |
| `pac-nonce` | `0600` | Server identity |
| Config directory | `0700` | Parent directory |

### PAC server nonce

The PAC HTTP server generates a random 16-byte nonce on each start and includes it as the `X-Agent-Proxy` response header. The `status` and `doctor` commands verify this nonce to confirm they're talking to the **current** agent-proxy PAC server, not another process that happens to occupy port 18080.

!!! info "Not an authentication mechanism"
    The nonce prevents port-conflict misidentification (e.g., detecting a stale server from a previous version). It does **not** prevent same-user impersonation — any process running as your user can read the nonce file.

### PAC state machine

When `agent-proxy on` modifies the system PAC, it first saves a snapshot of the original state to `pac-state.json`:

```json
{
  "Wi-Fi": {
    "original_url": "",
    "was_enabled": false
  }
}
```

When `agent-proxy off` runs, it restores the original state **only if** the current PAC still points to agent-proxy. If another tool has changed the PAC in the meantime, agent-proxy leaves it alone.

Writes to `pac-state.json` are atomic (temp file → rename) to prevent corruption.

---

## Supply Chain Security

### Release verification

1. **SHA-256 checksums**: Every release archive has a `.sha256` checksum file. The install script verifies the checksum after download and **fails closed** — a mismatch aborts the installation.

2. **cosign signing**: Checksum files are signed with [cosign](https://github.com/sigstore/cosign) using keyless OIDC (GitHub Actions identity). This ties the checksums to a specific GitHub Actions run.

3. **Version-pinned install script**: `agent-proxy update` downloads the install script from the **current version's release tag**, not the mutable `main` branch. This prevents a compromised `main` from serving a malicious install script to updaters.

### CI/CD hardening

- **GitHub Actions pinned to commit SHA**: All workflow actions (checkout, setup-go, etc.) are pinned to specific commit hashes, not mutable tags like `@v4`.
- **GoReleaser version pinned**: The release tool is pinned to `~> v2` to avoid unexpected breaking changes.
- **SLSA provenance**: Release artifacts include SLSA provenance metadata for verifiable build reproducibility.

---

## What Is NOT a Security Boundary

To set clear expectations:

1. **The PAC nonce** prevents port-conflict misidentification, not same-user impersonation. Any process running as your user can read `~/.config/agent-proxy/pac-nonce`.

2. **`no_proxy` wildcards** (`10.*`) have inconsistent cross-client behavior. Node.js and Java ignore IP wildcards entirely. This is a compatibility issue, not a security vulnerability, but it means some traffic may unexpectedly go through the proxy.

3. **SSH `accept-new` is no longer used.** Versions prior to v0.6.0 used `StrictHostKeyChecking=accept-new`, which auto-trusts the host key on first connection. This was replaced with `StrictHostKeyChecking=yes` + project `known_hosts` + manual fingerprint verification.

4. **Squid is not a sandbox.** The ACLs are defense-in-depth. If an attacker gains SSH access to your ECS, they can modify the Squid config, read logs, and access anything on the server.

5. **No traffic obfuscation.** SSH has a well-known protocol fingerprint. Deep packet inspection (DPI) can identify and throttle SSH traffic. agent-proxy does not implement obfuscation layers (obfs4, VLESS, Trojan).
