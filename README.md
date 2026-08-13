# MikroTik MCP Server

A **Model Context Protocol (MCP) server** that lets AI assistants manage MikroTik routers over the native RouterOS API. Ships as a single static binary and works with any MCP client — opencode, Claude Code, VS Code, Zed, and more.

A Go port of [parkerkane/mikrotik-manager](https://github.com/parkerkane/mikrotik-manager).

## Features

- **Full router management through MCP tools** — interfaces, IP addressing, routing, DHCP, DNS, bridges, VLANs, firewall, PPP, WireGuard, and more
- **Multi-device fleet** — manage any number of routers from one server via a JSON inventory, targeting each with a `device` argument (`list_devices` discovers them)
- **RouterOS Safe Mode** — enable/commit/rollback: mutations are held in memory until you commit, and revert automatically on rollback or disconnect
- **Generic low-level access** (`resource_print` / `add` / `set` / `remove`, `command_run`) for anything without a dedicated tool
- **Files & backups** — router file listing, download over SSH/SFTP, config export, and one-shot backup collection
- **Passwordless API startup** — rotate the API password on each start via an SSH key
- **Healthcheck** — verifies API, SSH/SFTP, and passwordless readiness in one call
- **Optional `jq_filter`** on normalized `resource_print` results

## Requirements

- A MikroTik router running **RouterOS 7+** with the API service enabled:
  `/ip/service set api disabled=no`
- RouterOS credentials for a user with API permissions
- A computer with an MCP client (or Go 1.25+ to build from source)

## Installation

```bash
# Option 1 — download the matching archive from Releases
# (linux/darwin/windows × amd64/arm64, as .tar.xz or .7z)

# Option 2 — install with Go
go install github.com/Delnegend/mikrotik-mcp@latest

# Option 3 — install with Homebrew
brew tap Delnegend/tap
brew install mikrotik-mcp

# Option 4 — curl | tar, straight into /usr/local/bin (Linux amd64)
curl -fsSL https://github.com/Delnegend/mikrotik-mcp-server/releases/latest/download/mikrotik-mcp-linux-amd64.tar.xz | sudo tar -xJf - -C /usr/local/bin
sudo mv /usr/local/bin/mikrotik-mcp-linux-amd64 /usr/local/bin/mikrotik-mcp
```

Replace `linux-amd64` with `linux-arm64` or `darwin-amd64` for other platforms. The archive contains a single binary named after its platform, hence the rename so the MCP configs below can call it as `mikrotik-mcp`.

Verify it starts:

```bash
mikrotik-mcp <router-host>   # hostname or IP of the router
mikrotik-mcp -version        # prints the embedded build version (e.g. mikrotik-mcp 0.1.0)
```

## MCP Client Configuration

The `<router-host>` argument is required. Credentials are provided as environment variables (see below).

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
| `MIKROTIK_TLS_VERIFY` | no | `true` | Verify the TLS certificate |
| `MIKROTIK_API_PORT` | no | `8729` (SSL) / `8728` (plain) | RouterOS API port |
| `MIKROTIK_API_TIMEOUT` | no | `10.0` | RouterOS API timeout in seconds |
| `MIKROTIK_API_PASSWORDLESS_ENABLED` | no | `false` | Rotate API password on each startup via SSH |
| `MIKROTIK_API_PASSWORDLESS_LENGTH` | no | `32` | Generated API password length |
| `MIKROTIK_SCP_HOST` | no | *API host* | SSH server for file download / backups |
| `MIKROTIK_SCP_PORT` | no | `22` | SSH port |
| `MIKROTIK_SCP_USER` | no | `MIKROTIK_USER` | SSH username |
| `MIKROTIK_SCP_PASSWORD` | no | `MIKROTIK_PASSWORD` | SSH password |
| `MIKROTIK_SCP_PRIVATE_KEY` | no | — | Path to an SSH private key (replaces password) |
| `MIKROTIK_SCP_KEY_PASSPHRASE` | no | — | Passphrase for the private key |
| `MIKROTIK_SCP_HOST_FINGERPRINT_SHA256` | no | — | Expected SSH host key fingerprint (`SHA256:...`), required for passwordless mode |
| `MIKROTIK_SCP_INSECURE` | no | `0` | Set to `1` to skip SSH host key verification (MITM risk) |
| `MIKROTIK_SCP_TIMEOUT` | no | `30.0` | SSH timeout in seconds |
| `MIKROTIK_INVENTORY` | no | — | Inline JSON fleet inventory (array of devices; wins over the file) |
| `MIKROTIK_INVENTORY_FILE` | no | — | Path to a JSON fleet inventory file |

¹ Not required when `MIKROTIK_API_PASSWORDLESS_ENABLED=true` (SSH key auth replaces it).

### Fleet inventory

Without an inventory, the server manages the single device from the flat
environment above. To manage a fleet, set `MIKROTIK_INVENTORY` (inline) or
`MIKROTIK_INVENTORY_FILE` (path) to a JSON array:

```json
[
  {"title": "RouterA", "host": "192.168.88.1", "password": "pw-a", "tags": ["lab"], "region": "NL"},
  {"title": "RouterB", "host": "10.0.0.2", "username": "ops", "password": "pw-b", "api_ssl": false}
]
```

Per device: `title` (required, unique), `host` (required), `port` (default
`8728`), `username` (default `admin`), `password`, `api_ssl` (default `true`),
`tls_verify` (default `true`), `timeout` (seconds, default `10`), `ssh_port`
(default `22`), `ssh_fingerprint`, `tags`, `region`.

- Every device-scoped tool accepts a `device` argument with the device's
  `title` (case-insensitive). With one device it can be omitted; with several
  it is required, and an unknown/missing title errors listing the fleet so the
  caller can self-correct.
- `list_devices` returns title, host, port, username, tags, region —
  **credentials are never returned**.
- An invalid inventory (bad JSON, missing/duplicate title) stops the server at
  startup with one clear message.

## Security & guarantees

- **TLS by default.** The API connection uses TLS and the router certificate is verified; set `MIKROTIK_TLS_VERIFY=false` to disable (insecure). Unrecognized values for the TLS switches are **rejected at startup** — a typo cannot silently downgrade you to plaintext.
- **Private CAs.** Drop `.pem`/`.crt`/`.cer` files into a `certs/` directory next to the working directory to trust them (files ending in `.disabled` are ignored).
- **SSH host keys fail closed.** Without `MIKROTIK_SCP_HOST_FINGERPRINT_SHA256` (or an explicit `MIKROTIK_SCP_INSECURE=1`), every SSH/SFTP connection is refused. No silent MITM.
- **Downloads are contained.** `file_download` and `system_backup_collect` write only inside the workspace root; absolute paths outside it are rejected.
- **Cancellable long-running commands.** `resource_listen`, `tool_ping`, and `tool_traceroute` run on short-lived cloned connections and are interrupted (`/cancel`) when the MCP request is cancelled, so nothing is left running on the router.
- **Safe mode.** `enable_safe_mode` opens a persistent console session; mutating tools are routed through it until `commit_safe_mode` (persist) or `rollback_safe_mode` (revert everything, automatically on disconnect too).

## Tool Surface

The server exposes the following MCP tools, grouped by area:

| Group | Tools |
|---|---|
| **Generic RouterOS primitives** | `resource_print`, `resource_add`, `resource_set`, `resource_remove`, `command_run`, `resource_listen`, `command_cancel` |
| **Fleet & safe mode** | `list_devices`, `safe_mode_status`, `enable_safe_mode`, `commit_safe_mode`, `rollback_safe_mode` |
| **System & health** | `healthcheck`, `system_resource_get`, `system_identity_get`, `system_clock_get` |
| **Interfaces, addressing & routing** | `interface_list`, `interface_get`, `interface_monitor`, `ip_address_list`, `ip_address_get`, `ip_route_list`, `ip_route_get` |
| **DHCP & DNS** | `dhcp_lease_list`, `dhcp_server_list`, `dhcp_network_list`, `dns_get`, `dns_set`, `dns_resolve` |
| **Files & backups** | `file_list`, `file_download`, `system_backup_save`, `system_export`, `system_backup_collect` |
| **Bridges & VLANs** | `bridge_list`, `bridge_add`, `bridge_remove`, `bridge_port_list`, `bridge_port_add`, `bridge_port_remove`, `bridge_vlan_list`, `bridge_vlan_add`, `bridge_vlan_remove`, `vlan_list`, `vlan_add`, `vlan_remove` |
| **Firewall** | `firewall_filter_list`, `firewall_filter_add`, `firewall_filter_set`, `firewall_filter_remove`, `firewall_nat_list`, `firewall_nat_add`, `firewall_nat_set`, `firewall_nat_remove`, `firewall_rule_move`, `firewall_address_list_list`, `firewall_address_list_add`, `firewall_address_list_remove` |
| **PPP** | `ppp_active_list`, `ppp_secret_list`, `ppp_secret_add`, `ppp_secret_remove` |
| **WireGuard** | `wireguard_interface_list`, `wireguard_interface_add`, `wireguard_peer_list`, `wireguard_peer_add`, `wireguard_peer_remove` |
| **Network utilities** | `tool_ping`, `tool_traceroute` |

`jq_filter` is applied after RouterOS replies are normalized into JSON.

## Backup & restore CLI

`rosbackup` (part of this repo, built into the release archives) backs up and
restores a full RouterOS configuration from any platform — a single Go binary,
no shell or PowerShell twins:

```sh
rosbackup backup  -host 192.168.88.1 -user admin -password PW \
                  -fingerprint SHA256:... -dir ./backups -export
rosbackup restore -host 192.168.88.1 -user admin -password PW \
                  -fingerprint SHA256:... -file ./backups/router-*.backup
```

- `backup` saves the full binary config (`/system/backup/save`, secrets
  included) and downloads it over fingerprint-pinned SFTP; `-export` also
  fetches a portable text `.rsc` export (`-sensitive` includes secrets).
  Router files are removed after download unless `-keep-remote`.
- `restore` uploads a `.backup` (loaded via `/system/backup/load`) or a `.rsc`
  (imported), and always keeps a timestamped pre-restore backup locally first
  unless `-no-preserve`. The API session drops after a binary restore — that
  is expected; reconnect and verify.
- RouterOS 7.17+ supports encrypted backups: pass the same `-backup-password`
  to `backup` and `restore`.
- Flags fall back to the `MIKROTIK_*` environment variables (see above).
  Prefer `-fingerprint`; `-insecure` skips host key verification (MITM risk).

See [docs/BACKUP-RESTORE.md](docs/BACKUP-RESTORE.md) for the full guide:
scheduling backups, restoring after a disaster, encrypted backups, host key
verification, and troubleshooting.

## Acknowledgements

This project is a Go port of [parkerkane/mikrotik-manager](https://github.com/parkerkane/mikrotik-manager), originally written in Python (FastMCP); the tool semantics and operational behavior are carried over.

## License

MIT
