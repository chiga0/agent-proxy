# agent-proxy

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

## Platform Support

| Platform | System PAC | CLI env | Auto-start | SSH tunnel |
|----------|-----------|---------|------------|------------|
| macOS    | ✅ | ✅ | ✅ LaunchAgent | ✅ |
| Linux    | ✅ GNOME | ✅ | ✅ systemd user | ✅ |
| Windows  | ✅ Registry | ✅ | ✅ Scheduled Task | ✅ |

Linux without GNOME: CLI-only mode (env vars work, system PAC skipped automatically).

## Presets

60+ domains across 5 groups, all enabled by default:

| Preset | Examples |
|--------|---------|
| `ai` | OpenAI, ChatGPT, Anthropic, Gemini, Copilot, Codex, Perplexity |
| `dev` | GitHub, StackOverflow, npm, PyPI, crates.io, Docker, HuggingFace |
| `search` | Google, DuckDuckGo, Bing, Wikipedia |
| `cloud` | AWS, GCP, Azure docs & consoles |
| `media` | YouTube, Twitter/X, Instagram, Facebook, Telegram |

## Requirements

- Go 1.24+ (build from source only)
- SSH access to your ECS
- No external runtime dependencies

## License

[MIT](https://github.com/chiga0/agent-proxy/blob/main/LICENSE)
