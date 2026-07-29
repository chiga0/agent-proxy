# agent-proxy

[![CI](https://github.com/chiga0/agent-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/chiga0/agent-proxy/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/chiga0/agent-proxy)](https://github.com/chiga0/agent-proxy/releases)

**Domain-based selective proxy CLI.** Route AI services, developer tools, and search engines through your overseas server — everything else stays direct.

60+ domains pre-configured. One command to set up. SSH-encrypted tunnel. Zero runtime dependencies.

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
│  Your ECS (Singapore / Tokyo / etc.)                           │
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

## Install

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
curl -fsSL ... | bash -s -- --version v0.6.1

# Go install
GONOSUMDB=github.com/chiga0/agent-proxy go install github.com/chiga0/agent-proxy/cmd/agent-proxy@latest

# Build from source
git clone https://github.com/chiga0/agent-proxy.git
cd agent-proxy && make build
```
</details>

## Quick Start

```bash
agent-proxy init          # Interactive setup (SSH key → deploy → tunnel → PAC → verify)
```

Add to `~/.zshrc` or `~/.bashrc`:

```bash
[ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"
```

Done. New terminals auto-load proxy env vars. Browsers use system PAC automatically.

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

# After upgrading from < v0.6.0:
agent-proxy trust-host      # Migrate SSH host key to project known_hosts
agent-proxy setup           # Rewrite ECS Squid to loopback-only
```

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

## Requirements

- Go 1.24+ (build from source only)
- SSH access to your ECS
- No external runtime dependencies

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
