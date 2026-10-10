# Configuration

Configuration comes from environment variables, with a `.env` file in the
working directory as a fallback (existing process env wins). Unrecognized
values for the TLS switches are rejected at startup — a typo cannot silently
downgrade you to plaintext.

## Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `MIKROTIK_USER` | — | Yes | RouterOS API username |
| `MIKROTIK_PASSWORD` | — | Yes¹ | RouterOS API password |
| `MIKROTIK_API_SSL`, `MIKROTIK_TLS_VERIFY` | `true`, `true` | No | TLS for the API; verify the router certificate |
| `MIKROTIK_API_PORT` | `8729` SSL / `8728` plain | No | RouterOS API port |
| `MIKROTIK_API_TIMEOUT` | `10.0` | No | API timeout in seconds |
| `MIKROTIK_API_PASSWORDLESS_ENABLED`, `MIKROTIK_API_PASSWORDLESS_LENGTH` | `false`, `32` | No | Rotate the API password over SSH at startup; generated length |
| `MIKROTIK_SCP_HOST`, `MIKROTIK_SCP_PORT` | API host, `22` | No | SSH endpoint for file download and backups |
| `MIKROTIK_SCP_USER`, `MIKROTIK_SCP_PASSWORD`, `MIKROTIK_SCP_PRIVATE_KEY`, `MIKROTIK_SCP_KEY_PASSPHRASE` | `MIKROTIK_USER`, `MIKROTIK_PASSWORD`, —, — | No | SSH credentials (key replaces password) |
| `MIKROTIK_SCP_HOST_FINGERPRINT_SHA256`, `MIKROTIK_SCP_INSECURE` | —, `0` | No | Pinned `SHA256:…` host key; `1` skips verification (MITM risk) |
| `MIKROTIK_SCP_TIMEOUT` | `30.0` | No | SSH timeout in seconds |

¹ Not required when `MIKROTIK_API_PASSWORDLESS_ENABLED=true`.

## Fleet inventory

Without an inventory the server manages the single device from the flat
variables above. Set `MIKROTIK_INVENTORY` (inline JSON, wins) or
`MIKROTIK_INVENTORY_FILE` (path) to a device array:

```json
[
  {"title": "RouterA", "host": "192.168.88.1", "password": "<…>"},
  {"title": "RouterB", "host": "10.0.0.2", "username": "ops", "password": "<…>"}
]
```

Per device: `title` (required, unique), `host` (required), `port` (`8728`),
`username` (`admin`), `password`, `api_ssl` (`true`), `tls_verify` (`true`),
`timeout` (`10`), `ssh_port` (`22`), `ssh_fingerprint`, `tags`, `region`.

Every device-scoped tool takes a `device` title (case-insensitive; omittable
with one device, required with several). `list_devices` returns everything
except credentials, which are never exposed. Drop `.pem`/`.crt`/`.cer` files
into `certs/` to trust private CAs (`.disabled` ignored).
