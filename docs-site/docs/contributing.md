# Contributing

Thanks for your interest in contributing to agent-proxy!

## Development Setup

```bash
git clone https://github.com/chiga0/agent-proxy.git
cd agent-proxy
go mod tidy
make build
```

## Project Structure

```
cmd/agent-proxy/       CLI entry point (cobra commands)
internal/config/       Config loading/saving, presets, validation
internal/pac/          PAC generation, HTTP server, nonce, hot-reload
internal/proxy/        Proxy on/off/status/doctor lifecycle
internal/platform/     OS-specific code (build-tagged: darwin/linux/windows)
internal/ecs/          SSH-based Squid deployment, log parsing
internal/tunnel/       SSH tunnel management via ControlMaster
internal/bench/        Proxy vs direct latency benchmarking
internal/trace/        Network path tracing
test/squid/            Docker-based Squid ACL integration tests
```

## Before Submitting

Run the full check suite:

```bash
go build ./...          # Must compile
go test ./...           # All tests pass
go test -race ./...     # No race conditions
go vet ./...            # No vet warnings
gofmt -l .              # No unformatted files (must be empty)
```

CI runs all of the above on ubuntu, macOS, and Windows, plus Docker-based Squid ACL integration tests.

## Guidelines

- **Minimal dependencies** — prefer stdlib; no new deps without discussion
- **No secrets** — no hardcoded IPs, credentials, or personal info
- **Cross-platform** — use `internal/platform/` build tags for OS-specific code; never call `pgrep`/`wmic`/`networksetup` directly from shared code
- **Windows compatibility** — always set `USERPROFILE` in tests; skip Unix permission checks with `runtime.GOOS != "windows"`
- **Error propagation** — never silently swallow errors; return them or log with `⚠` prefix
- **Tests** — new features need tests; bug fixes need regression tests

## Pull Requests

1. Fork the repo
2. Create a feature branch
3. Make your changes with tests
4. Run the full check suite above
5. Submit a PR with a clear description of what and why
