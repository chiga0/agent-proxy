# Configuration

Complete reference for `config.yaml` — every field, type, default, and validation rule.

## File Location

| Platform | Path |
|----------|------|
| macOS / Linux | `~/.config/agent-proxy/config.yaml` |
| Windows | `%USERPROFILE%\.config\agent-proxy\config.yaml` |

The file is created automatically with defaults on first run. Permissions are set to `0600` (owner read/write only). Writes are atomic: temp file → rename.

All other data files live in the same directory:

| File | Purpose |
|------|---------|
| `config.yaml` | Main configuration |
| `proxy.pac` | Generated PAC file |
| `env.sh` | Generated shell environment exports |
| `known_hosts` | Project-specific SSH host keys |
| `pac-state.json` | Snapshot of original system PAC (for restore) |
| `pac-server.pid` | PAC daemon PID |
| `pac-nonce` | PAC server identity nonce |
| `ssh-ctrl-*` | SSH ControlMaster sockets |

## Full Reference

```yaml
proxy:
  host: 1.2.3.4                  # ECS public IP or hostname (required)
  port: 18443                    # Squid port on ECS (default: 18443)
  ssh_key: ~/.ssh/id_ed25519     # SSH private key path
  ssh_user: root                 # SSH username (default: root)
  tunnel: true                   # SSH tunnel mode (default: false, recommended: true)
  tunnel_local_port: 18443       # Local port for tunnel endpoint (default: same as port)

  # Multi-ECS failover (optional)
  fallback_host: 5.6.7.8         # Fallback ECS IP
  fallback_ssh_key: ~/.ssh/backup_key  # Fallback SSH key (default: same as ssh_key)
  fallback_ssh_user: root        # Fallback SSH user (default: same as ssh_user)

presets: [ai, dev, search, cloud, media]

custom_domains:
  - example.com
  - my-internal-tool.io

no_proxy:
  - localhost
  - 127.0.0.1
  - ::1
  - .alibaba-inc.com
  - .baidu.com
```

## Field Reference

### proxy

| Field | Type | Default | Required | Description |
|-------|------|---------|:--------:|-------------|
| `host` | string | — | ✅ | Public IP or hostname of your ECS server. Must not contain `://` or spaces. |
| `port` | int | `18443` | — | Port Squid listens on. Range: 1–65535. In tunnel mode, this is the remote port; clients connect to `tunnel_local_port` (or this port if unset). |
| `ssh_key` | string | — | Conditional | Path to SSH private key. Required when `tunnel: true` unless `SSH_AUTH_SOCK` is set (ssh-agent). `~` is expanded. |
| `ssh_user` | string | `root` | — | SSH username on the ECS. |
| `tunnel` | bool | `false` | — | Enable SSH tunnel mode. When `true`, Squid listens on `127.0.0.1` only on the ECS, and all proxy traffic is encrypted via SSH. **Strongly recommended.** |
| `tunnel_local_port` | int | same as `port` | — | Local port the SSH tunnel binds to. Useful when the default port conflicts with another service. |
| `fallback_host` | string | — | — | IP or hostname of a fallback ECS for automatic failover. Used when the primary host is unreachable during tunnel startup. |
| `fallback_ssh_key` | string | same as `ssh_key` | — | SSH key for the fallback host. Defaults to the primary key. |
| `fallback_ssh_user` | string | same as `ssh_user` | — | SSH user for the fallback host. Defaults to the primary user. |

**Validation rules:**

- `host` must be non-empty for mutating commands (`on`, `setup`, `ip`, etc.)
- `port` must be 1–65535
- `ssh_key` is required when `tunnel: true` and `SSH_AUTH_SOCK` is not set
- All preset names must be valid
- All custom domains must match `^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`

### presets

A list of preset group names to enable. All five are enabled by default.

| Preset | Description | Domain count |
|--------|-------------|:------------:|
| `ai` | AI services (OpenAI, Anthropic, Google AI, OpenRouter, Copilot) | 18 |
| `dev` | Developer tools (GitHub, StackOverflow, package registries) | 18 |
| `search` | Search engines and knowledge bases | 16 |
| `cloud` | Cloud provider docs and consoles (AWS, GCP, Azure) | 8 |
| `media` | Video and social media (YouTube, Twitter/X, Instagram) | 14 |

**Full domain list by preset:**

=== "ai"

    | Domain | Service |
    |--------|---------|
    | `chatgpt.com` | ChatGPT |
    | `openai.com` | OpenAI API |
    | `oaistatic.com` | OpenAI static assets |
    | `oaiusercontent.com` | OpenAI user content |
    | `anthropic.com` | Anthropic API |
    | `claude.ai` | Claude |
    | `gemini.google.com` | Google Gemini |
    | `gemini.gstatic.com` | Gemini static assets |
    | `clients6.google.com` | Google API client |
    | `googleapis.com` | Google APIs |
    | `generativelanguage.googleapis.com` | Generative Language API |
    | `ai.google.dev` | Google AI Studio |
    | `openrouter.ai` | OpenRouter |
    | `copilot.github.com` | GitHub Copilot |
    | `githubcopilot.com` | GitHub Copilot |
    | `mistral.ai` | Mistral AI |
    | `perplexity.ai` | Perplexity |

=== "dev"

    | Domain | Service |
    |--------|---------|
    | `github.com` | GitHub |
    | `githubusercontent.com` | GitHub raw content |
    | `github.io` | GitHub Pages |
    | `githubassets.com` | GitHub assets |
    | `stackoverflow.com` | Stack Overflow |
    | `stackexchange.com` | Stack Exchange |
    | `npmjs.com` | npm registry |
    | `registry.npmjs.org` | npm registry API |
    | `pypi.org` | Python PyPI |
    | `files.pythonhosted.org` | PyPI file hosting |
    | `crates.io` | Rust crates |
    | `rubygems.org` | Ruby gems |
    | `go.dev` | Go documentation |
    | `pkg.go.dev` | Go package index |
    | `docs.rs` | Rust documentation |
    | `hub.docker.com` | Docker Hub |
    | `docker.io` | Docker registry |
    | `huggingface.co` | Hugging Face |

=== "search"

    | Domain | Service |
    |--------|---------|
    | `mail.google.com` | Gmail |
    | `docs.google.com` | Google Docs |
    | `drive.google.com` | Google Drive |
    | `photos.google.com` | Google Photos |
    | `scholar.google.com` | Google Scholar |
    | `maps.google.com` | Google Maps |
    | `translate.google.com` | Google Translate |
    | `calendar.google.com` | Google Calendar |
    | `contacts.google.com` | Google Contacts |
    | `keep.google.com` | Google Keep |
    | `classroom.google.com` | Google Classroom |
    | `sites.google.com` | Google Sites |
    | `ggpht.com` | YouTube thumbnails |
    | `duckduckgo.com` | DuckDuckGo |
    | `bing.com` | Bing |
    | `wikipedia.org` | Wikipedia |
    | `en.wikipedia.org` | English Wikipedia |

=== "cloud"

    | Domain | Service |
    |--------|---------|
    | `aws.amazon.com` | AWS |
    | `docs.aws.amazon.com` | AWS documentation |
    | `console.aws.amazon.com` | AWS console |
    | `cloud.google.com` | Google Cloud |
    | `console.cloud.google.com` | GCP console |
    | `cloud.console.aliyun.com` | Alibaba Cloud console |
    | `portal.azure.com` | Azure portal |
    | `learn.microsoft.com` | Microsoft Learn |

=== "media"

    | Domain | Service |
    |--------|---------|
    | `youtube.com` | YouTube |
    | `googlevideo.com` | YouTube video CDN |
    | `ytimg.com` | YouTube images |
    | `youtu.be` | YouTube short URLs |
    | `twitter.com` | Twitter |
    | `x.com` | X (Twitter) |
    | `twimg.com` | Twitter images |
    | `t.co` | Twitter link shortener |
    | `instagram.com` | Instagram |
    | `cdninstagram.com` | Instagram CDN |
    | `facebook.com` | Facebook |
    | `fbcdn.net` | Facebook CDN |
    | `telegram.org` | Telegram |
    | `t.me` | Telegram links |

### custom_domains

Additional domains to route through the proxy, beyond presets. Managed via `agent-proxy whitelist add/rm`.

```yaml
custom_domains:
  - example.com
  - my-internal-tool.io
```

Domains are validated against a regex: lowercase alphanumeric labels separated by dots, with a TLD of at least 2 characters. Wildcards (`*.example.com`) are **not** supported in the domain list — the PAC file automatically matches subdomains via `dnsDomainIs`.

### no_proxy

Domains and addresses that should **never** go through the proxy. Exported as the `no_proxy` environment variable for CLI tools.

**Default entries (set by `init`):**

```yaml
no_proxy:
  # Loopback and private IPs
  - localhost
  - 127.0.0.1
  - "::1"
  - "10.*"
  - "172.16.*"    # through 172.31.*
  - "192.168.*"

  # Alibaba ecosystem
  - .alibaba-inc.com
  - .aliyun.com
  - .aliyuncs.com
  - .aliyun-inc.com
  - .aliyunportal.com
  - .taobao.org
  - .antgroup.com
  - .alipay.com
  - .dingtalk.com

  # Other domestic services
  - .baidu.com
  - .qq.com
  - .tencent.com
  - .bilibili.com
  - .zhihu.com
  - .npmmirror.com
  - .mirrors.aliyun.com

  # Go module proxy (needed for go install/build)
  - proxy.golang.org
  - sum.golang.org
  - index.golang.org
```

!!! warning "no_proxy wildcard behavior"
    Domain suffixes (`.example.com`) have the broadest cross-client compatibility — curl, Go, Python, Node, and Java all handle them correctly. IP wildcards (`10.*`) behave differently across clients:

    | Client | `10.*` | `.example.com` | CIDR |
    |--------|:------:|:--------------:|:----:|
    | curl | ✅ | ✅ | ✅ |
    | Go | ✅ | ✅ | ❌ |
    | Python requests | ✅ | ✅ | ❌ |
    | Node.js | ❌ | ✅ | ❌ |
    | Java | ❌ | ✅ | ❌ |

    Prefer domain suffixes and explicit IP entries for maximum compatibility.

## Environment Variable Overrides

agent-proxy does not use environment variables for configuration — all settings come from `config.yaml`. However, it **respects** `SSH_AUTH_SOCK`:

- When `tunnel: true` and `ssh_key` is empty, agent-proxy checks for `SSH_AUTH_SOCK`. If set, SSH will use the ssh-agent for authentication instead of a key file.

The generated `env.sh` file **exports** (not reads) these variables for other tools:

```bash
export https_proxy='http://127.0.0.1:18443'
export http_proxy='http://127.0.0.1:18443'
export no_proxy='localhost,127.0.0.1,...'
export NO_PROXY="${no_proxy}"
```

## Example Configs

### Minimal (tunnel mode, recommended)

```yaml
proxy:
  host: 1.2.3.4
  ssh_key: ~/.ssh/id_ed25519
  tunnel: true

presets: [ai, dev, search, cloud, media]
```

Everything else uses defaults: port 18443, user root, all presets enabled, default no_proxy list.

### Direct mode (no SSH tunnel)

```yaml
proxy:
  host: 1.2.3.4
  port: 18443
  ssh_key: ~/.ssh/id_ed25519
  ssh_user: root
  tunnel: false

presets: [ai, dev]
no_proxy:
  - localhost
  - 127.0.0.1
  - .alibaba-inc.com
```

!!! warning
    Direct mode exposes Squid on a public port, protected only by an IP whitelist. Run `agent-proxy ip` whenever your public IP changes. Prefer tunnel mode.

### Multi-ECS failover

```yaml
proxy:
  host: 1.2.3.4
  ssh_key: ~/.ssh/id_ed25519
  tunnel: true

  fallback_host: 5.6.7.8
  fallback_ssh_key: ~/.ssh/backup_key
  fallback_ssh_user: admin

presets: [ai, dev, search, cloud, media]
```

When the primary host is unreachable during `agent-proxy on`, the tunnel automatically tries the fallback host. Both hosts must have Squid deployed (`agent-proxy setup` configures the primary; deploy to the fallback manually or via a second config).

### SSH agent authentication

```yaml
proxy:
  host: 1.2.3.4
  tunnel: true
  # No ssh_key — uses ssh-agent via SSH_AUTH_SOCK

presets: [ai, dev]
```

Requires `SSH_AUTH_SOCK` to be set (e.g., via `eval $(ssh-agent)` and `ssh-add`).

### Custom tunnel local port

```yaml
proxy:
  host: 1.2.3.4
  port: 18443
  tunnel: true
  tunnel_local_port: 28443    # Local endpoint on a different port
  ssh_key: ~/.ssh/id_ed25519
```

Useful when port 18443 is already in use by another service. The PAC file and env.sh will point to `127.0.0.1:28443`.

## Validation

Validate your config without making changes:

```bash
agent-proxy config-validate
```

This checks all validation rules and prints a summary. Mutating commands (`on`, `setup`, `ip`, `whitelist add/rm`, `preset enable/disable`) also run validation before proceeding.
