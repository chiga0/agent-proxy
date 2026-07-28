# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security (P2 hardening)
- **SSH host key verification**: `init` now fetches the ECS host key via `ssh-keyscan`, displays the fingerprint for user confirmation, and stores it in a project-specific `known_hosts` (`~/.config/agent-proxy/known_hosts`). All SSH connections use `StrictHostKeyChecking=yes` with this file (no more TOFU `accept-new`).
- **PAC server nonce**: static `X-Agent-Proxy: pac` header replaced with a per-start random 128-bit nonce persisted to disk; `ServerRunning()` verifies the nonce, distinguishing agent-proxy from unrelated HTTP services on the same port (not a same-user security boundary).
- **GitHub Actions pinned to commit SHA**: `actions/checkout`, `actions/setup-go`, and `goreleaser/goreleaser-action` pinned to immutable SHAs with version comments.
- **Squid temp file validation**: strict regex `^/etc/squid/squid\.conf\.[A-Za-z0-9]+$` replaces prefix+char check.
- **System tuning**: failures now display a visible warning instead of faking success.
- **Doctor**: SSH errors during Squid mode check reported as warnings, not silently ignored.
- **Autostart cleanup**: `UninstallAutoStart()` returns aggregated errors; `off` reports them.
- **Whitelist migration**: `Save()` error checked and warned.
- **All autostart SSH args**: updated to `StrictHostKeyChecking=yes` + project `UserKnownHostsFile`.

### Security (Phase A — audit P0)
- **Tunnel mode is now a strict security boundary**: Squid listens on `127.0.0.1` only (`http_port 127.0.0.1:<port>`), no public IP is fetched or whitelisted, and the `ip` command is disabled in tunnel mode
- **Squid ACL rewritten to deny-first**: `deny !Safe_ports` → `deny CONNECT !SSL_ports` → `deny to_localhost/to_linklocal/to_rfc1918/to_metadata` → `allow trusted_ip` → `deny all`
- **Init flow reordered**: tunnel/direct choice is now made *before* Squid deployment, so the Squid config is generated with the correct security model from the start
- **Removed Basic auth entirely**: `user`/`password` fields, `HasAuth()`, `configureAuth()`, `htpasswd`, `basic_ncsa_auth` ACLs, and Proxy-Authorization header injection are all removed
- **Public IP fetch hardened**: uses Go HTTP client with environment proxy disabled, validates HTTP status, response size, IP format, and rejects loopback/private/link-local addresses
- **Squid config deployment is transactional**: temp file → syntax check → backup → atomic replace → restart → health check → rollback on failure
- **Strict config validation**: mutating commands (`on`, `off`, `setup`, `ip`) now fail on invalid config instead of warning

### Security (Phase B — audit P1)
- **PAC state save/restore**: `on` saves the original system PAC URL and enabled state; `off` restores it instead of blindly clearing; only modifies PAC if it still belongs to agent-proxy
- **SSH ControlPath management**: tunnel uses `ssh -O check/-O exit` via ControlMaster socket for precise lifecycle control, replacing fragile PID-file + pattern-matching approach
- **PAC server identity**: responses include `X-Agent-Proxy: pac` header; `ServerRunning()` verifies the header to avoid false positives from other HTTP services on the same port
- **Windows tunnel autostart**: added `AgentProxyTunnel` scheduled task alongside the existing PAC task
- **Autostart error propagation**: macOS/Linux autostart now returns errors for directory creation, file writes, and service registration instead of silently swallowing them
- **Unified SSH parameters**: all SSH consumers (tunnel, deploy, autostart, trace) use shared `SSHBaseArgs()`/`SSHTarget()` from config; autostart uses `BatchMode=yes`, matching ciphers and ControlPath
- **Autostart logs**: moved from fixed `/tmp/*.log` to `~/.config/agent-proxy/logs/`

### Security (Phase C — audit P2)
- **Install script verifies SHA-256 checksums** from release `checksums.txt` before extraction
- **`update` command** uses the versioned release tag URL instead of executing the mutable `main` branch script
- **CI**: added `gofmt` check and `go test -race` (non-Windows)
- **Formatted** `internal/bench/bench.go` and `internal/trace/trace.go`
- **Docs**: README security section rewritten to describe SSH tunnel as the security boundary; CLI vs PAC routing semantics clarified; shell autoload instructions corrected; Go 1.24+ badge

### Removed
- `ProxyConfig.User` / `ProxyConfig.Password` fields
- `Config.HasAuth()` / `Config.ProxyURLNoAuth()`
- `ecs.configureAuth()` / `ecs.sshRunWithStdin()`
- Squid `auth_param` / `basic_ncsa_auth` / `authenticated` ACL generation
- `apache2-utils` / `httpd-tools` from Squid install (no longer needed for htpasswd)
- Proxy-Authorization header in SNI detection
- 407 auth check in status forwarding test
- PID file management in tunnel (replaced by ControlPath)

## [0.5.2] - 2026-07-28

### Fixed
- **P0**: Deleted stale `docs/known-issues.md` (all listed issues already fixed)
- **P0**: Removed `proxy.golang.org` from dev preset — was in both dev (PAC → proxy) and `no_proxy` (CLI → direct), causing split-brain routing
- **P0**: Replaced Unix-only `grep`/`pgrep`/`ps` shell calls with cross-platform `internal/platform` helpers (build-tagged `proc_unix.go` / `proc_windows.go`)
- **P1**: `os.UserHomeDir()` error handling with fallback warning instead of silent empty path
- **P1**: `init` wizard validates port input and rejects URL-style server addresses
- **P1**: `installSquid` detects `apt`/`yum`/`apk` package managers (was Debian-only)
- **P1**: Replaced `checkPACFile` shell `grep` with pure Go `strings.Count`
- **P1**: Fixed `isProcessAlive` on Windows (uses platform helper instead of `kill -0`)

### Added
- CI matrix: ubuntu + macOS + Windows (was ubuntu-only)
- `--verbose` / `-v` global flag
- `agent-proxy update` self-update command
- Tests for `platform`, `proxy`, `tunnel` packages (3 new test files)

### Changed
- `SECURITY.md` supported versions updated to 0.4.x / 0.5.x

## [0.5.1] - 2026-07-28

### Changed
- Split Google wildcards (`google.com`, `gstatic.com`, `googleusercontent.com`) into specific blocked services — CDN domains now go direct for speed
- Added `clients6.google.com` and `gemini.gstatic.com` to ai preset for Gemini API
- Removed `google-analytics.com` and `googletagmanager.com` from search preset (tracking scripts accessible from China)
- Removed WireGuard/udp2raw tunnel code (adds complexity without benefit in UDP-blocked networks)

### Performance
- Gemini page load: 15s → 3.1s (5x faster) by letting CDN resources load direct

## [0.5.0] - 2026-07-28

### Added
- SSH ControlMaster/ControlPersist=600 for connection multiplexing
- TCP Fast Open (TFO) on ECS — saves 1 RTT per TCP handshake
- Squid `collapsed_forwarding on` — deduplicates concurrent identical requests

### Changed
- SSH tunnel: `ServerAliveInterval=30` (was 60), `IPQoS=throughput`, `TCPKeepAlive=yes`

### Performance
- Tunnel TTFB variance: 1.52s → 0.04s (97% reduction)
- 5s+ outlier eliminated
- Average tunnel TTFB: 2.46s → 1.50s (39% faster)

## [0.4.4] - 2026-07-28

### Fixed
- **P0**: PAC server daemon now uses `Setsid` (Unix) / `CREATE_NEW_PROCESS_GROUP` (Windows) — survives terminal close and shell exit
- **P0**: `stopPACDaemon()` cleans up legacy `__pac-server` processes from v0.3.x — prevents port 18080 conflict after upgrade
- **P1**: `ServerRunning()` health check uses `Transport{Proxy: nil}` — avoids proxy env var loop when `NO_PROXY` missing `127.0.0.1`
- **P1**: `doctor` skips raw SSH port check in tunnel mode — eliminates false-negative `✗ SSH (22)` when tunnel is working

### Changed
- SSH tunnel: prefer `aes128-gcm@openssh.com` (ARM hardware acceleration), `Compression=no` (HTTPS already compressed), `ServerAliveInterval=30` (faster dead-peer detection), `IPQoS=throughput` (DSCP marking)
- Squid: `pconn_timeout 2 minutes`, `half_closed_clients off`, DNS cache (`positive_dns_ttl 1h`), `max_filedescriptors 65536`
- Added design doc: `docs/network-optimization.md`

## [0.4.3] - 2026-07-28

### Fixed
- `Config.Validate()` now called in `Load()` (warns) and `init` (blocks) — catches bad port, half-set credentials, missing SSH key
- Eliminated double-start: `InstallAutoStart` only writes boot config files, no longer calls `launchctl load` / `systemctl start` — `On()` solely manages current-session processes
- `tunnel.Start` / `startPACDaemon` return `(started bool, err error)` — rollback only stops resources this call actually started
- PID kill verifies process identity (`ps -o comm=` for ssh, `ps -o args=` for serve-pac) before killing — prevents killing unrelated processes on PID reuse
- PAC start failure now kills the child process and removes stale PID file
- `Off()` aggregates cleanup errors and reports warnings instead of silently ignoring
- `writeSquidConfig` fails deployment in direct+no-auth mode when public IP is unavailable (would deploy an unreachable Squid)
- SNI detection sets connection deadline before CONNECT read (was blocking indefinitely)
- `conn.Write` error checked in SNI detection
- All address construction uses `net.JoinHostPort` — fixes `go vet` IPv6 warning
- `init` wizard calls `Validate()` before saving config

## [0.4.2] - 2026-07-28

### Added
- `proxy.tunnel_local_port` — separate local/remote tunnel ports (fixes local port conflicts)
- Windows auto-start via `schtasks` (ONLOGON trigger)
- Tests: Squid config generation, domain validation, shell quoting, LocalPort, Config.Validate

### Changed
- Plist values XML-escaped (paths with `&`, `<`, `>` no longer break LaunchAgent)
- Systemd unit values quoted per systemd escaping rules (`%` → `%%`, spaces handled)
- `tunnel.Running` checks PID file + process liveness before port check
- Benchmark shows success/attempts count, DNS avg, and last error on partial failure
- All `cfg.Save()` / `pac.Write()` calls in CLI commands now return errors

### Fixed
- Local port conflict when `proxy.port` is used by another service — use `tunnel_local_port` to override

## [0.4.1] - 2026-07-28

### Fixed
- **P0**: `installSquid` passed `nil` config → SSH to empty host; now passes `cfg`
- **P0**: `validateSSHKey` returned old path after TCC auto-copy; now returns final path
- **P0**: `env.sh` saved with `0644` exposing credentials; now `0600`
- Proxy URL credentials now properly URL-encoded via `url.UserPassword`
- Shell values in `env.sh` now single-quoted (safe against `$(...)`, spaces, etc.)
- `configureAuth` passes password via stdin (`htpasswd -cbi`), preventing remote command injection
- Proxy username validated against `[a-zA-Z0-9._-]+`
- Custom domain validation: only valid DNS names accepted (prevents PAC JS injection)
- SSH `StrictHostKeyChecking` changed from `no` to `accept-new`
- `On()` now rolls back completed steps on failure
- PAC daemon uses PID file instead of unreliable `pgrep -f`
- SSH tunnel uses PID file for lifecycle management
- `ServerRunning` now actually requests `/proxy.pac` (not just port check)
- Tunnel mode `status` skips public Squid port check (only checks SSH + local tunnel)
- SNI detection wired into `doctor` (was dead code)
- `InstallAutoStart` returns error instead of silently ignoring failures
- Config `Save()` is now atomic (temp file + rename)
- Added `Config.Validate()` for consistent config checking
- Version injected via ldflags (`-X main.version=`), no longer hardcoded
- Google CDN domains added: gstatic.com, googleusercontent.com, google-analytics.com, googletagmanager.com, ggpht.com

## [0.4.0] - 2026-07-28

### Added
- **Built-in SSH tunnel** — `proxy.tunnel: true` in config; `on`/`off` auto-manage the tunnel process, encrypting proxy traffic to bypass GFW SNI filtering
- **Media preset** — YouTube, Twitter/X, Instagram, Facebook, Telegram (14 domains)
- **Auto-start** — `on` registers LaunchAgent (macOS) / systemd user unit (Linux) for SSH tunnel + PAC server; survives reboot
- **PAC server daemon** — PAC HTTP server now runs as a detached background process (`serve-pac`), no longer dies when the command exits
- **`init` wizard redesign** — one command does everything: SSH check → Squid deploy → tunnel setup → PAC + env → auto-start → connectivity verify
- **PEM path validation** — warns when SSH key is in macOS TCC-protected directories (~/Documents, ~/Desktop), offers auto-copy to ~/.ssh/
- **`doctor` actionable diagnosis** — each failure now prints a specific fix command; SNI filtering detection
- `githubcopilot.com` added to ai preset (fixes Codex desktop)
- `HasAuth()`, `EffectiveHost()` config helpers

### Changed
- **Auth is now optional** — Squid trusts 127.0.0.1 (SSH tunnel) + IP whitelist by default; username/password only needed for direct mode without tunnel
- `proxy.user`/`proxy.password` are optional in config.yaml
- Squid config template always includes 127.0.0.1 in trusted IPs
- `status` shows SSH tunnel check when tunnel is enabled
- Default presets now include `media` (5 presets, 62 domains)
- Version bumped to v0.4.0

### Fixed
- PAC server no longer stops when `agent-proxy on` exits
- Browser proxy auth prompt (407) when using SSH tunnel — Squid now trusts localhost

## [0.3.0] - 2026-07-27

### Added
- Domain preset system (`ai`, `dev`, `search`, `cloud`) — enabled by default, zero-config
- 50+ pre-configured domains covering AI, dev tools, search, cloud providers
- `agent-proxy preset ls/enable/disable` — manage preset groups
- `agent-proxy init` — interactive first-time setup
- `agent-proxy doctor` — comprehensive diagnostics
- `agent-proxy bench` — benchmark proxy vs direct latency (TTFB, total RT)
- `agent-proxy trace` — network path trace (local → ECS → target)
- CHANGELOG.md
- Issue templates (bug report, feature request)
- Pull request template
- CODE_OF_CONDUCT.md
- SECURITY.md

### Changed
- Default whitelist now uses presets instead of flat list
- README expanded with preset documentation and troubleshooting

## [0.2.0] - 2026-07-27

### Added
- Cross-platform support: macOS, Linux, Windows
- Linux system proxy via gsettings (GNOME)
- Windows system proxy via registry (IE/Edge)
- Platform support matrix in README

### Changed
- PAC HTTP server rewritten in pure Go (removed python3 dependency)
- GoReleaser now builds 6 targets (darwin/linux/windows × amd64/arm64)

## [0.1.0] - 2026-07-27

### Added
- Initial release
- Cobra CLI: `on`, `off`, `status`, `whitelist`, `setup`, `ip refresh`
- PAC generation + local HTTP server for Chrome compatibility
- macOS system proxy via networksetup
- CLI environment variables with no_proxy exclusions
- SSH-based Squid deployment with auth + IP whitelist
- YAML config at `~/.config/agent-proxy/config.yaml`
- Unit tests for config and pac packages
- GoReleaser + GitHub Actions CI/CD
- MIT license
