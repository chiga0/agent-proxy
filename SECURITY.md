# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.5.x   | :white_check_mark: |
| 0.4.x   | :white_check_mark: |
| < 0.4   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it by opening a **private** issue or emailing the maintainer directly. Do **not** open a public issue for security vulnerabilities.

We will acknowledge receipt within 48 hours and provide a timeline for a fix.

## Security Considerations

- Proxy credentials are stored in `~/.config/agent-proxy/config.yaml` with `0600` permissions
- The config file is never committed to the repository
- Squid proxy requires authentication for non-whitelisted IPs
- The PAC HTTP server binds to `127.0.0.1` only (not exposed to network)
