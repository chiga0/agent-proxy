# Configuration

Configuration is stored at `~/.config/agent-proxy/config.yaml` with `0600` permissions.

## Full Reference

```yaml
proxy:
  host: 1.2.3.4            # ECS public IP address
  port: 18443              # Squid proxy port on ECS
  ssh_key: ~/.ssh/key.pem  # Path to SSH private key
  ssh_user: root           # SSH username on ECS
  tunnel: true             # Use SSH tunnel (recommended)

presets: [ai, dev, search, cloud, media]
custom_domains: []
no_proxy: [localhost, 127.0.0.1, .alibaba-inc.com, .baidu.com]
```

## Fields

### proxy

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | *(required)* | Public IP address of your ECS server |
| `port` | int | `18443` | Port Squid listens on (loopback only) |
| `ssh_key` | string | *(required)* | Path to SSH private key for ECS access |
| `ssh_user` | string | `root` | SSH username on the ECS |
| `tunnel` | bool | `true` | Use SSH tunnel mode (recommended). When `true`, Squid only listens on `127.0.0.1` on the ECS |

### presets

List of preset groups to enable. Available groups:

| Preset | Domains |
|--------|---------|
| `ai` | OpenAI, ChatGPT, Anthropic, Gemini, Copilot, Codex, Perplexity |
| `dev` | GitHub, StackOverflow, npm, PyPI, crates.io, Docker, HuggingFace |
| `search` | Google, DuckDuckGo, Bing, Wikipedia |
| `cloud` | AWS, GCP, Azure docs & consoles |
| `media` | YouTube, Twitter/X, Instagram, Facebook, Telegram |

### custom_domains

Additional domains to route through the proxy, beyond presets.

```yaml
custom_domains:
  - example.com
  - "*.internal-tool.io"
```

### no_proxy

Domains and addresses that should **never** go through the proxy. These are exported as the `no_proxy` environment variable.

```yaml
no_proxy:
  - localhost
  - 127.0.0.1
  - .alibaba-inc.com
  - .baidu.com
  - .aliyun.com
```

!!! warning "no_proxy wildcard behavior"
    Domain suffixes (`.example.com`) have the broadest cross-client compatibility. IP wildcards (`10.*`) behave differently across curl, Go, Python, Node, and Java. Prefer CIDR or explicit entries for IP ranges.

## Environment File

The setup generates `~/.config/agent-proxy/env.sh` which exports proxy environment variables:

```bash
export https_proxy=http://127.0.0.1:18443
export http_proxy=http://127.0.0.1:18443
export no_proxy=localhost,127.0.0.1,.alibaba-inc.com,...
```

Source this in your shell profile:

```bash
[ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"
```

## Validation

Validate your config without making changes:

```bash
agent-proxy config-validate
```
