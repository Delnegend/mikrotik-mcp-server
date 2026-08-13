# Backup & restore guide

This guide covers backing up and restoring the **entire configuration** of a
MikroTik RouterOS device with [`rosbackup`](../cmd/rosbackup/main.go) — the
repo's platform-agnostic CLI. It works on Linux, macOS, and Windows: a single
Go binary, no shell or PowerShell twins.

## Concepts: binary backup vs text export

RouterOS offers two ways to capture a configuration, and they complement each
other:

| | Binary backup (`.backup`) | Text export (`.rsc`) |
|---|---|---|
| Created by | `/system backup save` | `/export` |
| Restored by | `/system backup load` | `/import` |
| Contains | Everything, **including secrets** (passwords, keys) | Config, **secrets hidden** (unless exported with `show-sensitive`) |
| Portable across RouterOS versions | Same major version recommended | Yes (editable, reviewable in a diff) |
| Use case | Exact disaster recovery | Drift review, porting, version bumps |

`rosbackup backup` always takes the binary backup; pass `-export` to also grab
the `.rsc`. Restore a `.backup` for exact recovery, or an edited `.rsc` to
re-apply a reviewed configuration.

## Getting the tool

- **From a release archive** — `rosbackup` is built for
  linux/darwin/windows × amd64/arm64 and shipped with every release.
- **From source** — `go run ./cmd/rosbackup` (needs the Go toolchain).
- Inside this repo's devcontainer there is also `just backup` / `just restore`.

## Backing up

```sh
# Minimal: binary backup to ./backups, using MIKROTIK_* env vars for settings
MIKROTIK_PASSWORD="secret" \
MIKROTIK_SCP_HOST_FINGERPRINT_SHA256="SHA256:AbC…" \
rosbackup backup -host 192.168.88.1 -dir ./backups

# Full: binary + portable export, everything explicit
rosbackup backup \
  -host 192.168.88.1 -user admin -password secret \
  -ssh-port 22 \
  -fingerprint "SHA256:AbC…" \
  -dir ./backups -export

# Keep the copies on the router after download (e.g. for offsite pulls)
rosbackup backup -host 192.168.88.1 -fingerprint "SHA256:…" -keep-remote
```

What happens:

1. Connects to the RouterOS API (`-api-port`, default `8728`; add `-api-ssl`
   with the SSL port for encrypted API).
2. Creates a `backups/` directory on the router and runs
   `/system backup save name=backups/<host>-<timestamp>`.
3. Waits for the file to appear, then downloads it over SFTP with the SSH
   host key pinned to your fingerprint.
4. Deletes the files from the router (unless `-keep-remote`) and prints a
   JSON summary with the local paths.

Output example:

```json
{"export_local":"backups/192-168-88-1-20260813T163626Z.rsc",
 "export_router":"backups/192-168-88-1-20260813T163626Z.rsc",
 "local":"backups/192-168-88-1-20260813T163626Z.backup",
 "router":"backups/192-168-88-1-20260813T163626Z.backup",
 "type":"binary backup"}
```

### Scheduling backups

Any scheduler works since the tool is a plain CLI. Example cron (nightly at
02:30, keeps 30 days of archives):

```cron
30 2 * * *  /usr/local/bin/rosbackup backup -host 192.168.88.1 -dir /srv/router-backups -export \
             >> /var/log/rosbackup.log 2>&1 && find /srv/router-backups -name '*.backup' -mtime +30 -delete
```

## Restoring

> **A binary restore replaces the entire running configuration.** Plan
> accordingly: you will lose the management session, and any uncommitted
> state on the router is gone.

```sh
# Exact restore of a binary backup
rosbackup restore \
  -host 192.168.88.1 -user admin -password secret \
  -fingerprint "SHA256:AbC…" \
  -file ./backups/192-168-88-1-20260813T163626Z.backup

# Re-apply an edited text export (e.g. after a version bump)
rosbackup restore -host 192.168.88.1 -fingerprint "SHA256:…" \
  -file ./backups/192-168-88-1-20260813T163626Z.rsc
```

What happens:

1. **Safety net** — a timestamped `pre-restore-*.backup` of the *current*
   config is taken and downloaded next to your restore file first (skip with
   `-no-preserve`). If the restore file is bad, you still have a way back.
2. The file is uploaded to the router over SFTP.
3. `.backup` files are applied with `/system backup load`; `.rsc` files with
   `/import`.
4. The API session usually **drops after a binary restore** — this is normal.
   The tool detects it and reports success; verify afterwards (see below).

### Verifying a restore

The router needs a few seconds to come back after a binary load. Then:

```sh
rosbackup backup -host 192.168.88.1 -fingerprint "SHA256:…" -dir ./verify
# or
go run ./cmd/chrprovision   # only for the test CHR: fails when password is already set
```

A successful `rosbackup backup` is the strongest check: it needs API + SSH +
credentials, exactly what a restored router must have.

## Encrypted backups (RouterOS 7.17+)

Newer RouterOS versions support encrypting binary backups. Use the same
`-backup-password` on save **and** load — it is **not** recoverable:

```sh
rosbackup backup  -backup-password "long-random" -export ...
rosbackup restore -backup-password "long-random" -file backups/...backup
```

Keep the password somewhere safe (password manager, secret store) or the
backup is unrecoverable.

## Host key verification

SSH/SFTP transfers are fail-closed: without a fingerprint the tool refuses to
run.

- Get the fingerprint once: connect with your SSH client, or use
  `MIKROTIK_SCP_INSECURE=1` + the printed fingerprint on the *first* run and
  pin it afterwards.
- Prefer `-fingerprint`; set it permanently via
  `MIKROTIK_SCP_HOST_FINGERPRINT_SHA256` in your environment.
- `-insecure` skips verification entirely — only for throwaway labs, never
  for production routers.

All connection flags fall back to the standard `MIKROTIK_*` variables:
`MIKROTIK_PASSWORD`, `MIKROTIK_SCP_HOST`, `MIKROTIK_SCP_PORT`,
`MIKROTIK_SCP_USER`, `MIKROTIK_SCP_PRIVATE_KEY`,
`MIKROTIK_SCP_HOST_FINGERPRINT_SHA256`.

## Full worked example

```sh
# 1. First contact: fetch the fingerprint (throws away nothing)
ssh-keyscan -p 2222 192.168.88.1 | ssh-keygen -lf -          # SSH on 2222

# 2. Nightly backup with both artifacts
rosbackup backup -host 192.168.88.1 -password secret \
  -fingerprint "SHA256:AbC…" -dir ./backups -export -backup-password vault-pass

# 3. Disaster: router replaced / factory reset -> restore the last good state
rosbackup restore -host 192.168.88.1 -password secret \
  -fingerprint "SHA256:AbC…" -backup-password vault-pass \
  -file backups/192-168-88-1-20260813T163626Z.backup

# 4. Verify
rosbackup backup -host 192.168.88.1 -password secret \
  -fingerprint "SHA256:AbC…" -dir /tmp/verify
```

## Using the MCP server instead

The MCP server exposes the same workflow as tools, for AI-assisted use:

- `system_backup_save` — create a binary backup on the router.
- `system_export` — create a text export.
- `system_backup_collect` — create backup + export and download both into the
  workspace (the tool-level twin of `rosbackup backup`).

`rosbackup` is the right choice for scheduled, scripted, or manual
administrative use; the MCP tools are for interactive sessions.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `SSH host key verification is disabled` | No fingerprint set | Pass `-fingerprint`, or set `MIKROTIK_SCP_HOST_FINGERPRINT_SHA256` |
| `API connect ... connection refused` | Wrong API port / API disabled / router down | `-api-port 8728` (or `-api-ssl` with `-api-port 8729`); enable the `api` service |
| `API connect ... i/o timeout` | Router still booting or reloading after a restore | Wait ~20-60 s and retry (`rosbackup` retries nothing; wrap in a small retry loop if needed) |
| `backup/load: missing =password=` | RouterOS 7.17+ requires the password parameter | Re-run with `-backup-password` (empty password is sent automatically) |
| `backup/load` failed but the router is still reachable | Real error (bad file, wrong password, version mismatch) | The tool reports it; the pre-restore backup is still on disk |
| Session drops after restore | Normal — binary load replaces the config | Reconnect and verify (see "Verifying a restore") |
| Export misses secrets | `hide-sensitive` default | Add `-sensitive` to `backup` |
| Restore on a different RouterOS version | Binary format changed | Restore the `.rsc` export instead, or re-run `-export` backups |

## Further reading

- [Development docs](DEVELOPMENT.md) — test setup, the CHR test router, CI.
- [README](../README.md) — general project overview and MCP tools.
