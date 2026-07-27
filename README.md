# agent-proxy

[![CI](https://github.com/chiga0/agent-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/chiga0/agent-proxy/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/chiga0/agent-proxy)](https://github.com/chiga0/agent-proxy/releases)

Domain-based selective proxy routing via an overseas server. Route AI services, developer tools, and search engines through a proxy while keeping everything else direct.

**50+ domains pre-configured. Zero config to start.**

## How It Works

```
Browser / Desktop App  ──PAC──▶  Whitelisted domain?  ──Yes──▶  Overseas Proxy  ──▶  Target
                                        │
                                       No
                                        │
                                        ▼
                                     Direct connection

CLI Tools  ──env vars──▶  https_proxy + no_proxy  ──▶  Same routing logic
```

- **PAC (Proxy Auto-Config)** for browsers and Electron apps — selective per-domain routing
- **Environment variables** (`https_proxy` + `no_proxy`) for CLI tools
- **Squid forward proxy** on your ECS with password auth + IP whitelist
- **Built-in PAC HTTP server** (pure Go, no external dependencies)

## Install

**China mirror (recommended for CN users):**

```bash
curl -fsSL https://agent-proxy.oss-cn-hangzhou.aliyuncs.com/install.sh | bash
```

**GitHub (international):**

```bash
curl -fsSL https://raw.githubusercontent.com/chiga0/agent-proxy/main/install.sh | bash
```

**Options:**

```bash
# Specify version / install directory / mirror
curl -fsSL ... | bash -s -- --version v0.3.1 --dir ~/.local/bin --mirror oss

# Via Go
GONOSUMDB=github.com/chiga0/agent-proxy go install github.com/chiga0/agent-proxy/cmd/agent-proxy@latest

# Build from source
git clone https://github.com/chiga0/agent-proxy.git
cd agent-proxy && make build && sudo cp bin/agent-proxy /usr/local/bin/
```

The installer auto-detects OS/arch and picks the fastest mirror (GitHub or OSS).

## Quick Start

```bash
# One command does everything:
# SSH check → Squid deploy → SSH tunnel → PAC + env → auto-start → verify
agent-proxy init
```

That's it. `init` walks you through server IP + SSH key, deploys Squid, sets up an encrypted SSH tunnel (bypasses GFW SNI filtering), enables system PAC, configures CLI env vars, and registers auto-start on boot.

Add to your `~/.zshrc` or `~/.bashrc` for auto-loading CLI env vars:

```bash
[ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"
```

## Commands

| Command | Description |
|---------|-------------|
| `agent-proxy on` | Enable proxy (PAC + env vars + PAC HTTP server) |
| `agent-proxy off` | Disable proxy |
| `agent-proxy status` | Quick health check |
| `agent-proxy doctor` | Full diagnostics (config + connectivity) |
| `agent-proxy init` | Interactive first-time setup |
| `agent-proxy setup` | Deploy Squid on ECS (idempotent) |
| `agent-proxy ip` | Refresh Squid IP whitelist (when your public IP changes) |
| `agent-proxy bench [domains...]` | Benchmark proxy vs direct latency |
| `agent-proxy trace [domain]` | Network path trace: local → ECS → target |
| `agent-proxy whitelist ls` | List effective whitelist (presets + custom) |
| `agent-proxy whitelist add <domain>` | Add custom domain |
| `agent-proxy whitelist rm <domain>` | Remove custom domain |
| `agent-proxy preset ls` | List preset groups with domains |
| `agent-proxy preset enable <name>` | Enable a preset group |
| `agent-proxy preset disable <name>` | Disable a preset group |

## Presets (Zero-Config)

All presets are **enabled by default**. You get 50+ domains out of the box:

| Preset | Domains | Examples |
|--------|---------|---------|
| `ai` | AI services | OpenAI, ChatGPT, Anthropic, Claude, Gemini, OpenRouter, Copilot, Codex, Mistral, Perplexity |
| `dev` | Developer tools | GitHub, StackOverflow, npm, PyPI, crates.io, Go, Rust, Docker, HuggingFace |
| `search` | Search engines | Google, DuckDuckGo, Bing, Wikipedia |
| `cloud` | Cloud providers | AWS, GCP, Azure docs & consoles |
| `media` | Video & social | YouTube, Twitter/X, Instagram, Facebook, Telegram |

Manage presets:

```bash
agent-proxy preset ls              # Show all presets and their domains
agent-proxy preset disable cloud   # Disable cloud preset
agent-proxy preset enable cloud    # Re-enable
```

Add domains beyond presets:

```bash
agent-proxy whitelist add ipinfo.io mysite.com
agent-proxy whitelist rm mysite.com
```

## Diagnostics

### Benchmark

Compare proxy vs direct latency:

```bash
$ agent-proxy bench
Benchmarking 4 domains × 3 runs (proxy vs direct)...

  Domain                    Mode       TTFB    Total     Runs   Status
  ---------------------------------------------------------------------------
  chatgpt.com               proxy      350ms    352ms        3        ✓
  chatgpt.com               direct       -        -         3        ✗
  openai.com                proxy      340ms    342ms        3        ✓
  openai.com                direct     1380ms   1428ms        3        ✓
  github.com                proxy      320ms    330ms        3        ✓
  github.com                direct     690ms    700ms        3        ✓
```

### Network Trace

Trace the full path from your machine to the target via the proxy:

```bash
$ agent-proxy trace chatgpt.com
=== Network Trace ===

--- DNS Resolution ---
  47.236.51.96              → 47.236.51.96     (5ms)
  chatgpt.com               → 104.18.32.47     (12ms)

--- Local → ECS (47.236.51.96) ---
   1  192.168.1.1              2.1ms   1.8ms   2.3ms
   2  10.0.0.1                 5.2ms   4.9ms   5.5ms
   ...
  12  47.236.51.96            82.1ms  81.5ms  83.0ms

--- ECS → chatgpt.com ---
   1  172.16.0.1               0.5ms   0.4ms   0.6ms
   ...
   4  104.18.32.47             2.1ms   1.9ms   2.3ms
```

## Configuration

Config file: `~/.config/agent-proxy/config.yaml` (auto-created with defaults)

```yaml
proxy:
  host: 1.2.3.4          # Your ECS IP
  port: 18443
  ssh_key: ~/.ssh/key.pem # SSH key for tunnel + setup
  ssh_user: root
  tunnel: true            # SSH tunnel (recommended for China users)
  # user: proxyuser       # Optional: only needed for direct mode without tunnel
  # password: your-pass

presets:                  # Enabled preset groups
  - ai
  - dev
  - search
  - cloud
  - media

custom_domains: []        # Extra domains beyond presets

no_proxy:                 # Domains/IPs that bypass proxy
  - localhost
  - 127.0.0.1
  - .alibaba-inc.com
  - .baidu.com
  # ... (see default config for full list)
```

## Platform Support

| Platform | System PAC proxy | CLI env vars | ECS setup |
|----------|-----------------|--------------|-----------|
| macOS    | ✅ `networksetup` | ✅ | ✅ |
| Linux    | ✅ GNOME/gsettings | ✅ | ✅ |
| Windows  | ✅ Registry (IE/Edge) | ✅ | ✅ |

## Requirements

- Go 1.22+ (for building from source)
- SSH access to your ECS (for `setup` and `ip` commands)
- No external runtime dependencies

## Security

- Proxy credentials stored in `config.yaml` with `0600` permissions
- Squid requires authentication for non-whitelisted IPs
- Your public IP is whitelisted on Squid for PAC/browser access (no auth prompt)
- PAC HTTP server binds to `127.0.0.1` only
- No credentials in the repository

## Troubleshooting

### macOS: "cannot be opened because the developer cannot be verified"

The binary is not Apple-signed. Remove the quarantine attribute:

```bash
xattr -d com.apple.quarantine /usr/local/bin/agent-proxy
```

The `install.sh` script does this automatically.

### `go install` fails with sum.golang.org 500

New modules may not be indexed by the Go checksum database yet. Skip verification:

```bash
GONOSUMDB=github.com/chiga0/agent-proxy go install github.com/chiga0/agent-proxy/cmd/agent-proxy@latest
```

Or use the `install.sh` script which downloads directly from GitHub Releases.

### Proxy env vars break `go build` / `go install`

Go module domains are in the default `no_proxy` list. If you have an older config, regenerate:

```bash
agent-proxy on   # regenerates env.sh with updated no_proxy
```

Or temporarily unset: `unset https_proxy http_proxy && go build ...`

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Please read our [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE)
