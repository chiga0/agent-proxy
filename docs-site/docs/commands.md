# Commands

Complete reference for every agent-proxy command, flag, and subcommand.

## Global Flags

These flags work on all commands:

| Flag | Short | Description |
|------|-------|-------------|
| `--verbose` | `-v` | Enable verbose output |
| `--help` | `-h` | Show help for any command |

---

## Lifecycle Commands

### on

Enable the proxy: starts the SSH tunnel, PAC server, sets system PAC, and writes CLI env vars.

```bash
agent-proxy on
```

**What it does (in order):**

1. Starts the SSH tunnel (if `proxy.tunnel: true`) with ControlMaster multiplexing
2. Generates the PAC file from presets + custom domains
3. Starts the PAC HTTP server daemon on `127.0.0.1:18080`
4. Saves the current system PAC state (for safe restore later)
5. Sets the system PAC to `http://127.0.0.1:18080/proxy.pac`
6. Writes `~/.config/agent-proxy/env.sh` with proxy environment variables
7. Registers auto-start (LaunchAgent / systemd / Scheduled Task)

If any step fails, all previous steps are rolled back automatically.

**Sample output:**

```
  → SSH tunnel... ✓
  → PAC server... ✓

✓ Proxy enabled
  PAC (browser/desktop): http://127.0.0.1:18080/proxy.pac
  CLI env:               /Users/you/.config/agent-proxy/env.sh
  Proxy:                 127.0.0.1:18443 (via SSH tunnel → 1.2.3.4:18443)
  Network service:       Wi-Fi
  Whitelist:             61 domains (presets: [ai dev search cloud media])

Current terminal: source /Users/you/.config/agent-proxy/env.sh
```

---

### off

Disable the proxy: restores original system PAC, stops the PAC server and SSH tunnel, removes auto-start.

```bash
agent-proxy off
```

**Key behaviors:**

- Works even with a **corrupted or missing config file** (uses default config for cleanup)
- Restores the original PAC URL for every network service that was modified
- Only clears the system PAC if it currently points to agent-proxy (won't clobber another tool's PAC)
- Removes the CLI env file

**Sample output:**

```
✓ Proxy disabled
  Current terminal: unset https_proxy http_proxy no_proxy NO_PROXY
```

---

### status

Quick health check with 6 checks. Exits with code 1 if any check fails.

```bash
agent-proxy status
```

**Checks performed:**

| Check | What it verifies |
|-------|-----------------|
| SSH (22) | ECS SSH port reachable (skipped in tunnel mode) |
| SSH tunnel | ControlMaster socket alive and responding |
| Proxy forwarding | HTTP request through proxy returns your ECS exit IP |
| System PAC | OS proxy settings point to agent-proxy PAC URL |
| PAC file | Generated PAC file exists with expected domain count |
| PAC HTTP server | Local PAC server responds with correct nonce |

**Sample output (all healthy):**

```
  ✓ SSH (22) (via tunnel — direct check skipped)
  ✓ SSH tunnel (127.0.0.1:18443 → 1.2.3.4:18443)
  ✓ Proxy forwarding (exit IP: 1.2.3.4)
  ✓ System PAC (http://127.0.0.1:18080/proxy.pac)
  ✓ PAC file (61 domains)
  ✓ PAC HTTP server (127.0.0.1:18080)

  Result: 6 passed, 0 failed
```

**Sample output (tunnel down):**

```
  ✓ SSH (22) (via tunnel — direct check skipped)
  ✗ SSH tunnel (not running — run: agent-proxy on)
  ✗ Proxy forwarding (dial tcp 127.0.0.1:18443: connect: connection refused)
  ✓ System PAC (http://127.0.0.1:18080/proxy.pac)
  ✓ PAC file (61 domains)
  ✓ PAC HTTP server (127.0.0.1:18080)

  Result: 4 passed, 2 failed
```

---

### doctor

Full diagnostics with actionable fix suggestions. Includes everything in `status` plus SNI detection, Squid binding verification, and no_proxy coverage analysis.

```bash
agent-proxy doctor
agent-proxy doctor --fix    # Auto-fix no_proxy issues
```

| Flag | Description |
|------|-------------|
| `--fix` | Automatically add flagged Chinese domains to `no_proxy` and regenerate `env.sh` |

**What it does beyond `status`:**

1. Prints config summary (path, presets, whitelist count, no_proxy count)
2. Runs all `status` checks with fix suggestions for each failure
3. Detects GFW SNI filtering (direct mode only) by testing a TLS handshake through the proxy
4. Verifies ECS Squid is loopback-only (tunnel mode only)
5. Fetches the last 500 Squid access log lines and flags Chinese/domestic domains routing through the proxy

**Sample output:**

```
=== agent-proxy doctor ===

Config:     /Users/you/.config/agent-proxy/config.yaml
Presets:    [ai dev search cloud media]
Whitelist:  61 domains (56 from presets, 5 custom)
No-proxy:   32 entries
Proxy:      1.2.3.4:18443 (SSH tunnel enabled)

--- Connectivity ---
  ✓ SSH (22) (via tunnel — direct check skipped)
  ✓ SSH tunnel (127.0.0.1:18443 → 1.2.3.4:18443)
  ✓ Proxy forwarding (exit IP: 1.2.3.4)
  ✓ System PAC (http://127.0.0.1:18080/proxy.pac)
  ✓ PAC file (61 domains)
  ✓ PAC HTTP server (127.0.0.1:18080)

--- Diagnosis ---
  → no_proxy coverage... ✓

✓ Everything looks good!
```

**Sample output with flagged domains:**

```
--- Diagnosis ---
  → no_proxy coverage... ⚠ 2 Chinese domain(s) routing through proxy:
    • cdn.jsdelivr.net
    • registry.npmmirror.com
    Fix: agent-proxy doctor --fix   (auto-add to no_proxy)
    Or:  edit config.yaml → no_proxy → add these domains → agent-proxy on
```

---

## Setup Commands

### init

Interactive first-time setup wizard. Walks through server connection, SSH tunnel choice, host key verification, Squid deployment, local activation, and connectivity verification.

```bash
agent-proxy init
```

See the [Quick Start](quickstart.md) guide for a detailed walkthrough with expected output at each step.

**Validation performed:**

- Server IP must not contain `://` or spaces
- SSH key must exist (auto-fixes permissions to `0600`)
- Detects macOS TCC-protected directories and offers to copy the key
- Port must be 1–65535
- Host key fingerprint requires explicit `yes` confirmation

---

### setup

Deploy or redeploy Squid on your ECS. Idempotent — safe to run multiple times.

```bash
agent-proxy setup
```

**Deployment steps:**

1. **Connectivity** — verifies SSH access
2. **Install Squid** — via apt, yum, or apk (skips if already installed)
3. **System tuning** — enables `tcp_fastopen` (best-effort, non-fatal)
4. **Write Squid config** — transactional deployment:
   - Writes to a temp file on the ECS
   - Runs `squid -k parse` syntax check
   - Backs up existing config
   - Atomic replace via `mv`
   - Restarts Squid and health-checks
   - **Rolls back automatically** if restart fails
5. **Cleanup legacy** — removes old Basic auth password files

**Sample output:**

```
Deploying to 1.2.3.4:18443...

  → Connectivity... ✓
  → Install Squid... ✓
  → System tuning... ✓
  → Write Squid config... ✓
  → Cleanup legacy... ✓

✓ Setup complete. Next: agent-proxy on
```

---

### trust-host

Verify and trust the ECS SSH host key. Fetches the key via `ssh-keyscan`, displays the SHA256 fingerprint for manual verification, and stores it in the project-specific `known_hosts`.

```bash
agent-proxy trust-host
```

**When to use:**

- After upgrading from a version that used `~/.ssh/known_hosts`
- After ECS host key rotation (e.g., server rebuild)
- If you see `host not in project known_hosts` errors

**Sample output:**

```
  → 获取 1.2.3.4 主机密钥... ✓

  主机密钥指纹:
    256 SHA256:xR3jKq9mN2vB8wY5tL7pA1cD4fG6hJ0kM3nQ9sU2wX8 (ED25519)
    3072 SHA256:aB1cD2eF3gH4iJ5kL6mN7oP8qR9sT0uV1wX2yZ3aB4c (RSA)

  请核对以上 SHA256 指纹与 ECS 控制台一致。
  输入 yes 确认: yes
  ✓ 已保存到 /Users/you/.config/agent-proxy/known_hosts
```

If the host key is already in the project `known_hosts`, it reports success immediately. If found in the system `~/.ssh/known_hosts`, it offers to migrate the entry.

---

### ip

Refresh the Squid IP whitelist with your current public IP. **Only relevant in direct mode** (when `proxy.tunnel: false`).

```bash
agent-proxy ip
```

In tunnel mode, this command prints an informational message and exits:

```
Tunnel mode: Squid listens on 127.0.0.1 only — no IP whitelist needed.
Your proxy traffic is encrypted via SSH; no public Squid port is exposed.
```

In direct mode, it fetches your public IP from `ipinfo.io`, regenerates the Squid config with the new IP in the `trusted_ip` ACL, and redeploys transactionally.

---

## Diagnostics Commands

### stats

Show proxy traffic statistics from Squid access logs.

```bash
agent-proxy stats
agent-proxy stats -n 500    # Analyze last 500 lines (default: 1000)
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--lines` | `-n` | `1000` | Number of log lines to analyze |

**Sample output:**

```
Fetching last 1000 log entries from 1.2.3.4...

  Requests: 847
  Total traffic: 156.2 MB

  Top 15 domains by traffic:
  Domain                                        Requests  Traffic
  --------------------------------------------- -------- --------
  chatgpt.com                                        412  112.3 MB
  api.openai.com                                      89   28.1 MB
  github.com                                          67    8.4 MB
  api.anthropic.com                                   45    4.2 MB
  stackoverflow.com                                   34    1.8 MB
  npmjs.com                                           28    0.9 MB
  pypi.org                                            12    0.3 MB
  baidu.com                                            3   12.1 KB 🇳

  🇨 Chinese traffic: 3 requests, 12.1 KB (0% of total)
```

The 🇳 marker flags domains that appear to be Chinese/domestic. If Chinese traffic exceeds 10% of total, a warning suggests running `agent-proxy doctor`.

---

### bench

Benchmark proxy vs direct latency for configured domains.

```bash
agent-proxy bench
agent-proxy bench chatgpt.com github.com    # Specific domains
agent-proxy bench -n 5                      # 5 runs per domain (default: 3)
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--runs` | `-n` | `3` | Number of requests per domain per mode |

Default domains tested: `chatgpt.com`, `openai.com`, `api.anthropic.com`, `github.com`.

**Sample output:**

```
Benchmarking 4 domains × 3 runs (proxy vs direct)...

  Domain                    Mode       TTFB    Total     Status
  chatgpt.com               proxy      350ms    352ms       ✓
  chatgpt.com               direct       -        -         ✗
  openai.com                proxy      340ms    345ms       ✓
  openai.com                direct       -        -         ✗
  api.anthropic.com         proxy      380ms    382ms       ✓
  api.anthropic.com         direct       -        -         ✗
  github.com                proxy      320ms    330ms       ✓
  github.com                direct     235ms    240ms       ✓
```

A `✗` for direct mode means the domain is unreachable without the proxy (expected for blocked sites).

---

### trace

Network path trace from your machine through the ECS to a target domain.

```bash
agent-proxy trace
agent-proxy trace github.com    # Default: chatgpt.com
```

**What it shows:**

1. **DNS Resolution** — resolves both the ECS host and target domain, showing IPs and lookup time
2. **Local → ECS** — TCP connect time and TLS handshake to your ECS
3. **ECS → Target** — TCP connect time from the ECS to the target domain (via SSH)

**Sample output:**

```
=== Network Trace ===

--- DNS Resolution ---
  1.2.3.4                     → 1.2.3.4          (2ms)
  chatgpt.com                 → 104.18.6.192     (45ms)

--- Local → ECS (1.2.3.4) ---
  TCP connect:    32ms
  TLS handshake:  68ms
  Total:          100ms

--- ECS → chatgpt.com ---
  TCP connect:    12ms
  TLS handshake:  28ms
  Total:          40ms
```

Use this to identify whether latency is in the local→ECS hop or the ECS→target hop.

---

### config-validate

Validate the config file for syntax errors and invalid values without making any changes.

```bash
agent-proxy config-validate
```

**Validation rules checked:**

- `proxy.host` is set
- `proxy.port` is 1–65535
- `proxy.ssh_key` is set when tunnel is enabled (or `SSH_AUTH_SOCK` is available)
- All preset names are valid (`ai`, `dev`, `search`, `cloud`, `media`)
- All custom domains match the domain regex

**Sample output:**

```
✓ Config valid: /Users/you/.config/agent-proxy/config.yaml
  Host: 1.2.3.4:18443
  Tunnel: true
  Presets: [ai dev search cloud media]
  Whitelist: 61 domains
  No-proxy: 32 entries
  Fallback: 5.6.7.8
```

---

## Management Commands

### whitelist

Manage custom domains in the proxy whitelist. Aliases: `wl`.

```bash
agent-proxy whitelist add <domain> [domain...]
agent-proxy whitelist rm <domain> [domain...]
agent-proxy whitelist ls
```

Aliases for `rm`: `remove`, `del`.
Aliases for `ls`: `list`.

**`add` — Add one or more custom domains:**

```bash
$ agent-proxy whitelist add example.com my-tool.io
  + example.com
  + my-tool.io

✓ 2 added, PAC regenerated
```

If a domain already exists (in presets or custom), it's skipped:

```bash
$ agent-proxy whitelist add github.com
  = github.com (already exists)
```

**`rm` — Remove custom domains:**

```bash
$ agent-proxy whitelist rm example.com
  - example.com

✓ 1 removed, PAC regenerated
```

Preset domains cannot be removed via `whitelist rm` — use `preset disable` instead:

```bash
$ agent-proxy whitelist rm github.com
  ? github.com (not in custom; may be in a preset)
```

**`ls` — List all effective domains:**

```bash
$ agent-proxy whitelist ls
Effective whitelist: 63 domains

[✓] ai         AI services (OpenAI, Anthropic, Google AI, OpenRouter, Copilot) (18 domains)
[✓] dev        Developer tools (GitHub, StackOverflow, package registries) (18 domains)
[✓] search     Search engines and knowledge bases (16 domains)
[✓] cloud      Cloud provider docs and consoles (AWS, GCP, Azure) (8 domains)
[✓] media      Video and social media (YouTube, Twitter/X, Instagram) (14 domains)

[✓] custom     User-added (2):
             example.com
             my-tool.io
```

Both `add` and `rm` automatically save the config and regenerate the PAC file. If the PAC server is running with hot-reload, changes take effect immediately.

---

### preset

Toggle preset domain groups on or off.

```bash
agent-proxy preset ls
agent-proxy preset enable <name>
agent-proxy preset disable <name>
```

Alias for `disable`: `rm`.

**`ls` — List all presets with their domains:**

```bash
$ agent-proxy preset ls
Available presets:

  ai         [enabled] AI services (OpenAI, Anthropic, Google AI, OpenRouter, Copilot)
             chatgpt.com
             openai.com
             oaistatic.com
             oaiusercontent.com
             anthropic.com
             claude.ai
             ...

  dev        [enabled] Developer tools (GitHub, StackOverflow, package registries)
             github.com
             githubusercontent.com
             ...

  search     [enabled] Search engines and knowledge bases
             ...

  cloud      [enabled] Cloud provider docs and consoles (AWS, GCP, Azure)
             ...

  media      [enabled] Video and social media (YouTube, Twitter/X, Instagram)
             ...
```

**`disable` — Turn off a preset:**

```bash
$ agent-proxy preset disable media
✓ Preset 'media' disabled
```

**`enable` — Turn a preset back on:**

```bash
$ agent-proxy preset enable media
✓ Preset 'media' enabled
```

Both `enable` and `disable` save the config and regenerate the PAC file.

---

### update

Self-update to the latest release with SHA-256 verification.

```bash
agent-proxy update
```

The update downloads the install script from the **current version's release tag** (not the mutable `main` branch) to avoid executing unverified code. The install script then:

1. Detects your OS and architecture
2. Downloads the release binary
3. Verifies the SHA-256 checksum (fail-closed)
4. Replaces the current binary

---

### version

Print the current version.

```bash
$ agent-proxy version
agent-proxy v0.7.2
```

---

## Hidden Commands

### serve-pac

Run the PAC HTTP server in the foreground. Used internally by `agent-proxy on` to launch the PAC daemon. Not intended for manual use.

```bash
agent-proxy serve-pac    # Blocks until interrupted
```

In foreground mode, the server also starts a config watcher that polls `config.yaml` every 5 seconds and regenerates the PAC file + `env.sh` when the config changes (hot-reload).
