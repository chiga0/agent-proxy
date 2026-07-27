# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-07-27

### Added
- Domain preset system (`ai`, `dev`, `search`) — enabled by default, zero-config
- `agent-proxy preset ls` — list available presets and their domains
- `agent-proxy preset enable/disable` — toggle preset groups
- `agent-proxy init` — interactive first-time setup
- `agent-proxy doctor` — comprehensive diagnostics
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
