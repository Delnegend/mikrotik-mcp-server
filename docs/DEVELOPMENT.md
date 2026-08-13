# Development

Developer documentation for the MikroTik MCP server. For installation and
end-user usage, see the [README](../README.md).

## Layout

```
.devcontainer/           reproducible dev environment (Go, just, qemu; KVM passthrough)
cmd/chrprovision/        optional CHR hardening tool (set password, enable services)
internal/client/         RouterOS binary-protocol client (wire encoding, listen/cancel, TLS)
internal/downloads/      SSH/SFTP for file download + passwordless password rotation
internal/formatting/     markdown result rendering
internal/helpers/        shared helpers (paths, env, JSON)
internal/inventory/      multi-device fleet registry (JSON inventory, validation)
internal/runtime/        startup/env configuration
internal/safemode/       RouterOS safe-mode console sessions (Ctrl+X, commit/rollback)
internal/server/         MCP tool handlers
internal/integration/    env-gated tests against a live RouterOS VM
scripts/chr/             CHR (Cloud Hosted Router) QEMU workflow
```

## Development with the devcontainer

The repository ships a reproducible dev environment in `.devcontainer/`
(Dockerfile + devcontainer.json + postinstall.sh). The image preinstalls the
Go toolchain (pinned to go.mod), `just`, qemu-system-x86/qemu-utils, unzip,
netcat, and creates a `delnegend` user (uid 1000) with passwordless sudo. The
container runs `--privileged` so the host's `/dev/kvm` is passed through; on
first create, `postinstall.sh` makes it world-accessible (host group ids
differ) and runs `just check` to verify the toolchain.

Open the repo in VSCode and "Reopen in Container", or run the image directly:

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

Equivalent one-liners (what the `just` recipes call):

```bash
bash scripts/chr/up.sh
go run ./cmd/chrprovision -password admin
MIKROTIK_TEST_HOST=127.0.0.1 MIKROTIK_TEST_USER=admin MIKROTIK_TEST_PASSWORD=admin go test ./internal/integration/ -v
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

## Official references

The client is implemented and tested against MikroTik's official artifacts:

- **API protocol** — [RouterOS API](https://manual.mikrotik.com/docs/developer-guides/api/)
  documents framing, word-length encoding, sentences, queries, `.tag`, and
  `/login` + `/cancel` semantics, with exact example reply traces.
- **Reference client** — [Python3 example](https://manual.mikrotik.com/docs/developer-guides/api/python3-example)
  is MikroTik's own wire-protocol implementation. Our encoder matches it
  byte-for-byte for every length class (`TestEncodeSentenceMatchesOfficialReference`).
- **REST API** (RouterOS 7) — a cross-reference for command paths and
  attribute names; the server speaks the native binary protocol.

There is **no official Go SDK or conformance suite**. `github.com/mikrotik`
is **not** an official MikroTik org (unrelated third-party repos).

## Why not `go-routeros/routeros/v3`?

[go-routeros/routeros/v3](https://github.com/go-routeros/routeros) (MIT,
~270★) is the de-facto standard Go RouterOS library, and `internal/client`'s
low-level protocol core overlaps its `proto` package. We keep our own client
because it layers semantics the library doesn't ship: bounded auto-cancel
`listen`, `RunContext`/`ListenContext` that issue `/cancel` on request
cancellation, `Clone`/`Isolated` per-command connections,
certificate-fingerprint healthcheck data, and the RouterOS error taxonomy.
The protocol layer is verified byte-for-byte against the official spec, and
the library has had no release since Feb 2025. If that maintenance risk ever
outweighs it, `internal/client` is the single seam to swap out.

## Releases

Releases are cut automatically: a scheduled workflow updates dependencies
every Monday (`go get -u=patch ./...`) and opens a PR for review; a separate
workflow builds the platform archives and publishes them to GitHub Releases
when triggered manually.
