# Quick Start

## Install

```bash
# Auto-detect OS/arch, pick fastest mirror, verify SHA-256
curl -fsSL https://raw.githubusercontent.com/chiga0/agent-proxy/main/install.sh | bash
```

### Other install methods

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

## Initialize

Run the interactive setup wizard:

```bash
agent-proxy init
```

This walks you through:

1. SSH key selection
2. Squid deployment to your ECS
3. SSH tunnel establishment
4. System PAC configuration
5. End-to-end verification

## Shell Profile Setup

Add to `~/.zshrc` or `~/.bashrc`:

```bash
[ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"
```

Done. New terminals auto-load proxy env vars. Browsers use system PAC automatically.

## Verify

```bash
agent-proxy status    # Quick health check
agent-proxy doctor    # Full diagnostics
```

## Upgrading

```bash
agent-proxy update          # Self-update with SHA-256 verification

# After upgrading from < v0.6.0:
agent-proxy trust-host      # Migrate SSH host key to project known_hosts
agent-proxy setup           # Rewrite ECS Squid to loopback-only
```
