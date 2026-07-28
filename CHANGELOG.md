# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
