# Tutorials

Step-by-step guides for common scenarios beyond the basic quick start.

## Tutorial 1: Fresh ECS Setup from Scratch (Alibaba Cloud)

This tutorial walks through provisioning a new Alibaba Cloud ECS and configuring agent-proxy from zero.

### Step 1: Create the ECS instance

1. Log in to the [Alibaba Cloud Console](https://ecs.console.aliyun.com/)
2. Click **Create Instance**
3. Choose these settings:

    | Setting | Recommended value | Why |
    |---------|-------------------|-----|
    | Region | Singapore / Tokyo / US-West | Low latency from China, outside the firewall |
    | Instance type | ecs.t6-c1m1.large (1 vCPU, 1 GB) | Squid needs ~30 MB RAM; this is plenty |
    | OS | Ubuntu 22.04 64-bit | Best tested with agent-proxy |
    | Disk | 40 GB ESSD | Logs grow slowly; 40 GB lasts months |
    | Network | Assign public IP, 1 Mbps+ | Pay-by-traffic is cheapest for proxy use |
    | Security group | Allow TCP 22 inbound | SSH access; no other ports needed in tunnel mode |

4. Set the login method to **SSH key pair** (create one in the console if needed)
5. Launch the instance and note the **public IP address**

### Step 2: Configure SSH access

```bash
# Download the key pair from Alibaba Cloud console (or use your own)
chmod 600 ~/Downloads/my-ecs-key.pem

# Test SSH connectivity
ssh -i ~/Downloads/my-ecs-key.pem root@YOUR_ECS_IP "uname -a"
# Expected: Linux iZ... 5.15.0-... #1 SMP ... x86_64 x86_64 x86_64 GNU/Linux
```

!!! tip "Copy the key to ~/.ssh/"
    macOS background services can't access `~/Downloads/` due to TCC restrictions. Copy the key now:

    ```bash
    mkdir -p ~/.ssh && chmod 700 ~/.ssh
    cp ~/Downloads/my-ecs-key.pem ~/.ssh/ecs-singapore.pem
    chmod 600 ~/.ssh/ecs-singapore.pem
    ```

### Step 3: Install and initialize agent-proxy

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/chiga0/agent-proxy/main/install.sh | bash

# Run the wizard
agent-proxy init
```

When prompted:

```
服务器 IP: YOUR_ECS_IP
SSH 用户 [root]:                    ← press Enter
SSH 密钥路径 [~/.ssh/id_rsa]: ~/.ssh/ecs-singapore.pem
代理端口 [18443]:                   ← press Enter
启用 SSH 加密隧道? [Y/n]:          ← press Enter (yes)
```

Verify the host key fingerprint against the Alibaba Cloud console (Instance → Security → Key Pair fingerprint), then type `yes`.

### Step 4: Configure your shell

```bash
echo '[ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"' >> ~/.zshrc
source ~/.config/agent-proxy/env.sh
```

### Step 5: Verify

```bash
agent-proxy status
# All 6 checks should pass

curl -s https://httpbin.org/ip | grep origin
# Should show your ECS IP, not your local IP

curl -s https://www.baidu.com > /dev/null && echo "domestic OK"
# Should print "domestic OK" (goes direct, not through proxy)
```

### Step 6: Set up the ECS security group (tunnel mode)

In tunnel mode, only SSH (port 22) needs to be open. Verify your security group:

1. Go to **ECS Console → Security Groups**
2. Find your instance's security group
3. Ensure inbound rules allow **only** TCP 22 from your IP (or `0.0.0.0/0` if your IP changes)
4. **Remove** any rules opening port 18443 or other Squid ports — they're unnecessary in tunnel mode

---

## Tutorial 2: Upgrading from v0.5.x to v0.7.x

Versions before v0.6.0 used Basic auth and system `known_hosts`. This tutorial migrates cleanly.

### Step 1: Update the binary

```bash
agent-proxy update
agent-proxy version
# Expected: agent-proxy v0.7.x
```

### Step 2: Migrate SSH host keys

v0.6.0+ uses a project-specific `known_hosts` file instead of `~/.ssh/known_hosts`:

```bash
agent-proxy trust-host
```

If your ECS key is found in `~/.ssh/known_hosts`, the command offers to migrate it:

```
  → 在系统 known_hosts 中发现 1.2.3.4 的密钥
  迁移到项目专用 known_hosts? [Y/n]: y
  ✓ 已迁移到 /Users/you/.config/agent-proxy/known_hosts
```

### Step 3: Redeploy Squid (loopback-only)

v0.6.0+ uses loopback-only Squid in tunnel mode (no public data port):

```bash
agent-proxy setup
```

This rewrites the Squid config, removing Basic auth and binding to `127.0.0.1` only.

### Step 4: Clean up legacy config

The config loader automatically:

- Strips `proxy.user` and `proxy.password` fields (Basic auth removed)
- Migrates flat `whitelist` arrays to `presets` + `custom_domains`

Verify:

```bash
agent-proxy config-validate
# Should show valid config with presets listed
```

### Step 5: Restart the proxy

```bash
agent-proxy off
agent-proxy on
agent-proxy doctor
# Should show all checks passing, Squid loopback-only confirmed
```

---

## Tutorial 3: Multi-ECS Failover Setup

Set up a secondary ECS that agent-proxy automatically falls back to when the primary is unreachable.

### Architecture

```
Your Machine
    │
    ├─ SSH tunnel ──▶ Primary ECS (Singapore) ──▶ Squid ──▶ Internet
    │
    └─ (failover) ──▶ Fallback ECS (Tokyo) ──▶ Squid ──▶ Internet
```

### Step 1: Provision the fallback ECS

Set up a second ECS following [Tutorial 1](#tutorial-1-fresh-ecs-setup-from-scratch-alibaba-cloud). Use a different region for geographic diversity.

### Step 2: Deploy Squid on the fallback

agent-proxy's `setup` command targets the primary host. To deploy Squid on the fallback, temporarily switch the config:

```bash
# Save current config
cp ~/.config/agent-proxy/config.yaml ~/.config/agent-proxy/config.yaml.bak

# Edit config to point to fallback
# proxy.host: FALLBACK_ECS_IP
# proxy.ssh_key: ~/.ssh/fallback-key.pem

agent-proxy trust-host    # Trust the fallback host key
agent-proxy setup         # Deploy Squid to fallback

# Restore original config
cp ~/.config/agent-proxy/config.yaml.bak ~/.config/agent-proxy/config.yaml
```

### Step 3: Configure failover

Edit `~/.config/agent-proxy/config.yaml`:

```yaml
proxy:
  host: 1.2.3.4                    # Primary (Singapore)
  ssh_key: ~/.ssh/ecs-singapore.pem
  tunnel: true

  fallback_host: 5.6.7.8           # Fallback (Tokyo)
  fallback_ssh_key: ~/.ssh/ecs-tokyo.pem
  fallback_ssh_user: root           # Optional, defaults to primary ssh_user
```

### Step 4: Verify

```bash
agent-proxy config-validate
# Should show: Fallback: 5.6.7.8

agent-proxy on
# Connects to primary
```

### Step 5: Test failover

```bash
# Stop the primary ECS (or block its IP temporarily)
agent-proxy off
agent-proxy on
```

Expected output when primary is down:

```
  → SSH tunnel... ⚠ Primary 1.2.3.4 failed (connection timed out), trying fallback 5.6.7.8...
  → SSH tunnel... ✓
```

!!! info "Failover scope"
    Failover applies to tunnel startup only. If the primary goes down mid-session, the tunnel dies and you need to run `agent-proxy on` again (or rely on auto-start at next login). The `ControlPersist=600` setting keeps the tunnel alive for 10 minutes of inactivity, which handles brief network blips.

---

## Tutorial 4: Monitoring with Prometheus + Grafana

agent-proxy exposes Prometheus metrics on the PAC server. This tutorial sets up scraping and a dashboard.

### Step 1: Verify metrics endpoint

```bash
curl -s http://127.0.0.1:18080/metrics
```

Expected output:

```
# HELP agent_proxy_pac_requests_total Total PAC file requests served.
# TYPE agent_proxy_pac_requests_total counter
agent_proxy_pac_requests_total 42

# HELP agent_proxy_pac_server_up Whether the PAC server is running.
# TYPE agent_proxy_pac_server_up gauge
agent_proxy_pac_server_up 1

# HELP agent_proxy_config_domains_total Number of whitelisted domains.
# TYPE agent_proxy_config_domains_total gauge
agent_proxy_config_domains_total 61

# HELP agent_proxy_config_presets_total Number of enabled presets.
# TYPE agent_proxy_config_presets_total gauge
agent_proxy_config_presets_total 5

# HELP agent_proxy_config_noproxy_total Number of no_proxy entries.
# TYPE agent_proxy_config_noproxy_total gauge
agent_proxy_config_noproxy_total 32

# HELP agent_proxy_tunnel_enabled Whether tunnel mode is enabled.
# TYPE agent_proxy_tunnel_enabled gauge
agent_proxy_tunnel_enabled 1
```

### Step 2: Configure Prometheus

Add to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'agent-proxy'
    scrape_interval: 30s
    static_configs:
      - targets: ['127.0.0.1:18080']
    metrics_path: /metrics
```

Restart Prometheus:

```bash
# Docker
docker restart prometheus

# Systemd
sudo systemctl restart prometheus
```

### Step 3: Verify scraping

Open the Prometheus UI (`http://localhost:9090`) and query:

```promql
agent_proxy_pac_server_up
```

Should return `1`.

### Step 4: Create a Grafana dashboard

Import or create a dashboard with these panels:

| Panel | Query | Type |
|-------|-------|------|
| PAC Server Status | `agent_proxy_pac_server_up` | Stat (green/red) |
| PAC Requests Rate | `rate(agent_proxy_pac_requests_total[5m])` | Time series |
| Whitelisted Domains | `agent_proxy_config_domains_total` | Stat |
| Enabled Presets | `agent_proxy_config_presets_total` | Stat |
| No-Proxy Entries | `agent_proxy_config_noproxy_total` | Stat |
| Tunnel Mode | `agent_proxy_tunnel_enabled` | Stat (Enabled/Disabled) |

### Example alerting rule

```yaml
# alert-rules.yml
groups:
  - name: agent-proxy
    rules:
      - alert: PACServerDown
        expr: agent_proxy_pac_server_up == 0
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "agent-proxy PAC server is down"
          description: "Run: agent-proxy on"
```

---

## Tutorial 5: Using the Web Dashboard

agent-proxy includes a self-contained web dashboard — no external dependencies, no build step.

### Step 1: Open the dashboard

With the proxy running (`agent-proxy on`), open:

```
http://127.0.0.1:18080/dashboard
```

### Step 2: Read the dashboard

The dashboard has three sections:

**Proxy Status** — cards showing:

| Card | Meaning |
|------|---------|
| Host | Your ECS IP address |
| Port | Squid port |
| Tunnel Mode | Whether SSH tunnel is enabled |
| Fallback Host | Fallback ECS IP (or "None") |

**Whitelist & No-Proxy** — cards showing:

| Card | Meaning |
|------|---------|
| Active Presets | Which preset groups are enabled (badges) |
| Whitelist Domains | Total number of proxied domains |
| No-Proxy Entries | Number of no_proxy exclusions |

**Recent Traffic** — a table of the top 10 domains by traffic from the last 200 Squid access log lines:

| Column | Meaning |
|--------|---------|
| Domain | The requested domain |
| Requests | Number of requests in the sample |
| Bytes | Total traffic volume |

### Step 3: Use the JSON APIs

The dashboard is powered by two JSON endpoints you can use programmatically:

```bash
# Proxy status
curl -s http://127.0.0.1:18080/api/status | python3 -m json.tool

# Traffic stats
curl -s http://127.0.0.1:18080/api/stats | python3 -m json.tool
```

See the [API Reference](api.md) for full endpoint documentation.

### Troubleshooting the dashboard

| Issue | Cause | Fix |
|-------|-------|-----|
| Page won't load | PAC server not running | `agent-proxy on` |
| Status shows error | Config file missing/corrupt | `agent-proxy config-validate` |
| Traffic table empty | No Squid logs yet, or SSH failed | Use the proxy, then refresh |
| Traffic shows error | Can't SSH to ECS for logs | Check SSH connectivity |
