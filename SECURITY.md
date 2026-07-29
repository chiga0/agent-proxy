# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.7.x   | :white_check_mark: |
| 0.6.x   | :white_check_mark: |
| < 0.6   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it by opening a **private** issue or emailing the maintainer directly. Do **not** open a public issue for security vulnerabilities.

We will acknowledge receipt within 48 hours and provide a timeline for a fix.

## Security Model

### SSH Tunnel Mode (Recommended)

- Squid listens on `127.0.0.1` only on the ECS — **no public data port exposed**
- All proxy traffic encrypted via SSH tunnel
- Access control via SSH key authentication
- SSH host key verified via project-specific `known_hosts` (`StrictHostKeyChecking=yes`)
- `init` displays SHA256 fingerprint for manual verification against ECS console

### Squid ACL (Deny-First)

- `deny !Safe_ports` — only ports 80, 443, 8443 allowed
- `deny CONNECT !SSL_ports` — CONNECT only to 443, 8443
- `deny to_localhost` — blocks 127.0.0.0/8, ::1
- `deny to_linklocal` — blocks 169.254.0.0/16, fe80::/10
- `deny to_rfc1918` — blocks 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7
- `deny to_metadata` — blocks 169.254.169.254 (AWS), 100.100.100.200 (Alibaba Cloud)
- `allow trusted_ip` → `deny all`

### Local Security

- Config file: `0600` permissions
- PAC HTTP server: binds `127.0.0.1` only, random nonce per start
- PAC state file: `0600`, atomic write, per-service snapshots
- PID/nonce files: `0600` permissions
- No Basic auth — removed entirely in v0.6.0

### Supply Chain

- Release archives: SHA-256 checksums verified by installer (fail-closed)
- Checksums signed with cosign (keyless OIDC)
- GitHub Actions pinned to commit SHA
- GoReleaser version pinned to `~> v2`

## What Is NOT a Security Boundary

- The PAC nonce prevents port-conflict misidentification, not same-user impersonation
- `no_proxy` wildcards (`10.*`) have inconsistent cross-client behavior
- SSH `accept-new` is no longer used; `StrictHostKeyChecking=yes` with project `known_hosts` is enforced
