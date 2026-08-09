# MikroTik MCP Server

A Go implementation of an **MCP (Model Context Protocol) server** for managing MikroTik routers through the RouterOS API.

This project is a **Go port** of the Python project [parkerkane/mikrotik-manager](https://github.com/parkerkane/mikrotik-manager) — an OpenCode workspace and MCP server for managing MikroTik routers. It retains the same feature surface, tool semantics, and environment-variable configuration, reimplemented in idiomatic Go with [mcp-go](https://github.com/mark3labs/mcp-go) as the MCP framework, and shipped as a single static binary per platform.

## Features

- RouterOS API client over TCP/TLS with optional certificate verification
- Passwordless API startup mode with SSH key-based password rotation
- Healthcheck covering API, SSH/SFTP, and passwordless readiness
- stdio MCP server — works with opencode, Claude Code, VS Code, Zed, and any MCP client
- Low-level RouterOS read/write tools (`resource_print`, `resource_add`, `resource_set`, `resource_remove`, `command_run`)
- Tools for system, interfaces, addresses, routes, DHCP, DNS, bridges, VLANs, firewall, PPP, and WireGuard
- Router file listing, download, export, and backup collection workflows
- Optional `jq_filter` support for normalized `resource_print` results
- Mocked Go test coverage for client, runtime, and tool behavior (no network required)

## Requirements

- A MikroTik router running **RouterOS 7+** with the API service enabled (`/ip/service set api disabled=no`)
- RouterOS credentials for a user with the required API permissions
- Go 1.25+ if building from source

## Installation

```bash
# Option 1 — download the matching archive for your platform from Releases
# (linux/darwin/windows × amd64/arm64, as .tar.xz or .7z)

# Option 2 — install with Go
go install github.com/Delnegend/mikrotik-mcp@latest

# Option 3 — install with Homebrew
brew tap Delnegend/tap
brew install mikrotik-mcp

# Option 4 — curl | tar, extract the binary straight into /usr/local/bin (Linux amd64)
curl -fsSL https://github.com/Delnegend/mikrotik-mcp-server/releases/latest/download/mikrotik-mcp-linux-amd64.tar.xz | sudo tar -xJf - -C /usr/local/bin
sudo mv /usr/local/bin/mikrotik-mcp-linux-amd64 /usr/local/bin/mikrotik-mcp
```

Replace `linux-amd64` with `linux-arm64` or `darwin-amd64` for other platforms. The archive contains a single binary named after its platform (e.g. `mikrotik-mcp-linux-amd64`), hence the rename so the MCP configs below can call it as `mikrotik-mcp`.

Run the server directly to verify it connects:

```bash
mikrotik-mcp <router-host>
mikrotik-mcp -version   # prints the embedded build version (e.g. mikrotik-mcp 0.1.0)
```

The `<router-host>` argument is required; it is the router's hostname or IP address.

## MCP Client Configuration

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

## Environment Variables

Configuration is read from environment variables, with a `.env` file in the working directory as a fallback. SCP variables are only needed for downloading backups or using passwordless API auth.

| Variable | Required | Default | Description |
|---|---|---|---|
| `MIKROTIK_USER` | **yes** | — | RouterOS API username |
| `MIKROTIK_PASSWORD` | **yes**¹ | — | RouterOS API password |
| `MIKROTIK_API_SSL` | no | `true` | Use TLS for the RouterOS API |
| `MIKROTIK_API_TLS_VERIFY` | no | `true` | Verify the TLS certificate |
| `MIKROTIK_API_PORT` | no | `8729` (SSL) / `8728` (plain) | RouterOS API port |
| `MIKROTIK_API_PASSWORDLESS_ENABLED` | no | `false` | Rotate API password on each startup via SCP |
| `MIKROTIK_API_PASSWORDLESS_LENGTH` | no | `32` | Generated API password length |
| `MIKROTIK_SCP_HOST` | no | *API host* | SCP server for backup downloads |
| `MIKROTIK_SCP_PORT` | no | `22` | SCP port |
| `MIKROTIK_SCP_USER` | no | `MIKROTIK_USER` | SCP username |
| `MIKROTIK_SCP_PASSWORD` | no | `MIKROTIK_PASSWORD` | SCP password |
| `MIKROTIK_SCP_PRIVATE_KEY` | no | — | Path to an SCP private key (replaces password) |
| `MIKROTIK_SCP_KEY_PASSPHRASE` | no | — | Passphrase for the private key |
| `MIKROTIK_SCP_HOST_FINGERPRINT_SHA256` | no | — | Expected SCP host key fingerprint (`SHA256:...`), required for passwordless mode |
| `MIKROTIK_SCP_TIMEOUT` | no | `30.0` | SCP timeout in seconds |

¹ Not required when `MIKROTIK_API_PASSWORDLESS_ENABLED=true` (SSH key auth replaces it).

Notes:

- TLS is enabled by default and the router certificate is verified; set `MIKROTIK_API_TLS_VERIFY=false` to disable (insecure).
- A `certs/` directory next to the working directory is loaded into the TLS trust store (`.pem`, `.crt`, `.cer` files; names ending in `.disabled` are ignored) for private CA setups.
- SSH host fingerprint verification is enforced when `MIKROTIK_SCP_HOST_FINGERPRINT_SHA256` is set; passwordless startup **requires** it and fails if it is missing, invalid, or mismatched.

## Tool Surface

The server exposes the following MCP tools, grouped by area:

**Generic RouterOS primitives**

`resource_print` · `resource_add` · `resource_set` · `resource_remove` · `command_run` · `resource_listen` · `command_cancel`

**System & health**

`healthcheck` · `system_resource_get` · `system_identity_get` · `system_clock_get`

**Interfaces, addressing & routing**

`interface_list` · `interface_get` · `interface_monitor` · `ip_address_list` · `ip_address_get` · `ip_route_list` · `ip_route_get`

**DHCP & DNS**

`dhcp_lease_list` · `dhcp_server_list` · `dhcp_network_list` · `dns_get` · `dns_set` · `dns_resolve`

**Files & backups**

`file_list` · `file_download` · `system_backup_save` · `system_export` · `system_backup_collect`

**Bridges & VLANs**

`bridge_list` · `bridge_add` · `bridge_remove` · `bridge_port_list` · `bridge_port_add` · `bridge_port_remove` · `bridge_vlan_list` · `bridge_vlan_add` · `bridge_vlan_remove` · `vlan_list` · `vlan_add` · `vlan_remove`

**Firewall**

`firewall_filter_list` · `firewall_filter_add` · `firewall_filter_set` · `firewall_filter_remove` · `firewall_nat_list` · `firewall_nat_add` · `firewall_nat_set` · `firewall_nat_remove` · `firewall_rule_move` · `firewall_address_list_list` · `firewall_address_list_add` · `firewall_address_list_remove`

**PPP**

`ppp_active_list` · `ppp_secret_list` · `ppp_secret_add` · `ppp_secret_remove`

**WireGuard**

`wireguard_interface_list` · `wireguard_interface_add` · `wireguard_peer_list` · `wireguard_peer_add` · `wireguard_peer_remove`

**Network utilities**

`tool_ping` · `tool_traceroute`

`jq_filter` is applied after RouterOS replies have been normalized into JSON. Long-running operations (`resource_listen`, `tool_ping`, `tool_traceroute`) run on short-lived cloned RouterOS connections so they do not interfere with the shared session socket.

## Development

```bash
go run . <router-host>   # run the server
go test ./...            # run all tests (mocked, no network needed)
go vet ./...             # static checks
go fmt ./...             # format code
```

Releases are cut automatically: a scheduled workflow updates dependencies every Monday, and when tests pass it bumps the patch version from the latest tag and publishes binaries to GitHub Releases.

## Acknowledgements

This project is a Go port of [parkerkane/mikrotik-manager](https://github.com/parkerkane/mikrotik-manager), originally written in Python (FastMCP). The tool semantics, RouterOS wire protocol handling, and operational behavior are carried over from that project.

## License

MIT
