# Quick Start

Get from zero to a working selective proxy in about 5 minutes.

## Prerequisites

### Your ECS server

You need a Linux server outside the firewall (Singapore, Tokyo, US-West, etc.) with:

| Requirement | Details |
|-------------|---------|
| OS | Any Linux with `apt`, `yum`, or `apk` (Ubuntu 22.04+, CentOS 7+, Alpine 3.18+ tested) |
| RAM | 512 MB minimum (Squid uses ~30 MB idle) |
| Disk | 1 GB free (Squid + logs) |
| Network | Public IP, outbound internet access |
| SSH | Port 22 reachable from your machine |
| Firewall | Port 22 open. In tunnel mode, **no other ports needed**. In direct mode, open your Squid port (default 18443) to your IP only |

!!! tip "Recommended: Alibaba Cloud Singapore"
    A 1 vCPU / 1 GB ECS in Singapore costs ~¥30/month and provides low-latency access from mainland China. Any provider works — AWS Lightsail, GCP e2-micro, Vultr, etc.

### SSH key access

agent-proxy authenticates via SSH key (no passwords). If you don't have a key yet:

```bash
# Generate an Ed25519 key (recommended)
ssh-keygen -t ed25519 -C "agent-proxy"

# Copy it to your ECS
ssh-copy-id -i ~/.ssh/id_ed25519 root@YOUR_ECS_IP
```

Verify you can connect without a password prompt:

```bash
ssh -i ~/.ssh/id_ed25519 root@YOUR_ECS_IP "echo ok"
# Expected: ok
```

!!! warning "macOS TCC-protected directories"
    If your SSH key is in `~/Documents/`, `~/Desktop/`, or `~/Downloads/`, background services (LaunchAgent) cannot access it due to macOS TCC restrictions. The `init` wizard will detect this and offer to copy the key to `~/.ssh/` automatically.

## Step 1: Install agent-proxy

```bash
# Auto-detect OS/arch, pick fastest mirror, verify SHA-256
curl -fsSL https://raw.githubusercontent.com/chiga0/agent-proxy/main/install.sh | bash
```

Expected output:

```
  → Detected: darwin/arm64
  → Downloading agent-proxy v0.7.2 ...
  → Verifying SHA-256 checksum... ✓
  → Installing to /usr/local/bin/agent-proxy
  ✓ agent-proxy v0.7.2 installed
```

### Alternative install methods

```bash
# China mirror (faster for CN users)
curl -fsSL https://agent-proxy.oss-cn-hangzhou.aliyuncs.com/install.sh | bash

# Specific version
curl -fsSL https://raw.githubusercontent.com/chiga0/agent-proxy/main/install.sh | bash -s -- --version v0.7.2

# Go install (requires Go 1.24+)
GONOSUMDB=github.com/chiga0/agent-proxy go install github.com/chiga0/agent-proxy/cmd/agent-proxy@latest

# Build from source
git clone https://github.com/chiga0/agent-proxy.git
cd agent-proxy && make build
```

Verify the installation:

```bash
agent-proxy version
# Expected: agent-proxy v0.7.2
```

## Step 2: Run the setup wizard

```bash
agent-proxy init
```

The interactive wizard walks you through everything. Here's what each step does and what you'll see:

### 2a. Server connection

```
  ┌─────────────────────────────────┐
  │   agent-proxy setup wizard      │
  └─────────────────────────────────┘

Server IP: 1.2.3.4
SSH user [root]:
SSH key path [~/.ssh/id_rsa]: ~/.ssh/id_ed25519
Proxy port [18443]:
```

- **Server IP**: Your ECS public IP address (no `http://` prefix).
- **SSH user**: Defaults to `root`. Use a different user if your ECS requires it.
- **SSH key path**: Path to your private key. Defaults to `~/.ssh/id_rsa`.
- **Proxy port**: The port Squid will listen on. Default `18443` is fine for most setups.

### 2b. SSH tunnel choice

```
─── SSH Tunnel ───
  Enable SSH encrypted tunnel? (recommended for China users) [Y/n]:
```

Press Enter to accept the default (yes). This is **strongly recommended** for users in China:

- Squid listens on `127.0.0.1` only — no public data port exposed
- All proxy traffic is encrypted via SSH
- No need to open firewall ports beyond SSH (22)

If you choose "n" (direct mode), Squid listens on all interfaces and is protected only by an IP whitelist. You'll need to ensure your ECS security group restricts the Squid port.

### 2c. Host key verification

```
─── Host Verification ───
  → Fetching host key from 1.2.3.4... ✓

  Host key fingerprint:
    256 SHA256:AbCdEf123456789... (ED25519)

  Verify the SHA256 fingerprint above matches your ECS console.
  Type yes to confirm: yes
  ✓ Saved to ~/.config/agent-proxy/known_hosts
```

The wizard fetches your ECS host key and displays its SHA256 fingerprint. **Verify this matches your ECS console** before typing `yes`. This prevents man-in-the-middle attacks on your SSH connection.

The key is stored in a project-specific `known_hosts` file (`~/.config/agent-proxy/known_hosts`) with `StrictHostKeyChecking=yes` — subsequent connections will reject any key mismatch.

### 2d. SSH connectivity + Squid deployment

```
─── Connectivity ───
  → SSH connection... ✓

─── Server Deployment ───
  → Connectivity... ✓
  → Install Squid... ✓
  → System tuning... ✓
  → Write Squid config... ✓
```

The wizard SSHes into your ECS and:

1. Installs Squid via the system package manager (apt/yum/apk)
2. Enables TCP fast open for better performance
3. Writes a deny-first Squid config (loopback-only in tunnel mode)
4. Validates the config syntax, restarts Squid, and verifies it's running

### 2e. Tunnel + local activation

```
─── SSH Tunnel ───
  → Establishing SSH tunnel... ✓

  ✓ Config saved: ~/.config/agent-proxy/config.yaml

─── Enable Proxy ───
  → SSH tunnel... ✓
  → PAC server... ✓

✓ Proxy enabled
  PAC (browser/desktop): http://127.0.0.1:18080/proxy.pac
  CLI env:               ~/.config/agent-proxy/env.sh
  Proxy:                 127.0.0.1:18443 (via SSH tunnel → 1.2.3.4:18443)
  Network service:       Wi-Fi
  Whitelist:             61 domains (presets: [ai dev search cloud media])
```

### 2f. Connectivity verification

```
─── Connectivity Check ───
  → google.com           ✓ 200 0.35s
  → github.com           ✓ 200 0.28s
  → youtube.com          ✓ 200 0.41s

  🎉 Setup complete!
```

## Step 3: Set up your shell profile

Add this line to `~/.zshrc` (or `~/.bashrc`):

```bash
[ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"
```

Then reload your current terminal:

```bash
source ~/.config/agent-proxy/env.sh
```

**What does this do?** The `env.sh` file exports three environment variables:

```bash
export https_proxy='http://127.0.0.1:18443'
export http_proxy='http://127.0.0.1:18443'
export no_proxy='localhost,127.0.0.1,::1,10.*,...,.alibaba-inc.com,.baidu.com,...'
```

- `https_proxy` / `http_proxy` — tells CLI tools (curl, pip, npm, etc.) to route through the local SSH tunnel endpoint.
- `no_proxy` — tells CLI tools to skip the proxy for localhost, private IPs, and domestic services.

Without sourcing this file, only browsers (via PAC) will use the proxy. CLI tools will go direct.

## Step 4: Verify everything works

```bash
# Quick health check (6 checks)
agent-proxy status
```

Expected output:

```
  ✓ SSH (22) (via tunnel — direct check skipped)
  ✓ SSH tunnel (127.0.0.1:18443 → 1.2.3.4:18443)
  ✓ Proxy forwarding (exit IP: 1.2.3.4)
  ✓ System PAC (http://127.0.0.1:18080/proxy.pac)
  ✓ PAC file (61 domains)
  ✓ PAC HTTP server (127.0.0.1:18080)

  Result: 6 passed, 0 failed
```

```bash
# Full diagnostics (includes no_proxy coverage analysis)
agent-proxy doctor
```

```bash
# Test a proxied request from CLI
curl -s https://httpbin.org/ip
# Should return your ECS IP, not your local IP

# Test that domestic sites go direct
curl -s https://www.baidu.com > /dev/null && echo "direct OK"
```

## What Just Happened?

Here's a summary of everything `agent-proxy init` set up:

| Component | Location | Purpose |
|-----------|----------|---------|
| Config file | `~/.config/agent-proxy/config.yaml` | All settings (host, port, presets, no_proxy) |
| SSH known_hosts | `~/.config/agent-proxy/known_hosts` | ECS host key for StrictHostKeyChecking |
| PAC file | `~/.config/agent-proxy/proxy.pac` | Generated JavaScript for browser routing |
| Env file | `~/.config/agent-proxy/env.sh` | Shell exports for CLI proxy |
| PAC state | `~/.config/agent-proxy/pac-state.json` | Snapshot of original PAC for safe restore |
| PID file | `~/.config/agent-proxy/pac-server.pid` | PAC daemon process tracking |
| Nonce file | `~/.config/agent-proxy/pac-nonce` | PAC server identity verification |
| SSH control socket | `~/.config/agent-proxy/ssh-ctrl-*` | ControlMaster connection multiplexing |
| ECS Squid config | `/etc/squid/squid.conf` (on ECS) | Deny-first proxy ACLs |
| Auto-start | LaunchAgent / systemd / Scheduled Task | Start proxy on login |

## Upgrading

```bash
# Self-update with SHA-256 verification
agent-proxy update

# Verify and trust the ECS SSH host key (after key rotation)
agent-proxy trust-host

# Rewrite ECS Squid config (e.g., switching to tunnel mode)
agent-proxy setup
```

## Next Steps

- Browse the [Commands](commands.md) reference for all available commands
- Customize your [Configuration](configuration.md) (add domains, adjust no_proxy)
- Explore the [Web Dashboard](api.md#dashboard) at `http://127.0.0.1:18080/dashboard`
- Set up [Monitoring](tutorials.md#tutorial-3-monitoring-with-prometheus-grafana) with Prometheus
