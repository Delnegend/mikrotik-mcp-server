# Deployment & Operations

## Install

### Homebrew (Recommended)

```bash
brew install Delnegend/tap/mikrotik-mcp
```

### Prebuilt Binary

Download the latest release archive from [GitHub Releases](https://github.com/Delnegend/mikrotik-mcp-server/releases):

```bash
URL=$(curl -fsSL https://api.github.com/repos/Delnegend/mikrotik-mcp-server/releases/latest | grep -o 'https://[^"]*linux-amd64\.tar\.xz')
curl -fsSL -o mikrotik-mcp.tar.xz "$URL" && tar -xJf mikrotik-mcp.tar.xz --strip-components=1
install -m 755 mikrotik-mcp ~/.local/bin/mikrotik-mcp
```

## MCP Client Configuration

The server communicates over standard I/O (`stdio`). Register the binary with your MCP client.

### Claude Code (`.mcp.json`)

```json
{
  "mcpServers": {
    "mikrotik": {
      "command": "mikrotik-mcp",
      "args": ["192.168.88.1"],
      "env": {
        "MIKROTIK_USER": "admin",
        "MIKROTIK_PASSWORD": "<password>"
      }
    }
  }
}
```

### VS Code (`.vscode/mcp.json`)

```json
{
  "servers": {
    "mikrotik": {
      "command": "mikrotik-mcp",
      "args": ["192.168.88.1"],
      "env": {
        "MIKROTIK_USER": "admin",
        "MIKROTIK_PASSWORD": "<password>"
      }
    }
  }
}
```

### OpenCode (`opencode.json`)

```json
{
  "mcp": {
    "mikrotik": {
      "type": "local",
      "command": ["mikrotik-mcp", "192.168.88.1"],
      "environment": {
        "MIKROTIK_USER": "admin",
        "MIKROTIK_PASSWORD": "<password>"
      }
    }
  }
}
```

For passwordless SSH key rotation and full inventory configuration, see [Configuration](configuration.md).

## Upgrade

```bash
brew upgrade mikrotik-mcp
```
