# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.7.5] - 2026-07-30


### Added

- Automated release flow — git-cliff + make release
## [0.7.4] - 2026-07-30


### Added

- Domain rule subscriptions and proxy health auto-recovery
- Wire autostart CLI + launchctl load + doctor check

### Changed

- Extract setup package, add health monitoring status warning

### Fixed

- Windows CI — set USERPROFILE in tests, skip Unix perm checks
- Audit findings — 15 fixes across health, rules, config, dashboard
- Fallback tunnel management, second-pass audit cleanup
- Serve-pac sets system PAC on startup for autostart recovery
- Doctor forwarding check — use /ip endpoint, bump timeout to 20s
- Deploy hardening + comprehensive docs rewrite
## [0.7.3] - 2026-07-30


### Added

- Windows env files, boost test coverage (pac 84%, proxy 56%)
- Add log command, platform tests (13→43%), PR template
## [0.7.2] - 2026-07-30


### Changed

- Remove all legacy compat code and internalize English UI

### Fixed

- Docs deploy path — mkdocs outputs to docs-site/site/
- Merge new no_proxy defaults on load and add npm proxy to env.sh
## [0.7.1] - 2026-07-29


### Added

- Env.sh hot-reload, config-validate, SSH agent, doctor --fix, templates
- Web management dashboard at /dashboard with /api/status and /api/stats
- Prometheus metrics endpoint on PAC server (/metrics)

### Release

- V0.7.1 — dashboard, metrics, docs site, SLSA, doctor --fix, env hot-reload
## [0.7.0] - 2026-07-29


### Added

- Doctor no_proxy check, stats command, PAC hot-reload
- Homebrew tap via GoReleaser + cosign release signing
- README rewrite + multi-ECS failover

### Fixed

- Add .aliyuncs.com to default no_proxy — prevents Alibaba Cloud traffic from routing through proxy
- Windows CI — USERPROFILE in tests, skip perm check, use config.EnvPath
- Homebrew formula to same repo (Formula/ dir), no separate tap needed

### Release

- V0.7.0 — stats, doctor audit, PAC hot-reload, failover, Homebrew, signing
## [0.6.1] - 2026-07-29


### Fixed

- Trust-host blocked by own guard; install.sh tmp_dir unbound variable
## [0.6.0] - 2026-07-28


### Fixed

- Upgrade Go 1.22 → 1.24, fixes macOS CI dyld LC_UUID crash
- Windows CI — set USERPROFILE in tests, add nil guard for os.Stat
- Skip Unix permission check on Windows in config tests
- Re-audit — PAC idempotency, strict tunnel check, metadata ACL, fail-closed checksum
- Third audit — PAC state machine, Squid parse, WMIC, platform state
- Exclude PowerShell self-match in FindPIDsByPattern
- Fourth audit — restore order, error propagation, CLI-only, regex
- Fifth audit — fingerprint, known_hosts migration, CLI-only, nonce bind

### Release

- V0.6.0 — security audit hardening

### Security

- Sanitize sensitive info from docs and strengthen .gitignore
- Audit fix — strict tunnel boundary, deny-first ACL, remove Basic auth
- P2 hardening — SSH TOFU fix, PAC nonce, Actions SHA pin
## [0.5.2] - 2026-07-28


### Fixed

- Audit P0-P2 — Windows compat, validation, CI, tests, update cmd (#6)
## [0.5.1] - 2026-07-28


### Performance

- Split Google wildcards into specific domains for faster page loads (#5)

### Revert

- Remove WireGuard/udp2raw tunnel — keep SSH optimizations only (#4)
## [0.5.0] - 2026-07-28


### Added

- WireGuard tunnel with auto-fallback, SSH ControlMaster, TFO, Squid tuning (#3)
## [0.4.4] - 2026-07-28


### Fixed

- PAC daemon isolation, migration cleanup, tunnel perf tuning (#2)
## [0.4.3] - 2026-07-28


### Fixed

- Service ownership, PID safety, Validate, vet, deploy guard
## [0.4.2] - 2026-07-28


### Fixed

- P2 hardening — port split, escaping, Windows autostart, bench, tests
## [0.4.1] - 2026-07-27


### Fixed

- Add Google CDN domains (gstatic, googleusercontent) to search preset
- Security hardening, lifecycle reliability, diagnostic accuracy
## [0.4.0] - 2026-07-27


### Added

- SSH tunnel, media preset, auth optional, init wizard, auto-start
## [0.3.2] - 2026-07-27


### Added

- OSS mirror for China users + STS upload workflow
## [0.3.1] - 2026-07-27


### Added

- Bench + trace commands, comprehensive README for v0.3
- Add install.sh for curl-pipe-bash installation

### Changed

- Rename prevVal/hasPrev → pendingRttVal/hasPendingRtt for clarity

### Fixed

- Httptrace for real timing, fix traceroute RTT parser, harden SSH host key check
- Add Go module proxy domains to no_proxy defaults
- MacOS Gatekeeper quarantine removal + troubleshooting docs
## [0.3.0] - 2026-07-27


### Added

- **v0.3**: Presets, init, doctor, changelog, collaboration templates
## [0.2.0] - 2026-07-27


### Added

- Cross-platform support (macOS/Linux/Windows)
## [0.1.0] - 2026-07-27


### Added

- Initial release — domain-based selective proxy CLI
- Add tests, GoReleaser, release workflow, repo metadata
