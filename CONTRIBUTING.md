# Contributing

Thanks for your interest in contributing to agent-proxy!

## Development

```bash
git clone https://github.com/chiga0/agent-proxy.git
cd agent-proxy
go mod tidy
make build
```

## Project Structure

```
cmd/agent-proxy/       CLI entry point (cobra commands)
internal/config/       Config loading/saving (YAML)
internal/pac/          PAC file generation + local HTTP server
internal/proxy/        Proxy on/off/status logic
internal/platform/     OS-specific code (macOS networksetup)
internal/ecs/          SSH-based Squid deployment
```

## Guidelines

- Keep dependencies minimal
- No hardcoded IPs, credentials, or personal info
- Run `go vet ./...` and `go build ./...` before submitting
- Test on macOS (primary target platform)

## Pull Requests

1. Fork the repo
2. Create a feature branch
3. Make your changes
4. Ensure `make build` and `go vet ./...` pass
5. Submit a PR with a clear description
