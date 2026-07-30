# FAQ

Common questions about agent-proxy, organized by topic.

## General

### Does this work with Codex desktop?

Yes. Codex desktop (and other Electron apps like VS Code, Cursor, and Claude desktop) use the system PAC settings that agent-proxy configures. However, **Electron apps cache the PAC at startup**, so you must:

1. Enable the proxy first: `agent-proxy on`
2. Then launch (or restart) the app

If the app was already running when you enabled the proxy, quit it completely and relaunch. Simply closing the window is not enough — use Cmd+Q (macOS) or Alt+F4 (Windows/Linux).

For CLI tools running inside VS Code's integrated terminal, also ensure your shell profile sources `env.sh`:

```bash
[ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"
```

---

### Can I use it without an ECS?

No. agent-proxy requires a Linux server outside the firewall to run Squid. It is not a VPN client or a protocol implementation — it manages a forward proxy on your own server.

If you don't have an ECS, options include:

- **Alibaba Cloud Singapore**: ~¥30/month for a 1 vCPU / 1 GB instance
- **AWS Lightsail**: ~$3.50/month for a 512 MB instance
- **Any Linux VPS**: Vultr, DigitalOcean, Linode, etc.

The server needs minimal resources (Squid uses ~30 MB RAM idle) and only SSH access in tunnel mode.

---

### What happens when my ECS reboots?

Squid is installed as a system service and starts automatically on boot. The SSH tunnel, however, runs on **your machine** and will die when the ECS goes down.

**What you'll see:** `agent-proxy status` shows the tunnel is down; proxied sites stop loading.

**What to do:**

```bash
agent-proxy on    # Restarts the SSH tunnel
```

If you have auto-start configured (LaunchAgent / systemd), the tunnel will also restart on your next login. But if the ECS reboots while you're logged in, you need to run `agent-proxy on` manually.

With a `fallback_host` configured, `agent-proxy on` automatically tries the fallback ECS if the primary is unreachable.

---

### How do I add a new domain?

```bash
agent-proxy whitelist add example.com
```

This adds the domain to `custom_domains` in `config.yaml`, saves the config, and regenerates the PAC file. If the PAC server is running with hot-reload, the change takes effect within 5 seconds.

To verify:

```bash
agent-proxy whitelist ls    # Should show your domain under "custom"
curl -s http://127.0.0.1:18080/proxy.pac | grep example.com
```

Subdomains are matched automatically — adding `example.com` also proxies `api.example.com`, `cdn.example.com`, etc.

---

### Why is my bandwidth high?

Common causes:

1. **Media preset**: YouTube, Instagram, and Facebook stream large amounts of data. If you only need AI/dev tools:

    ```bash
    agent-proxy preset disable media
    ```

2. **Misrouted domestic traffic**: Chinese sites going through your overseas ECS waste bandwidth and add latency:

    ```bash
    agent-proxy stats          # Check the 🇨 Chinese traffic percentage
    agent-proxy doctor --fix   # Auto-add flagged domains to no_proxy
    ```

3. **Package manager downloads**: Large `npm install`, `pip install`, or Docker pulls go through the proxy. These are usually one-time bursts, not sustained traffic.

4. **Cloud preset**: AWS/GCP/Azure console pages can be heavy. Disable if not needed:

    ```bash
    agent-proxy preset disable cloud
    ```

---

### Is this a VPN?

No. agent-proxy is an HTTP/HTTPS forward proxy. It handles:

- ✅ Browser traffic (via PAC)
- ✅ CLI tools that respect `https_proxy` (curl, pip, npm, etc.)
- ✅ SDKs and API clients that use system proxy settings

It does **not** handle:

- ❌ Arbitrary TCP/UDP (SSH, gaming, VoIP)
- ❌ Apps that ignore system proxy settings
- ❌ DNS-level unblocking (DNS must still resolve)

For full-device tunneling, consider a proper VPN (WireGuard, OpenVPN) or a SOCKS5 proxy.

---

## Configuration

### How do I change the proxy port?

Edit `~/.config/agent-proxy/config.yaml`:

```yaml
proxy:
  port: 28443    # Squid port on ECS
```

Then redeploy and restart:

```bash
agent-proxy setup    # Rewrites Squid config with new port
agent-proxy on       # Restarts tunnel with new port
```

If you only want to change the **local** tunnel endpoint (without changing the ECS port):

```yaml
proxy:
  port: 18443           # ECS port (unchanged)
  tunnel_local_port: 28443    # Local port
```

Then just `agent-proxy on` (no `setup` needed).

---

### How do I use ssh-agent instead of a key file?

Remove or leave empty the `ssh_key` field and ensure `SSH_AUTH_SOCK` is set:

```yaml
proxy:
  host: 1.2.3.4
  tunnel: true
  # ssh_key: (omitted)
```

```bash
eval $(ssh-agent)
ssh-add ~/.ssh/id_ed25519
agent-proxy on
```

agent-proxy checks for `SSH_AUTH_SOCK` during validation. If it's set, `ssh_key` is not required.

---

### Can I use a hostname instead of an IP?

Yes. The `proxy.host` field accepts any valid hostname:

```yaml
proxy:
  host: my-proxy.example.com
```

DNS resolution happens at connection time. This is useful if your ECS has a dynamic IP with a DDNS service.

---

### What's the difference between `whitelist` and `presets`?

- **Presets** are curated domain groups maintained by agent-proxy (`ai`, `dev`, `search`, `cloud`, `media`). They're enabled/disabled as groups via `agent-proxy preset enable/disable`.

- **Custom domains** are individual domains you add via `agent-proxy whitelist add`. They're always active as long as they're in the list.

The effective whitelist is the union of all enabled preset domains + all custom domains.

---

## Operations

### How do I completely uninstall agent-proxy?

```bash
# 1. Disable proxy and restore system settings
agent-proxy off

# 2. Remove auto-start
# macOS:
launchctl unload ~/Library/LaunchAgents/com.agent-proxy.plist 2>/dev/null
rm -f ~/Library/LaunchAgents/com.agent-proxy.plist

# Linux:
systemctl --user disable agent-proxy 2>/dev/null
rm -f ~/.config/systemd/user/agent-proxy.service

# 3. Remove config and data
rm -rf ~/.config/agent-proxy

# 4. Remove the binary
rm -f /usr/local/bin/agent-proxy

# 5. Remove shell profile line
# Delete this line from ~/.zshrc or ~/.bashrc:
# [ -f "$HOME/.config/agent-proxy/env.sh" ] && source "$HOME/.config/agent-proxy/env.sh"

# 6. (Optional) Remove Squid from ECS
ssh root@YOUR_ECS_IP "systemctl stop squid && apt remove -y squid"
```

---

### How do I check what traffic is going through the proxy?

```bash
# Quick summary with top domains
agent-proxy stats

# Larger sample
agent-proxy stats -n 5000

# Via web dashboard
open http://127.0.0.1:18080/dashboard

# Via JSON API
curl -s http://127.0.0.1:18080/api/stats | python3 -m json.tool
```

---

### How do I temporarily disable the proxy for one command?

```bash
# Unset proxy for a single command
https_proxy= http_proxy= curl https://www.baidu.com

# Or in a subshell
(unset https_proxy http_proxy no_proxy; some-command)
```

To disable the proxy entirely:

```bash
agent-proxy off
# ... do your thing ...
agent-proxy on
```

---

### Can multiple users share one ECS?

Yes, with caveats:

- Each user runs their own agent-proxy instance on their own machine
- In tunnel mode, each user has their own SSH tunnel — Squid sees all connections from loopback
- The Squid ACL allows all loopback traffic, so there's no per-user isolation on the ECS
- Bandwidth is shared — heavy usage by one user affects others

For per-user accounting, you'd need to modify the Squid config to use different ports or authentication, which is outside agent-proxy's scope.

---

### Does agent-proxy work with Docker?

Docker respects `https_proxy` / `http_proxy` environment variables for image pulls and build-time network access:

```bash
# Source the proxy env
source ~/.config/agent-proxy/env.sh

# Docker pull will go through the proxy
docker pull ubuntu:22.04
```

For the Docker daemon itself (not just the CLI), configure the proxy in `/etc/docker/daemon.json` or the systemd service file. Note that `docker.io` and `hub.docker.com` are in the `dev` preset, so browser access to Docker Hub also goes through the proxy.

---

### Why does `agent-proxy off` work even with a broken config?

This is by design. The `off` command is the emergency exit — it must work regardless of config state. When config loading fails, `off` falls back to a default config and performs best-effort cleanup:

1. Clears the system PAC (only if it points to agent-proxy)
2. Stops the PAC server daemon (via PID file or process pattern)
3. Stops the SSH tunnel (via ControlMaster socket or process pattern)
4. Removes auto-start registration
5. Deletes the env file

This ensures you can always recover from a bad state without manual intervention.
