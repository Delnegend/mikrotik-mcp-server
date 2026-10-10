# Client setup

The server speaks MCP over stdio (`server.ServeStdio`), so every client below
launches the same binary with the router host as its positional argument and
passes credentials as environment variables. All four rows validate: stdio
transport, `command` = `mikrotik-mcp`, `env_keys` = `MIKROTIK_USER` plus
`MIKROTIK_PASSWORD`.

| Client | `command` | `env_keys` | `transport` |
|---|---|---|---|
| `opencode` (`opencode.json`) | `["mikrotik-mcp", "<…>"]` | `MIKROTIK_USER`, `MIKROTIK_PASSWORD` | `local` (stdio) |
| `Claude Code` (`.mcp.json`) | `"mikrotik-mcp"`, `args: ["<…>"]` | `MIKROTIK_USER`, `MIKROTIK_PASSWORD` | `stdio` |
| `VS Code` (`.vscode/mcp.json`) | `"mikrotik-mcp"`, `args: ["<…>"]` | `MIKROTIK_USER`, `MIKROTIK_PASSWORD` | `stdio` |
| `Zed` (`settings.json`) | `"mikrotik-mcp"`, `args: ["<…>"]` | `MIKROTIK_USER`, `MIKROTIK_PASSWORD` | `stdio` |

`<…>` is the router hostname or IP (e.g. `192.168.88.1`).

```json
{
  "mcp": {
    "mikrotik": {
      "type": "local",
      "command": ["mikrotik-mcp", "192.168.88.1"],
      "enabled": true,
      "environment": {
        "MIKROTIK_USER": "admin",
        "MIKROTIK_PASSWORD": "<…>"
      }
    }
  }
}
```

The other three clients use the same shape with `command` / `args` / `env`
keys per their own schema; only the key names differ, never the values.

When `MIKROTIK_API_PASSWORDLESS_ENABLED=true`, `MIKROTIK_PASSWORD` is not
needed — the server rotates the API password over SSH at startup. See
[Configuration](configuration.md) for the full variable inventory.
