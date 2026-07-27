# agent-proxy

Domain-based selective proxy routing via an overseas server. Route whitelisted domains (AI services, GitHub, etc.) through a proxy while keeping everything else direct.

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
- **Local PAC HTTP server** for Chrome compatibility (Chrome doesn't support `file://` PAC)

## Install

```bash
go install github.com/chiga0/agent-proxy/cmd/agent-proxy@latest
```

Or build from source:

```bash
git clone https://github.com/chiga0/agent-proxy.git
cd agent-proxy
make build
sudo cp bin/agent-proxy /usr/local/bin/
```

## Quick Start

### 1. Configure

```bash
# Generate default config at ~/.config/agent-proxy/config.yaml
agent-proxy status

# Edit config — set your ECS host, port, credentials
vim ~/.config/agent-proxy/config.yaml
```

Example config:

```yaml
proxy:
  host: 1.2.3.4
  port: 18443
  user: proxyuser
  password: your-password
  ssh_key: ~/.ssh/your-key.pem
  ssh_user: root

whitelist:
  - chatgpt.com
  - openai.com
  - anthropic.com
  - github.com
  # ... add more domains

no_proxy:
  - localhost
  - 127.0.0.1
  - .alibaba-inc.com
  - .baidu.com
  # ... internal/domestic domains to exclude
```

### 2. Deploy proxy on ECS

```bash
agent-proxy setup
```

This installs and configures Squid on your ECS (idempotent — safe to re-run).

### 3. Enable proxy

```bash
agent-proxy on
```

This will:
- Generate PAC file from whitelist
- Start local PAC HTTP server (for Chrome)
- Set macOS system PAC proxy
- Write CLI environment variables (`~/.config/agent-proxy/env.sh`)

Add this line to your `~/.zshrc` or `~/.bashrc` for auto-loading:

```bash
[ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"
```

### 4. Verify

```bash
agent-proxy status
```

## Usage

```
agent-proxy on                        # Enable proxy
agent-proxy off                       # Disable proxy
agent-proxy status                    # Health check
agent-proxy whitelist add gemini.google.com google.com
agent-proxy whitelist rm github.com
agent-proxy whitelist ls              # List domains
agent-proxy setup                     # Deploy Squid on ECS
agent-proxy ip refresh                # Update IP whitelist when your public IP changes
```

## Architecture

```
~/.config/agent-proxy/
├── config.yaml          # Main config (whitelist, credentials, proxy settings)
├── proxy.pac            # Generated PAC file
└── env.sh               # Generated CLI env vars (sourced by shell)

Local:
  127.0.0.1:18080        # PAC HTTP server (python3 http.server)

ECS:
  Squid :18443           # Forward proxy with auth + IP whitelist
```

## Security

- Proxy credentials stored in `config.yaml` with `0600` permissions
- Squid requires authentication for non-whitelisted IPs
- Your public IP is whitelisted on Squid for PAC/browser access (no auth prompt)
- No credentials in the repository

## Platform Support

| Platform | System PAC proxy | CLI env vars | ECS setup |
|----------|-----------------|--------------|-----------|
| macOS    | ✅ `networksetup` | ✅ | ✅ |
| Linux    | ✅ GNOME/gsettings | ✅ | ✅ |
| Windows  | ✅ Registry (IE/Edge) | ✅ | ✅ |

## Requirements

- Go 1.22+ (for building from source)
- SSH access to your ECS (for `setup` and `ip refresh`)
- No external runtime dependencies (PAC server is built-in)

## License

MIT
