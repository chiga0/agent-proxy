# agent-proxy

[![CI](https://github.com/chiga0/agent-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/chiga0/agent-proxy/actions/workflows/ci.yml)
[![Docs](https://img.shields.io/badge/Docs-chiga0.github.io-blue?logo=gitbook)](https://chiga0.github.io/agent-proxy/)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/chiga0/agent-proxy)](https://github.com/chiga0/agent-proxy/releases)

**Domain-based selective proxy CLI.** Route AI services, developer tools, and search engines through your overseas server — everything else stays direct.

60+ domains pre-configured. One command to set up. SSH-encrypted tunnel. Zero runtime dependencies.

📖 **Full documentation: [chiga0.github.io/agent-proxy](https://chiga0.github.io/agent-proxy/)**

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Your Machine                                                   │
│                                                                 │
│  Browser / Electron ──PAC──▶ 127.0.0.1:18080 (PAC server)      │
│                                    │                            │
│  CLI / SDK ──env vars──▶ 127.0.0.1:18443 (SSH tunnel)          │
│                                    │                            │
└────────────────────────────────────┼────────────────────────────┘
                                     │ SSH (encrypted)
                                     ▼
┌────────────────────────────────────────────────────────────────┐
│  Your ECS (Tokyo / Singapore)                                  │
│                                                                │
│  127.0.0.1:18443 ──▶ Squid (loopback only) ──▶ Target Site    │
│                                                                │
│  • Deny-first ACL (no public data port)                       │
│  • Blocks localhost / RFC1918 / cloud metadata                │
└────────────────────────────────────────────────────────────────┘
```

**Two routing paths, one proxy:**

| Path | Mechanism | Scope |
|------|-----------|-------|
| Browser / Desktop | System PAC → `127.0.0.1:18080` | Only whitelisted domains |
| CLI / SDK | `https_proxy` + `no_proxy` env vars | All HTTP(S) except `no_proxy` |

## ECS Requirements

Before setting up, ensure your overseas server meets these requirements:

| Item | Requirement | Notes |
|------|------------|-------|
| **OS** | Ubuntu 18.04+, Debian 10+, CentOS 7+, Alpine 3.12+ | Needs `apt`, `yum`, or `apk` |
| **Init system** | systemd, OpenRC, or SysVinit | Auto-detected during deploy |
| **CPU / RAM** | 1 vCPU / 512 MB minimum | Squid is lightweight; SSH tunnel is the main overhead |
| **Disk** | 1 GB free | Squid package + logs |
| **Network** | Public IP (EIP) or NAT gateway with outbound internet | Required for package install and proxying |
| **SSH** | Port 22 accessible from your machine | Key-based auth recommended |
| **Security group** | Inbound: TCP 22 (SSH). Tunnel mode needs nothing else | Direct mode also needs TCP 18443 from your IP |
| **DNS** | ECS can resolve public domains | Uses ECS's `/etc/resolv.conf` nameservers |

> **Recommended regions:** Tokyo (preferred), Singapore — low latency to both China and major AI/dev services.

> **Tunnel mode (recommended):** Only SSH port 22 needs to be open. Squid listens on `127.0.0.1` only — zero public data ports.

## Setup

### Install agent-proxy

```bash
# Auto-detect OS/arch, pick fastest mirror, verify SHA-256
curl -fsSL https://raw.githubusercontent.com/chiga0/agent-proxy/main/install.sh | bash
```

<details>
<summary>Other install methods</summary>

```bash
# China mirror (faster for CN users)
curl -fsSL https://agent-proxy.oss-cn-hangzhou.aliyuncs.com/install.sh | bash

# Specific version
curl -fsSL ... | bash -s -- --version v0.7.3

# Go install
GONOSUMDB=github.com/chiga0/agent-proxy go install github.com/chiga0/agent-proxy/cmd/agent-proxy@latest

# Build from source
git clone https://github.com/chiga0/agent-proxy.git
cd agent-proxy && make build
```
</details>

### Option A: Automated (one command)

```bash
agent-proxy init
```

The interactive wizard handles everything:

```
1. Enter server IP, SSH user, key path, port
2. Verify SSH host key fingerprint (compare with ECS console)
3. Test SSH connectivity
4. Check ECS internet access
5. Install and configure Squid (auto-detects OS/package manager)
6. Start SSH tunnel
7. Generate PAC file + set system proxy
8. Write env.sh for CLI tools
9. Install auto-start service (LaunchAgent / systemd)
10. Verify connectivity (google.com, github.com)
```

Add to `~/.zshrc` or `~/.bashrc`:

```bash
[ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"
```

### Option B: Manual step-by-step

If you prefer to understand each step or need to troubleshoot:

**Step 1 — Install agent-proxy locally:**

```bash
curl -fsSL https://raw.githubusercontent.com/chiga0/agent-proxy/main/install.sh | bash
```

**Step 2 — Prepare your ECS:**

```bash
# SSH into your ECS
ssh root@YOUR_ECS_IP

# Install Squid
apt update && apt install -y squid    # Ubuntu/Debian
# yum install -y squid               # CentOS/RHEL
# apk add squid                      # Alpine

# Enable on boot
systemctl enable squid               # systemd
# rc-update add squid default        # OpenRC (Alpine)

# Verify Squid is running
systemctl status squid
```

**Step 3 — Configure Squid:**

```bash
agent-proxy trust-host               # Verify and trust ECS SSH host key
agent-proxy setup                    # Generate and deploy Squid config
```

This writes a deny-first Squid config with SSRF protection (blocks localhost, RFC1918, cloud metadata), privacy headers stripped, and loopback-only listening (tunnel mode).

**Step 4 — Enable proxy:**

```bash
agent-proxy on                       # Start tunnel + PAC + env vars
```

**Step 5 — Add to shell profile:**

```bash
echo '[ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"' >> ~/.zshrc
```

**Step 6 — Verify:**

```bash
agent-proxy status                   # Quick health check
agent-proxy bench                    # Latency comparison
curl -I https://api.openai.com       # Should return HTTP response
```

## Commands

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

## Presets

60+ domains across 5 groups, all enabled by default:

| Preset | Examples |
|--------|---------|
| `ai` | OpenAI, ChatGPT, Anthropic, Gemini, Copilot, Codex, Perplexity |
| `dev` | GitHub, StackOverflow, npm, PyPI, crates.io, Docker, HuggingFace |
| `search` | Google, DuckDuckGo, Bing, Wikipedia |
| `cloud` | AWS, GCP, Azure docs & consoles |
| `media` | YouTube, Twitter/X, Instagram, Facebook, Telegram |

```bash
agent-proxy preset ls              # Show all presets
agent-proxy preset disable cloud   # Disable a group
agent-proxy whitelist add foo.com  # Add custom domain
```

## Diagnostics

### Stats — see what's going through your proxy

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

### Doctor — full health + no_proxy audit

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

### Benchmark

```bash
$ agent-proxy bench
  Domain                    Mode       TTFB    Total     Status
  chatgpt.com               proxy      350ms    352ms       ✓
  chatgpt.com               direct       -        -         ✗
  github.com                proxy      320ms    330ms       ✓
  github.com                direct     235ms    240ms       ✓
```

## Configuration

`~/.config/agent-proxy/config.yaml`:

```yaml
proxy:
  host: 1.2.3.4
  port: 18443
  ssh_key: ~/.ssh/key.pem
  ssh_user: root
  tunnel: true              # SSH tunnel (recommended)

presets: [ai, dev, search, cloud, media]
custom_domains: []
no_proxy: [localhost, 127.0.0.1, .alibaba-inc.com, .baidu.com, ...]
```

> **`no_proxy` note:** Domain suffixes (`.example.com`) have the broadest cross-client compatibility. IP wildcards (`10.*`) behave differently across curl, Go, Python, Node, and Java.

## Upgrading

```bash
agent-proxy update          # Self-update with SHA-256 verification
agent-proxy trust-host      # Verify and trust the ECS SSH host key
agent-proxy setup           # Rewrite ECS Squid config (e.g., after mode change)
```

## Performance

Measured on a typical corporate network (China → Singapore ECS):

| Metric | Direct | Via Proxy | Notes |
|--------|--------|-----------|-------|
| TTFB (github.com) | 210ms | 213ms | Proxy adds < 5ms overhead |
| TTFB (openai.com) | 420ms | 370ms | **Proxy is faster** — better ECS routing |
| TTFB (google.com) | 461ms | 381ms | **Proxy is faster** — ECS has lower latency |
| Throughput | 0.28 MB/s | 0.25 MB/s | Bottleneck is local bandwidth, not proxy |

The SSH tunnel adds negligible overhead for HTTPS traffic (already encrypted). The main latency factor is network RTT, not protocol processing. For services hosted near the ECS region, the proxy can be **faster** than direct due to better routing.

## Security

- **SSH tunnel mode**: Squid listens on `127.0.0.1` only — no public data port exposed
- **SSH host key**: project-specific `known_hosts` with `StrictHostKeyChecking=yes` (no TOFU)
- **Deny-first ACL**: blocks unsafe ports, non-SSL CONNECT, localhost, RFC1918, cloud metadata (AWS + Alibaba)
- **PAC server**: random nonce per start prevents port-conflict misidentification
- **Release integrity**: SHA-256 checksums verified by installer; GitHub Actions pinned to commit SHA
- **Config permissions**: `0600`; PAC server binds `127.0.0.1` only

## Platform Support

| Platform | System PAC | CLI env | Auto-start | SSH tunnel |
|----------|-----------|---------|------------|------------|
| macOS    | ✅ | ✅ | ✅ LaunchAgent | ✅ |
| Linux    | ✅ GNOME | ✅ | ✅ systemd user | ✅ |
| Windows  | ✅ Registry | ✅ | ✅ Scheduled Task | ✅ |

Linux without GNOME: CLI-only mode (env vars work, system PAC skipped automatically).

## Troubleshooting

### Setup issues

| Problem | Cause | Fix |
|---------|-------|-----|
| `SSH connection failed` | Wrong IP/user/key, or port 22 blocked | Verify `ssh root@IP` works manually; check security group |
| `host not in project known_hosts` | First connection or host key changed | `agent-proxy trust-host` |
| `ECS cannot reach the internet` | No EIP / NAT gateway / outbound rules | Check ECS network config in cloud console |
| `unsupported package manager` | Non-standard Linux distro | Install Squid manually, then `agent-proxy setup` |
| `squid restart failed` | Config error or init system mismatch | `agent-proxy setup` retries with rollback; check `ssh root@IP 'journalctl -u squid -n 20'` |
| `SSH key not found` | Wrong path in config | Check `proxy.ssh_key` in `~/.config/agent-proxy/config.yaml` |
| macOS "developer cannot be verified" | Gatekeeper quarantine | `xattr -d com.apple.quarantine /usr/local/bin/agent-proxy` |

### Runtime issues

| Problem | Cause | Fix |
|---------|-------|-----|
| Proxy suddenly stops working | SSH tunnel dropped | `agent-proxy on` (auto-restart if autostart enabled) |
| Browser works but CLI doesn't | env vars not loaded | `source ~/.config/agent-proxy/env.sh` |
| `agent-proxy status` shows PAC server not running | PAC daemon crashed | `agent-proxy on` restarts it; health auto-recovery inactive until then |
| Chinese sites slow / going through proxy | Missing `no_proxy` entries | `agent-proxy doctor --fix` auto-detects and adds them |
| ECS rebooted, proxy dead | Squid not enabled on boot | `agent-proxy setup` (now runs `systemctl enable squid`) |
| Direct mode: sudden 403 errors | ISP rotated your public IP | `agent-proxy ip` refreshes the Squid IP whitelist |
| `npm install` fails with proxy error | npm ignores env vars | `source env.sh` runs `npm config set proxy`; or `npm config delete proxy` to undo |
| Codex / desktop app can't connect | App cached old PAC at startup | Restart the app |
| `go build` fails | Go proxy settings conflict | `unset https_proxy http_proxy && go build ...` |
| Tunnel port 18443 already in use | Another process on same port | Change `proxy.tunnel_local_port` in config.yaml |

### Diagnostic commands

```bash
agent-proxy status          # 6-point health check
agent-proxy doctor          # Full diagnostics + no_proxy audit + SNI detection
agent-proxy doctor --fix    # Auto-fix no_proxy coverage issues
agent-proxy bench           # Latency: proxy vs direct per domain
agent-proxy trace           # Network path: local → ECS → target
agent-proxy stats           # Traffic stats: top domains, bandwidth
```

### Emergency recovery

```bash
# Config corrupted — emergency off (works without valid config)
agent-proxy off

# Nuclear option — kill everything and restart
agent-proxy off
pkill -f "ssh.*-L.*18443"
pkill -f "agent-proxy serve-pac"
agent-proxy on

# Full redeploy (reinstalls Squid config on ECS)
agent-proxy setup
agent-proxy on
```

## Requirements

- Go 1.24+ (build from source only)
- SSH access to your ECS
- No external runtime dependencies

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
