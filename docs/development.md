# Development

Developer documentation for the MikroTik MCP server. For installation and
end-user usage, see the [README](../README.md).

## Prerequisites

- Go 1.27+ (pinned in `go.mod`), `just`
- Docker + KVM for the CHR integration suite; without them, `go test ./...`
  is the full offline suite

## Development with the devcontainer

The repository ships a reproducible dev environment in `.devcontainer/`
(Go pinned to go.mod, `just`, qemu, `--privileged` for host `/dev/kvm`):

```bash
docker build -t mikrotik-mcp-dev .devcontainer
docker run --rm --privileged -v "$PWD":/workspace -w /workspace \
  mikrotik-mcp-dev bash -lc 'bash .devcontainer/postinstall.sh && exec bash'
```

CHR ports (8728/8729/2222/8291/5555) are forwarded to the host. Without
`/dev/kvm` qemu falls back to software emulation — workable, just slower.

## Build & run locally

```bash
go run . <router-host>   # run the server
go build ./...           # build
just check               # go fix + go fmt + go vet
```

## Test strategy

Two layers, deliberately:

- **Pure-logic unit tests** (`go test ./...` with no VM): wire-protocol
  conformance (length-encoding boundary table, reserved control bytes, and
  golden traces transcribed from MikroTik's official API docs), formatting,
  runtime/env logic, and a stdio MCP subprocess smoke test. Fast and offline.
- **Live-Router integration suite** (`internal/integration`, run via
  `just test`): drives the real MCP tool registry against a RouterOS VM —
  client login/print/mutation/listen, tool families, healthcheck, real SFTP
  downloads, host-key verification, opt-in real SSH password rotation,
  **multi-device fleet routing** (`list_devices`, `device=` targeting, unknown
  device errors, no credential leaks), and **safe mode** (enable, mutate via
  the CLI session, rollback reverts, commit persists).

The integration suite is **env-gated**: it skips when `MIKROTIK_TEST_HOST` is
unset, and **fails** when it is set but the router is unreachable — a
misconfigured run is loud, not silently green.

### Testing against a real RouterOS (CHR)

[Cloud Hosted Router (CHR)](https://mikrotik.com/download/chr) is MikroTik's
official virtualized RouterOS, freely downloadable with a Free license. The
`scripts/chr/` workflow boots it in QEMU headless, disposable, with KVM
acceleration and SLIRP NAT (no network config). Run it inside the devcontainer
(the recommended environment; qemu is preinstalled there). Run from the repo
root:

```bash
just up            # boot (idempotent; ~30-60 s with KVM)
just provision     # set admin password, enable api/api-ssl/ssh/winbox, set identity
just test          # full suite against the VM (VM stays up)
just test-clean    # wipe state, test, then stop the VM
just down          # stop (state kept)
just fresh         # wipe state and boot clean
```
Notes:

- **WinBox**: connect by IP to `127.0.0.1` (port `8291`). Neighbor *discovery*
  is Layer-2 broadcast and won't see the VM through SLIRP NAT.
- **SLIRP forwards TCP/UDP only, not ICMP**: `ping`/`traceroute` from the
  router to external hosts time out (`ping 10.0.2.2` works). DNS, SSH, API,
  and TCP outbound all work.
- VSCode forwards the CHR ports automatically when using the devcontainer, so
  `127.0.0.1` on the host reaches the VM.
- The opt-in `MIKROTIK_TEST_PASSWORDLESS=1` runs a real SSH password
  rotation test (uploads a throwaway key, rotates, restores the password).

## CI

- **`ci.yml`** — `just check` plus `go test ./...` on every push and PR.
- **`chr-integration.yml`** — the live-CHR suite, `workflow_dispatch`-only.
- **`auto-merge.yml`** — auto-merges PRs labelled `dependencies`.
- **Dependabot** opens `github-actions` PRs weekly. Go has no native 14-day
  cooldown gate, so there is **no** dependency-upgrade workflow for Go
  modules; upgrade them by hand (`go get -u ./...`).

## Releases

Releases are cut by `.github/workflows/release.yml`, which takes an optional
`ref` input: empty ships the release, a version tag rebuilds one, and a
commit SHA test-builds without publishing ([dispatch modes](https://github.com/Delnegend/actions/blob/main/README.md)).

There is no manifest to bump: the version lives in git tags, so a ship tags
HEAD directly. The workflow also runs on a weekly cron (`0 0 * * 0`, Sunday
00:00 UTC) with an empty `ref`, so unattended maintenance releases happen
every Sunday.
