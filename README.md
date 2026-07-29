# mikrotik-mcp-server

MCP server for managing MikroTik routers through the RouterOS API.

## Prerequisites

- A MikroTik router running **RouterOS 7+** with SSH or API access enabled
- Go 1.23+ (if building from source)

## Quick Start

```bash
# Option 1 — download the latest binary from Releases and place it in your PATH
# Option 2 — install with Go
go install github.com/Delnegend/mikrotik-mcp-server@latest

# Run the server directly (for testing):
mikrotik-mcp <router-host>
```

Then add it to your MCP client:

<details>
<summary>opencode (<code>opencode.json</code>)</summary>

```json
{
  "mcp": {
    "mikrotik": {
      "type": "local",
      "command": ["mikrotik-mcp", "192.168.88.1"],
      "enabled": true,
      "environment": {
        "MIKROTIK_USER": "admin",
        "MIKROTIK_PASSWORD": "your-password"
      }
    }
  }
}
```
</details>

<details>
<summary>Claude Code (<code>.mcp.json</code>)</summary>

```json
{
  "mcpServers": {
    "mikrotik": {
      "command": "mikrotik-mcp",
      "args": ["192.168.88.1"],
      "env": {
        "MIKROTIK_USER": "admin",
        "MIKROTIK_PASSWORD": "your-password"
      }
    }
  }
}
```
</details>

<details>
<summary>VS Code (<code>.vscode/mcp.json</code>)</summary>

```json
{
  "servers": {
    "mikrotik": {
      "type": "stdio",
      "command": "mikrotik-mcp",
      "args": ["192.168.88.1"],
      "env": {
        "MIKROTIK_USER": "admin",
        "MIKROTIK_PASSWORD": "your-password"
      }
    }
  }
}
```
</details>

<details>
<summary>Zed (<code>settings.json</code>)</summary>

```json
{
  "mcp": {
    "mikrotik": {
      "command": "mikrotik-mcp",
      "args": ["192.168.88.1"],
      "env": {
        "MIKROTIK_USER": "admin",
        "MIKROTIK_PASSWORD": "your-password"
      }
    }
  }
}
```
</details>

### Environment Variables

Configuration is read from env vars or a `.env` file. SCP variables are only needed for downloading backups or using passwordless API auth.

| Variable | Required | Default | Description |
|---|---|---|---|
| `MIKROTIK_USER` | **yes** | — | RouterOS API username |
| `MIKROTIK_PASSWORD` | **yes**¹ | — | RouterOS API password |
| `MIKROTIK_API_SSL` | no | `true` | Use TLS for the RouterOS API |
| `MIKROTIK_API_TLS_VERIFY` | no | `true` | Verify the TLS certificate |
| `MIKROTIK_API_PORT` | no | `8729` (SSL) / `8728` (plain) | RouterOS API port |
| `MIKROTIK_API_PASSWORDLESS_ENABLED` | no | `false` | Rotate API password on each startup via SCP |
| `MIKROTIK_SCP_HOST` | no | *API host* | SCP server for backup downloads |
| `MIKROTIK_SCP_PORT` | no | *RouterOS default* | SCP port |
| `MIKROTIK_SCP_USER` | no | `MIKROTIK_USER` | SCP username |
| `MIKROTIK_SCP_PASSWORD` | no | `MIKROTIK_PASSWORD` | SCP password |
| `MIKROTIK_SCP_PRIVATE_KEY` | no | — | Path to an SCP private key (replaces password) |
| `MIKROTIK_SCP_KEY_PASSPHRASE` | no | — | Passphrase for the private key |
| `MIKROTIK_SCP_HOST_FINGERPRINT_SHA256` | no | — | Expected SCP host key fingerprint (`SHA256:...`) |

¹ Not required when `MIKROTIK_API_PASSWORDLESS_ENABLED=true`.

## Development

```bash
go run . <router-host>   # run the server
go test ./...             # run all tests
go fmt ./...              # format code
```

## License

MIT
