# Troubleshooting

Organized by symptom. Each issue includes the diagnosis command, root cause, and fix.

## Proxy won't start

### "host not in project known_hosts"

**Symptom:**

```
Error: host 1.2.3.4 not in project known_hosts — run: agent-proxy trust-host
```

**Diagnosis:**

```bash
cat ~/.config/agent-proxy/known_hosts
# Empty or doesn't contain your ECS IP
```

**Root cause:** agent-proxy enforces `StrictHostKeyChecking=yes` with a project-specific `known_hosts` file. Your ECS host key hasn't been added yet (common after upgrading from < v0.6.0 or rebuilding the ECS).

**Fix:**

```bash
agent-proxy trust-host
# Verify the SHA256 fingerprint against your ECS console, then type "yes"
```

---

### "proxy.host is required"

**Symptom:**

```
Error: config validation: proxy.host is required
```

**Root cause:** The `proxy.host` field is empty in `config.yaml`. This happens if you ran a mutating command before `init`.

**Fix:**

```bash
agent-proxy init
# Or manually edit ~/.config/agent-proxy/config.yaml and set proxy.host
```

---

### "proxy.ssh_key is required when tunnel is enabled"

**Symptom:**

```
Error: config validation: proxy.ssh_key is required when tunnel is enabled (or set SSH_AUTH_SOCK for ssh-agent)
```

**Root cause:** Tunnel mode requires SSH authentication. Neither `proxy.ssh_key` nor `SSH_AUTH_SOCK` is configured.

**Fix (option A — key file):**

```yaml
# ~/.config/agent-proxy/config.yaml
proxy:
  ssh_key: ~/.ssh/id_ed25519
```

**Fix (option B — ssh-agent):**

```bash
eval $(ssh-agent)
ssh-add ~/.ssh/id_ed25519
agent-proxy on
```

---

### SSH tunnel fails to start

**Symptom:**

```
  → SSH tunnel... ✗
Error: start SSH tunnel: start SSH tunnel to root@1.2.3.4: ...: exit status 255
```

**Diagnosis:**

```bash
# Test SSH connectivity manually
ssh -i ~/.ssh/id_ed25519 root@1.2.3.4 "echo ok"

# Check if the tunnel port is occupied by another process
lsof -i :18443    # macOS
ss -tlnp | grep 18443    # Linux
```

**Common root causes:**

| Cause | How to tell | Fix |
|-------|-------------|-----|
| ECS is down | SSH connection times out | Start the ECS, retry |
| Wrong SSH key | "Permission denied (publickey)" | Check `proxy.ssh_key` path |
| Port conflict | Another process on 18443 | Kill the process or set `tunnel_local_port` |
| Firewall blocks SSH | Connection refused on port 22 | Open port 22 in ECS security group |
| Stale control socket | `ssh-ctrl-*` file exists but master is dead | `rm ~/.config/agent-proxy/ssh-ctrl-*` then retry |

**Fix for stale control socket:**

```bash
rm -f ~/.config/agent-proxy/ssh-ctrl-*
agent-proxy on
```

---

## Proxy is running but not working

### Browser shows "proxy connection refused"

**Symptom:** Whitelisted sites fail to load; non-whitelisted sites work fine.

**Diagnosis:**

```bash
agent-proxy status
```

Look for:

```
  ✗ SSH tunnel (not running — run: agent-proxy on)
  ✗ Proxy forwarding (dial tcp 127.0.0.1:18443: connect: connection refused)
```

**Root cause:** The SSH tunnel is down. The PAC file tells the browser to use `PROXY 127.0.0.1:18443`, but nothing is listening there.

**Fix:**

```bash
agent-proxy on
```

If the tunnel keeps dying, check ECS connectivity and SSH keepalive settings (see [Tunnel drops frequently](#tunnel-drops-frequently)).

---

### "TLS reset — possible SNI filtering"

**Symptom:**

```
  ✗ Proxy forwarding (TLS reset — possible SNI filtering (try: proxy.tunnel: true))
```

**Root cause:** You're using direct mode (`tunnel: false`). The GFW is inspecting the TLS ClientHello and resetting connections to blocked domains. The proxy CONNECT request goes through, but the subsequent TLS handshake is killed.

**Diagnosis:**

```bash
agent-proxy doctor
# Look for: "⚠ TLS connections reset after CONNECT — GFW SNI filtering detected"
```

**Fix:** Enable SSH tunnel mode:

```yaml
# ~/.config/agent-proxy/config.yaml
proxy:
  tunnel: true
```

Then redeploy and restart:

```bash
agent-proxy setup    # Rewrites Squid for loopback-only
agent-proxy on
```

---

### PAC not working (browser ignores proxy)

**Symptom:** `agent-proxy status` shows all checks pass, but the browser still goes direct for whitelisted domains.

**Diagnosis:**

```bash
# Check the system PAC setting
# macOS:
networksetup -getautoproxyurl Wi-Fi

# Verify PAC file is served
curl -s http://127.0.0.1:18080/proxy.pac | head -5

# Check PAC file content
cat ~/.config/agent-proxy/proxy.pac | grep "chatgpt"
```

**Common root causes:**

| Cause | Fix |
|-------|-----|
| App cached PAC at startup | Restart the browser/app |
| Wrong network service | `agent-proxy off && agent-proxy on` (re-detects service) |
| PAC server from old version | `agent-proxy off && agent-proxy on` (nonce mismatch triggers takeover) |
| Browser proxy extension override | Disable SwitchyOmega or similar extensions |

---

### Codex / desktop app can't connect

**Symptom:** Codex desktop, VS Code, or other Electron apps fail to reach AI APIs even though `curl` works.

**Root cause:** Electron apps cache the system PAC at startup. If you enabled the proxy after launching the app, it doesn't see the new PAC.

**Fix:**

1. Quit the application completely (not just close the window)
2. Ensure the proxy is running: `agent-proxy status`
3. Relaunch the application

For CLI-based tools inside VS Code's integrated terminal, ensure `env.sh` is sourced:

```bash
source ~/.config/agent-proxy/env.sh
```

---

## Performance issues

### Bandwidth too high

**Symptom:** Your ECS bandwidth bill is unexpectedly large.

**Diagnosis:**

```bash
agent-proxy stats -n 2000
```

Look for:

- High-traffic domains you didn't expect
- Chinese traffic percentage (should be near 0%)
- Media preset domains (YouTube, Instagram) consuming bulk bandwidth

**Fixes:**

1. **Disable unused presets:**

    ```bash
    agent-proxy preset disable media    # YouTube, Twitter, Instagram
    agent-proxy preset disable cloud    # AWS/GCP/Azure consoles
    ```

2. **Fix misrouted domestic traffic:**

    ```bash
    agent-proxy doctor --fix
    ```

3. **Check for runaway processes:** A CI/CD pipeline or package manager downloading large artifacts through the proxy can consume significant bandwidth. Check `stats` output for unexpected domains.

---

### Proxy is slow

**Symptom:** Pages load noticeably slower through the proxy than expected.

**Diagnosis:**

```bash
# Compare proxy vs direct latency
agent-proxy bench

# Trace the network path
agent-proxy trace chatgpt.com
```

**Interpreting results:**

| Finding | Meaning | Fix |
|---------|---------|-----|
| Local → ECS > 200ms | Your machine is far from the ECS | Use a closer ECS region |
| ECS → Target > 200ms | ECS is far from the target | Use a different ECS region |
| Both hops fast, total slow | DNS or TLS overhead | Check ECS DNS settings |
| Direct faster than proxy | Domain isn't actually blocked | Remove from whitelist |

**Performance tuning:** agent-proxy already enables TCP fast open, persistent connections, collapsed forwarding, and DNS caching on the ECS. If latency is still high, the bottleneck is likely geographic distance.

---

## Tunnel issues

### Tunnel drops frequently

**Symptom:** The proxy works initially but stops after a few minutes. `agent-proxy status` shows the tunnel is down.

**Diagnosis:**

```bash
# Check if the control socket exists
ls -la ~/.config/agent-proxy/ssh-ctrl-*

# Check SSH process
ps aux | grep "ssh.*-L.*18443"
```

**Root causes and fixes:**

1. **Network instability:** The SSH keepalive (30s interval, 3 max) declares the tunnel dead after 90 seconds of no response. On unstable networks, this can be too aggressive.

    Unfortunately, agent-proxy's SSH options are hardcoded for security. As a workaround, ensure your local network is stable, or use a wired connection.

2. **ECS suspends idle connections:** Some cloud providers throttle or drop idle SSH sessions. The `ServerAliveInterval=30` should prevent this, but some providers are more aggressive.

3. **ControlPersist timeout:** The master socket expires after 600 seconds (10 minutes) of inactivity. If you don't use the proxy for 10 minutes, the tunnel shuts down. Run `agent-proxy on` to restart it.

---

### "port 18443 occupied by another process"

**Symptom:**

```
  ✗ SSH tunnel (port 18443 occupied by another process — not our tunnel)
```

**Diagnosis:**

```bash
lsof -i :18443    # macOS
ss -tlnp | grep 18443    # Linux
```

**Fix (option A — kill the conflicting process):**

```bash
kill <PID>    # Replace with the PID from lsof/ss
agent-proxy on
```

**Fix (option B — use a different local port):**

```yaml
# ~/.config/agent-proxy/config.yaml
proxy:
  tunnel_local_port: 28443
```

```bash
agent-proxy on
```

---

## ECS / Squid issues

### "ECS Squid is NOT loopback-only"

**Symptom:**

```
  ⚠ ECS Squid is NOT loopback-only: http_port 18443
    Your Squid may still be listening on all interfaces from a previous deployment.
    Fix: agent-proxy setup   (rewrites Squid config for tunnel mode)
```

**Root cause:** You upgraded from a version that used direct mode, or manually edited the Squid config. Squid is still listening on `0.0.0.0`, exposing it to the network.

**Fix:**

```bash
agent-proxy setup
```

This rewrites the Squid config with `http_port 127.0.0.1:18443` and restarts Squid.

---

### Squid won't start on ECS

**Symptom:** `agent-proxy setup` fails at "Write Squid config" step.

**Diagnosis:**

```bash
# SSH into your ECS and check Squid status
ssh root@YOUR_ECS_IP
systemctl status squid
journalctl -u squid --no-pager -n 50

# Check if the config is valid
squid -k parse
```

**Common causes:**

| Cause | Fix |
|-------|-----|
| Squid not installed | `apt install -y squid` (or yum/apk) |
| Port already in use | `lsof -i :18443` on ECS, kill conflicting process |
| SELinux blocking | `setsebool -P squid_use_tty 1` or check audit log |
| Disk full | `df -h` on ECS, clean up logs |

!!! info "Automatic rollback"
    agent-proxy's deployment is transactional. If Squid fails to restart with the new config, it automatically rolls back to the previous config and restarts. Your ECS won't be left in a broken state.

---

## Config issues

### Config corrupted, need emergency off

**Symptom:** `config.yaml` is corrupted (bad YAML, missing fields) and you need to disable the proxy immediately.

**Fix:**

```bash
agent-proxy off
```

This works even with a broken config — it loads a default config and performs best-effort cleanup:

```
Warning: config load failed (...), proceeding with best-effort cleanup
✓ Proxy disabled
```

---

### Chinese traffic going through proxy

**Symptom:** Domestic sites (Baidu, Bilibili, etc.) are slow because they're routing through your overseas ECS.

**Diagnosis:**

```bash
agent-proxy doctor
# Look for: "⚠ N Chinese domain(s) routing through proxy"
```

**Fix (automatic):**

```bash
agent-proxy doctor --fix
# Adds flagged domains to no_proxy and regenerates env.sh
```

**Fix (manual):**

```yaml
# ~/.config/agent-proxy/config.yaml
no_proxy:
  - .example.cn
  - .mysite.com.cn
```

```bash
agent-proxy on    # Regenerates env.sh
```

---

### `go build` fails with proxy

**Symptom:**

```
go: module lookup disabled by GOPROXY=off
```

Or Go downloads are slow/failing through the proxy.

**Root cause:** `proxy.golang.org` and `sum.golang.org` are in the default `no_proxy` list, but your shell may not have sourced the latest `env.sh`.

**Fix:**

```bash
source ~/.config/agent-proxy/env.sh
go build ./...
```

If the issue persists, verify:

```bash
echo $no_proxy | tr ',' '\n' | grep golang
# Should show: proxy.golang.org, sum.golang.org, index.golang.org
```

---

## macOS-specific

### "developer cannot be verified"

**Symptom:** macOS Gatekeeper blocks the binary on first launch.

**Fix:**

```bash
xattr -d com.apple.quarantine /usr/local/bin/agent-proxy
```

---

### SSH key in TCC-protected directory

**Symptom:** The tunnel works in the foreground but fails when started by LaunchAgent (auto-start).

**Root cause:** macOS TCC (Transparency, Consent, and Control) restricts background processes from accessing `~/Documents/`, `~/Desktop/`, and `~/Downloads/`. Your SSH key is in one of these directories.

**Fix:**

```bash
# Copy the key to ~/.ssh/
cp ~/Documents/my-key.pem ~/.ssh/my-key.pem
chmod 600 ~/.ssh/my-key.pem

# Update config
# proxy.ssh_key: ~/.ssh/my-key.pem
```

The `init` wizard detects this and offers to copy automatically.

---

## Quick reference

| Symptom | First command to run |
|---------|---------------------|
| Any issue | `agent-proxy status` |
| Need details | `agent-proxy doctor` |
| Slow proxy | `agent-proxy bench` |
| Network path | `agent-proxy trace <domain>` |
| Traffic audit | `agent-proxy stats` |
| Emergency stop | `agent-proxy off` |
| Full reset | `agent-proxy off && agent-proxy setup && agent-proxy on` |
