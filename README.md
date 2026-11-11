<div align="center">

# MikroTik MCP Server

**Let AI assistants manage MikroTik routers over the native RouterOS API: one static binary, any MCP client.**

[![CI](https://img.shields.io/github/actions/workflow/status/Delnegend/mikrotik-mcp-server/ci.yml?branch=main&style=flat-square)](https://github.com/Delnegend/mikrotik-mcp-server/actions)
[![Release](https://img.shields.io/github/v/release/Delnegend/mikrotik-mcp-server?style=flat-square)](https://github.com/Delnegend/mikrotik-mcp-server/releases)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)

</div>

---

## Quick Start

```bash
# 1. Install
brew install Delnegend/tap/mikrotik-mcp

# 2. Register it with your MCP client (opencode shown; 3 more in docs)
# opencode.json -> "mcp": { "mikrotik": {
#   "type": "local", "command": ["mikrotik-mcp", "192.168.88.1"],
#   "environment": { "MIKROTIK_USER": "admin", "MIKROTIK_PASSWORD": "<…>" } } }

# 3. Verify
mikrotik-mcp -version
```

Then ask your assistant to run `list_devices`; a healthy server answers with your fleet.

## Highlights

- **Full router control from chat**: interfaces, routing, DHCP, DNS, firewall, WireGuard, and more.
- **Fleet-ready**: manage many routers from one server; each call targets a device by title.
- **Safe by default**: TLS verified, SSH host keys pinned, and Safe Mode holds changes until you commit.
- **One binary, zero daemons**: speaks MCP over stdio; release archives also ship `rosbackup`.

## Documentation

- **[Architecture](docs/architecture.md)**: Components, safe mode console session, and protocol invariants.
- **[Development](docs/development.md)**: Building from source, running unit tests, and live-Router CHR integration suite.
- **[Deployment](docs/deployment.md)**: MCP client configuration, Homebrew tap, and prebuilt binary installation.

## License

[MIT](LICENSE)
