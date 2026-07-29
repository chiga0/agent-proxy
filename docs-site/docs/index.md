# agent-proxy

**Domain-based selective proxy CLI.** Route AI services, developer tools, and search engines through your overseas server — everything else stays direct.

60+ domains pre-configured. One command to set up. SSH-encrypted tunnel. Zero runtime dependencies.

## Why agent-proxy?

If you're a developer in China (or behind a restrictive firewall), you've probably tried several approaches to reach services like ChatGPT, GitHub, and Google. Here's how agent-proxy compares:

| Approach | Selective routing | CLI/SDK support | Encryption | Setup effort | Ongoing maintenance |
|----------|:-:|:-:|:-:|:-:|:-:|
| **agent-proxy** | ✅ Domain-based PAC + env | ✅ `https_proxy` + `no_proxy` | ✅ SSH tunnel | One command | Near-zero |
| ClashX / Clash | ✅ Rule-based | Partial (env vars) | ✅ | Medium (rule files, GUI) | Rule updates, GUI quirks |
| V2Ray / Xray | ✅ | Manual | ✅ | High (protocol config, ports) | Protocol tuning, key rotation |
| Manual `export https_proxy=...` | ❌ All-or-nothing | ✅ | Depends | Low | Breaks domestic sites |
| Browser extension (SwitchyOmega) | ✅ Browser only | ❌ | ❌ | Low | Per-browser config |

**Key differentiators:**

- **Zero-config presets.** 60+ domains across AI, dev, search, cloud, and media are pre-configured. Run `agent-proxy init` and everything works — no rule files, no YAML editing, no GUI.
- **Two routing paths, one proxy.** Browsers and desktop apps use the system PAC file; CLI tools and SDKs use `https_proxy`/`no_proxy` environment variables. Both paths go through the same Squid proxy on your ECS.
- **SSH-encrypted by default.** In tunnel mode, Squid listens on loopback only — no public data port is exposed. All proxy traffic is encrypted through an SSH tunnel with modern ciphers (AES-GCM, ChaCha20).
- **Self-contained binary.** No Python, no Node, no Docker at runtime. A single Go binary handles everything: deployment, tunnel management, PAC serving, and diagnostics.
- **Domestic traffic protection.** Built-in `no_proxy` defaults and `doctor --fix` ensure Chinese/domestic sites (Alibaba, Baidu, Bilibili, etc.) never accidentally route through your overseas proxy.

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

### How the PAC path works

1. `agent-proxy on` starts a local HTTP server on `127.0.0.1:18080` that serves a generated `proxy.pac` file.
2. The system proxy settings are updated to point to `http://127.0.0.1:18080/proxy.pac`.
3. For every HTTP request, the browser evaluates the PAC JavaScript: whitelisted domains go through `PROXY 127.0.0.1:18443`, everything else goes `DIRECT`.
4. The PAC server hot-reloads when `config.yaml` changes — no restart needed.

### How the CLI path works

1. `agent-proxy on` generates `~/.config/agent-proxy/env.sh` with `https_proxy`, `http_proxy`, and `no_proxy` exports.
2. Your shell profile sources this file, so every new terminal inherits the proxy settings.
3. Tools like `curl`, `pip`, `npm`, and Go's HTTP client respect these variables automatically.
4. The `no_proxy` list ensures domestic services (Alibaba Cloud, Baidu, etc.) bypass the proxy.

## Feature Matrix

| Feature | Details |
|---------|---------|
| **Presets** | 5 groups (ai, dev, search, cloud, media), 60+ domains, all enabled by default |
| **Custom domains** | `whitelist add/rm` with instant PAC regeneration |
| **SSH tunnel** | ControlMaster multiplexing, AES-GCM/ChaCha20 ciphers, keepalive, auto-failover |
| **Squid deployment** | Idempotent, transactional (syntax check → backup → replace → rollback on failure) |
| **PAC server** | Local HTTP, nonce-protected, hot-reload on config change |
| **Web dashboard** | Self-contained HTML at `/dashboard` with status + traffic stats |
| **Prometheus metrics** | 6 metrics at `/metrics` for monitoring |
| **Diagnostics** | `status` (6 checks), `doctor` (full + no_proxy analysis), `bench`, `trace` |
| **Traffic stats** | Squid access log parsing, top domains, bandwidth, Chinese traffic detection |
| **Self-update** | SHA-256 verified, version-pinned install script |
| **Auto-start** | LaunchAgent (macOS), systemd user (Linux), Scheduled Task (Windows) |
| **Emergency off** | `agent-proxy off` works even with corrupted config |
| **Multi-ECS failover** | Optional `fallback_host` for automatic tunnel failover |

## Who Is This For?

- **AI developers in China** who need reliable access to OpenAI, Anthropic, Gemini, and Copilot APIs from both browsers and CLI tools.
- **Full-stack developers** who use GitHub, npm, PyPI, Docker Hub, and StackOverflow daily and are tired of toggling proxy settings.
- **Teams** that want a simple, auditable proxy setup without maintaining a full VPN or V2Ray infrastructure.
- **Anyone** who wants selective proxying — proxy only what needs it, keep everything else direct for speed and reliability.

!!! info "Not a VPN"
    agent-proxy is an HTTP/HTTPS forward proxy, not a VPN. It handles web traffic (browsers, curl, package managers, SDKs) but does not tunnel arbitrary TCP/UDP (SSH, gaming, VoIP). For those use cases, consider a full VPN solution.

## Platform Support

| Platform | System PAC | CLI env | Auto-start | SSH tunnel |
|----------|:-:|:-:|:-:|:-:|
| macOS    | ✅ | ✅ | ✅ LaunchAgent | ✅ |
| Linux    | ✅ GNOME | ✅ | ✅ systemd user | ✅ |
| Windows  | ✅ Registry | ✅ | ✅ Scheduled Task | ✅ |

Linux without GNOME: CLI-only mode (env vars work, system PAC skipped automatically).

## Presets

60+ domains across 5 groups, all enabled by default:

| Preset | Examples |
|--------|---------|
| `ai` | OpenAI, ChatGPT, Anthropic, Claude, Gemini, Copilot, Codex, Perplexity, Mistral, OpenRouter |
| `dev` | GitHub, StackOverflow, npm, PyPI, crates.io, Docker Hub, HuggingFace, go.dev, docs.rs |
| `search` | Google (Mail, Docs, Drive, Scholar, Maps), DuckDuckGo, Bing, Wikipedia |
| `cloud` | AWS docs/console, GCP console, Azure portal, Microsoft Learn |
| `media` | YouTube, Twitter/X, Instagram, Facebook, Telegram |

See [Commands → preset](commands.md#preset) for managing presets, or [Configuration → presets](configuration.md#presets) for the full domain list.

## Requirements

- **Your machine:** macOS, Linux, or Windows. No runtime dependencies.
- **Your server:** Any Linux ECS with SSH access (Alibaba Cloud, AWS, GCP, etc.). apt, yum, or apk package manager.
- **Build from source only:** Go 1.24+

## License

[MIT](https://github.com/chiga0/agent-proxy/blob/main/LICENSE)
