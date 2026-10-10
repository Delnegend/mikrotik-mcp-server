# Backup

`rosbackup` (this repo, shipped in the release archives) backs up and restores
a full RouterOS configuration from any platform — a single Go binary. SSH host
keys stay pinned; connection flags fall back to the `MIKROTIK_*` environment.

```sh
rosbackup backup  -host 192.168.88.1 -user admin -password <…> \
                  -fingerprint SHA256:<…> -dir ./backups -export
rosbackup restore -host 192.168.88.1 -user admin -password <…> \
                  -fingerprint SHA256:<…> -file ./backups/router-*.backup
```

## Backup

`backup` runs `/system/backup/save` (full binary config, secrets included)
into `backups/<host>-<timestamp>.backup`, waits for the file, downloads it
over SFTP to `-dir` (default `.`), then removes the router copy unless
`-keep-remote`.

| Flag | Default | Description |
|---|---|---|
| `-dir` | `.` | Local output directory |
| `-name` | `backups/<host>-<timestamp>` | Backup base name |
| `-export` | `false` | Also fetch a portable `.rsc` text export (`/export`) |
| `-sensitive` | `false` | Include secrets in the export (`show-sensitive`) |
| `-keep-remote` | `false` | Keep router copies after download |
| `-backup-password` | — | Encrypt the binary backup (RouterOS 7.17+; reuse on restore) |

## Restore

`restore` uploads a `.backup` (applied via `/system/backup/load`) or `.rsc`
(applied via `/import`) under `backups/`, after keeping a timestamped
pre-restore backup beside `-file` unless `-no-preserve`. The API session drops
after a binary restore — reconnect and verify.

| Flag | Default | Description |
|---|---|---|
| `-file` | — | Local `.backup` or `.rsc` file (required) |
| `-no-preserve` | `false` | Skip the automatic pre-restore backup |
| `-backup-password` | — | Password of the binary backup (must match save time) |

Shared connection flags: `-host`, `-api-port` (`8728`), `-api-ssl`,
`-ssh-port` (`22`), `-user` (`admin`), `-password`, `-key`, `-fingerprint`,
`-insecure`, `-timeout` (`30s`). Full disaster-recovery flow: [Backup &
restore guide](BACKUP-RESTORE.md).
