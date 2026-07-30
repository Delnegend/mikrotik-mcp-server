# MikroTik MCP Server — Test Suite Remediation Plan

> Generated from a static comparative audit of the Python (original) and Go (port) test suites.
>
> Python source: `tools/mikrotik/mikrotik_mcp/` + `tools/mikrotik/tests/`
> Go source: `tools/mikrotik/internal/` + `tools/mikrotik/main.go`
>
> **Coverage snapshot:** Python has 176 test functions across 4 files. Go has 96 across 7 files. Approximately 110 Python tests have no Go counterpart. Of the ~30 where both exist, roughly half use weaker assertions in Go (substring `contains()` vs exact sentence verification, no structured-content checks).
>
> **Progress:** Phase 0 ✓, Phase 1 ✓ — ~6 / ~250 tasks complete (verified by re-review 2026-07-30)

---

## Phase 0: Audit Review & Decisions *(~0.5 day — discussion-driven)*

Before any code changes, decide which divergences are intentional simplifications vs bugs.

- [x] **D1 — Structured content:** Python sets `structuredContent` on every tool result; Go never does.
  - Decision: **(a) Implement in Go** — Phase 2.1.

- [x] **D2 — Panics in `normalize*` functions:** Python raises `ValueError`; Go panics.
  - Decision: **(a) Wrap every handler in `recover()`** — Phase 1.4.

- [x] **D3 — `ResolveSCPPrivateKeyPath`:** Python errors on configured-but-missing key; Go returns `""` (silent fallback to password).
  - Decision: **(a) Port Python behaviour (error)** — Phase 1.6.

- [x] **D4 — `shellQuote` always-quotes:** Go wraps every string in `'…'`; Python `shlex.quote` is conditional.
  - Decision: **(b) Accept always-quoting** — document the diff.

- [x] **D5 — `TLSSessionInfo` field formats:** Go dates RFC3339 vs Python raw, serial decimal vs hex, subject `CN=` vs `commonName=`.
  - Decision: **(b) Accept Go-native formatting** — document for clients.

- [x] **D6 — Tool input schemas:** ~40 tools with zero properties; clients can't discover parameters.
  - Decision: **(a) Declare full schemas matching Python** — Phase 1.1.

- [x] **D7 — Healthcheck formatting:** Python has curated flat table + "Likely issue:" diagnosis; Go renders raw Go map dumps.
  - Decision: **(a) Port the Python formatter** — Phase 2.5.

- [x] **D8 — `resource_print` JSON output:** Go uses compact JSON; Python uses pretty-printed (indent 2).
  - Decision: **(b) Accept compact** — clients that parse JSON don't care about formatting.

- [x] **D9 — Exact vs substring wire-sentence assertions:** Go checks `contains()` for pieces; Python checks full byte equality.
  - Decision: **(a) Force exact assertions after making attr order deterministic** — Phase 4.6.

---

## Phase 1: Critical Production Fixes *(~3–5 days — code changes, no new tests yet)*

These fix production behaviour the current Go suite would never catch.

### 1.1 — Declare Tool Input Schemas for All Tools

**Files:** `internal/server/tool_layer2.go` (12–29), `internal/server/tool_security.go` (13–30), `internal/server/tool_access.go` (12–25), `internal/server/tool_files.go` (line 55), plus the `reg`-registered tools in `internal/server/tool_core.go` (46–84).

Change every `reg(name, desc, handler)` to `s.AddTool(mcp.NewTool(name, mcp.WithDescription(desc), <params>), handler)` matching Python's `app.py`.

- [x] `resource_add`, `resource_set`, `resource_remove`: `menu` (required), `attributes` for add, `item_id` for set/remove
- [x] `resource_print`: `menu` (required), `proplist`, `queries`, `attributes`, `jq_filter`
- [x] `command_run`: `command` (required), `attributes`, `queries`
- [x] `command_cancel`: `tag` (required)
- [x] `resource_listen`: `menu` (required), `proplist`, `queries`, `attributes`, `tag`, `max_events`
- [x] `dns_resolve`: `name` (required), `server`
- [x] `interface_monitor`: `name` (required)
- [x] `system_*` tools: `system_resource_get`, `system_identity_get`, `system_clock_get`, `healthcheck`, `dhcp_server_list`, `dhcp_network_list`, `dns_get`, `dns_set` — match Python schemas per-tool
- [x] All `*_list` tools: add filter params per tool
- [x] All `*_add` tools: `attributes` (required object)
- [x] All `*_remove` tools: `item_id` (required)
- [x] `firewall_filter_set` / `firewall_nat_set`: `item_id` (required), `attributes` (required)
- [x] `firewall_rule_move`: `table` (required), `item_id` (required), `destination` (required)
- [x] `ppp_secret_add`: `attributes` (required, handler validates `name`+`password` internally)
- [x] `wireguard_interface_add`: `attributes` (required, handler validates `name` internally)
- [x] `wireguard_peer_add`: `attributes` (required, handler validates `interface`+`public-key` internally)
- [x] `file_list`: `directory`, `name`, `file_type`
- [x] `system_backup_save`: `name` (required)
- [x] `system_export`: `name` (required), `include_sensitive`, `compact`
- [x] `file_download`: `router_path` (required), `local_path`
- [x] `system_backup_collect`: `name_prefix`, `include_sensitive`, `compact`, `local_dir`

> **Phase 1.1 review retro (done):**
>
> - [x] **1.1-retro-a — `resource_print`/`resource_listen` `attributes` object not extracted:** Fixed — extract `argObject(req, "attributes")` first, then fall back to stripping control keys.
>
> - [x] **1.1-retro-b — `dns_set` sends `servers` as a slice:** Fixed — strip whitespace, reject all-empty servers, and join with `,`.
>
> - [x] **1.1-retro-c — Required `item_id`/`tag` strings not validated:** Fixed — validate with `helpers.NormalizeRequiredString(...)` in all remove/set/cancel handlers.
 
> **Why here:** Without schemas, production clients cannot discover or properly call ~40 of 60 tools. Empty-schema handlers receive no args → `RequireAttributes` returns error → tool always fails. This is a runtime correctness issue, not just a coverage gap.


---

### 1.2 — Fix Attrs Pollution in Core Handlers

**Files:** `internal/server/tool_core.go` lines 191, 222, 235, 260, 273; `internal/server/tool_layer2.go` line 43; `internal/server/tool_security.go` line 35; `internal/server/tool_access.go` lines 30, 44, 58.

The core handlers pass the *entire* `req.Params.Arguments` map as RouterOS attributes. Control keys (`menu`, `item_id`, `jq_filter`, `command`, `queries`, `tag`, `max_events`) end up on the wire as `=menu=…`, `=item_id=…` etc. Real routers would trap on unknown parameters.

- [x] Add `argObject(req, key)` helper in `tool_core.go`:
  ```go
  func argObject(req mcp.CallToolRequest, key string) map[string]any {
      v, ok := req.Params.Arguments[key]
      if !ok { return nil }
      m, ok := v.(map[string]any)
      if !ok { return nil }
      return m
  }
  ```
- [x] Fix `handlerResourceAdd` and `handlerResourceSet` — extract `attributes` sub-map from args, not whole args
- [x] Fix `handlerResourcePrint` — strip `menu`, `proplist`, `queries`, `jq_filter` before passing attrs (or add explicit `attributes` object param)
- [x] Fix `handlerCommandRun` — strip `command`, `queries` from attrs
- [x] Fix `handlerResourceListen` — strip `menu`, `proplist`, `queries`, `tag`, `max_events` from attrs
- [x] Fix generic `addHandler`/`setHandler` (tool_layer2, tool_security, tool_access) — extract only `attributes` sub-map after Phase 1.1 declares it

> **Phase 1.2 review retro (done):**
> >
> > - [x] **1.2-retro — `resource_print` and `resource_listen` still pollute RouterOS attributes:** Fixed — extract `argObject(req, "attributes")` first, then fall back to stripping control keys.
 
---
 
### 1.3 — Implement Custom CA Certificate Loading


**File:** `internal/client/client.go` line 321.

- [x] Replace `// TODO: load CA certs from x509.CertPool` with production code:
  ```go
  if c.tlsVerify && len(c.tlsCAFiles) > 0 {
      certPool := x509.NewCertPool()
      for _, f := range c.tlsCAFiles {
          pem, err := os.ReadFile(f)
          if err != nil {
              rawConn.Close()
              return fmt.Errorf("%w: failed to read CA cert %s: %v", ErrRouterOSTransportError, f, err)
          }
          if !certPool.AppendCertsFromPEM(pem) {
              rawConn.Close()
              return fmt.Errorf("%w: no valid CA cert found in %s", ErrRouterOSTransportError, f)
          }
      }
      tlsConfig.RootCAs = certPool
  }
  ```
  Mirrors Python `client.py:193–195` (`context.load_verify_locations(cafile=...)`).

---

### 1.4 — Add Panic Recovery to Handlers

**Files:** `internal/server/tool_core.go`, `tool_files.go`, `tool_layer2.go`, `tool_security.go`, `tool_access.go`.

Client functions `normalizeMenu`/`normalizeItemID`/`normalizeCommandPath`/`normalizeTag`/`normalizeQueries` (client.go:706–758) and `loadPrivateKey` (downloads.go:149–163) **panic** on bad input.

- [x] Add a `recoverHandler` wrapper:
  ```go
  func recoverHandler(h server.ToolHandlerFunc) server.ToolHandlerFunc {
      return func(ctx context.Context, req mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
          defer func() {
              if r := recover(); r != nil {
                  err = fmt.Errorf("internal error: %v", r)
              }
          }()
          return h(ctx, req)
      }
  }
  ```
- [x] Wrap every handler registration through `recoverHandler` (if D2 = (b), use as safety net during longer refactor)

---

### 1.5 — Fix Ping `packet_size` Validation

**File:** `internal/server/tool_core.go` line 323.

- [x] Replace dead condition `if packetSize > 0 && packetSize < 1` with:
  ```go
  if _, ok := req.Params.Arguments["packet_size"]; ok && packetSize < 1 {
      return nil, fmt.Errorf("packet_size must be at least 1")
  }
  ```
  Matches the traceroute pattern at line 391. `packet_size=0` explicitly provided now errors; omitted defaults to 0 (silently skipped).

---

### 1.6 — Fix `ResolveSCPPrivateKeyPath` to Error on Missing Configured Key

**File:** `internal/downloads/downloads.go` lines 412–425.

- [x] Change signature to return `(string, error)` — error if `MIKROTIK_SCP_PRIVATE_KEY` is set but file doesn't exist:
  ```go
  func ResolveSCPPrivateKeyPath() (string, error) {
      configured := os.Getenv("MIKROTIK_SCP_PRIVATE_KEY")
      if configured == "" { return "", nil }
      path := configured
      if !filepath.IsAbs(path) {
          path = filepath.Join(helpers.WorkspaceRoot(), path)
      }
      if _, err := os.Stat(path); os.IsNotExist(err) {
          return "", fmt.Errorf("SCP private key file '%s' does not exist", path)
      }
      return path, nil
  }
  ```
- [x] Update callers (`LoadFileTransferSettings`, `LoadPasswordRotationSettings`)
- [x] Update package-level vars `hcResolveSCPPrivateKeyPath` (tool_core.go) and `ftResolveSCPPrivateKeyPath` (tool_files.go)

> **Phase 1.6 review retro (done):**
>
> - [x] **1.6-retro — `ResolveSCPPrivateKeyPath` file check is incomplete:** Fixed — reject all stat errors, require regular file.
 
---
 
## Phase 2: Core Test Suite Porting *(~8–12 days — test-only changes)*


### 2.0 — Test Infrastructure Improvements *(pre-requisite)*

- [ ] **2.0a** — Replace every hand-rolled `contains()` with `strings.Contains`, including the copy in production code (`tool_core.go:839-846`). Files: `client_test.go`, `tool_core.go`.

- [ ] **2.0b** — Fix integration `fakeConn.Read` to return `io.EOF` at end, not `(0, nil)`. Violates `io.Reader` contract — over-reading code paths spin forever. Files: `server_integration_test.go:32-39`.

- [ ] **2.0c** — Create a shared `setenv(t, k, v)` helper using `t.Cleanup` restore, and a `clearMikrotikEnv(t)` that scrubs all `MIKROTIK_*` vars before each integration test. Replace hand-rolled save/restore blocks in healthcheck tests. Unblocks `t.Parallel()`. Files: `server_integration_test.go`.

- [ ] **2.0d** — Extract one `fakeConn` into `internal/testutil/fakeconn.go`. Currently duplicated with different EOF semantics across `client_test.go` and `server_integration_test.go`. Files: new.

- [ ] **2.0e** — Fix `readJSONLine` in `mikrotik_test.go` — use a single shared `bufio.Reader` for the pipe across reads, not a new `bufio.Scanner` per call (scanner read-ahead swallows subsequent lines). Files: `mikrotik_test.go:175-204`.

- [ ] **2.0f** — Make `mkReq` a shared test helper (`internal/testutil/mkreq.go`) so `server_test.go` doesn't need copy-pasted struct literals. Files: `server_integration_test.go`, `server_test.go`, new.

- [ ] **2.0g** — Replace `tempDir()` (returns `os.TempDir()`) with `t.TempDir()` in `downloads_test.go`. Files: `downloads_test.go:9-11`.

---

### 2.1 — Structured Content Tests *(depends on D1 = (a))*

**Files:** `internal/formatting/formatting.go`, `internal/formatting/formatting_test.go`, `internal/server/server_integration_test.go`.

- [ ] Patch `CallToolResultFromRecord` to set `result.StructuredContent = record`
- [ ] Patch `CallToolResultFromRecords` to set `result.StructuredContent = map[string]any{"result": items}`
- [ ] Add `nil` check to every existing formatting test and integration test: `if result.StructuredContent == nil { t.Error(...) }`
- [ ] Add table-driven structured-content field checks for the 10 tools that return structured records/lists
- [ ] Port the 10 Python `test_app_*` tests as integration tests asserting both markdown text AND structured content shape

---

### 2.2 — Client Tests: Missing & Weakened *(14 new, ~6 strengthened)*

**Files:** `internal/client/client_test.go`.

**New tests** (port missing Python coverage):

- [ ] **2.2a** — `TestRunReturnsDonePayloadWithoutRecords` — covers `test_command_run_returns_done_payload_without_records`
- [ ] **2.2b** — `TestRunSupportsExplicitTag` — covers `test_command_run_supports_explicit_tag`
- [ ] **2.2c** — `TestExecuteOpensConnectionLazily` — covers `test_execute_opens_connection_lazily_when_socket_is_missing`
- [ ] **2.2d** — `TestListenGeneratesTagWhenNotProvided` — covers `test_listen_generates_tag_when_not_provided`
- [ ] **2.2e** — `TestListenUsesRouterOSDotTagWord` — covers `test_listen_uses_routeros_dot_tag_word`
- [ ] **2.2f** — `TestListenCancelsAfterTimeout` — covers `test_listen_cancels_and_returns_empty_batch_after_timeout`
- [ ] **2.2g** — `TestConnectLoadsCustomCAFiles` — covers `test_connect_loads_active_custom_ca_files`
- [ ] **2.2h** — `TestConnectSkipsCAWhenTLSVerifyDisabled` — covers `test_connect_skips_custom_ca_loading_when_tls_verify_disabled`
- [ ] **2.2i** — `TestConnectWrapsTLSFailureWithCAHint` — covers `test_connect_wraps_tls_failures_with_custom_ca_hint`
- [ ] **2.2j** — `TestTLSSessionInfoReturnsNormalizedDetails` — covers `test_tls_session_info_returns_normalized_certificate_details`
- [ ] **2.2k** — `TestTLSSessionInfoReturnsNilForPlainSocket` — covers `test_tls_session_info_returns_none_for_plain_socket`
- [ ] **2.2l** — `TestIsolatedOpensAndClosesClonedClient` — covers `test_isolated_opens_and_closes_cloned_client`
- [ ] **2.2m** — Rewrite `TestNormalizeItemIDRejectsEmpty` (asserts panic) → `TestSetRequiresNonEmptyItemID` (asserts error with `"item_id is required"`). Covers `test_set_requires_non_empty_item_id`
- [ ] **2.2n** — Rewrite `TestNormalizeMenuRejectsEmpty` (asserts panic) → error-return test on `client.Print("", …)`

**Strengthen existing tests** (replace substring `contains()` with decoded-word-list assertions):

- [ ] **2.2o** — `TestPrintBuildsSentenceAndReturnsRecords`: assert `=.proplist=name,disabled`, `=detail=true`, `?disabled=false`, `?#|` are ALL present in decoded sent words
- [ ] **2.2p** — `TestRemoveBuildsSentenceAndReturnsEmptyResult`: assert decoded words contain `/ip/address/remove` AND `=.id=*3`
- [ ] **2.2q** — `TestRunReturnsRecords`: assert `?status=reachable` query word and `=count=1` attr word present
- [ ] **2.2r** — `TestListenReturnsBoundedRecordsAndCancelsByTag`: assert `=tag=listen-1` in both `/listen` and `/cancel` sentences; assert `result.CancelDone` not nil and empty
- [ ] **2.2s** — `TestLoginTrapRaisesCredentialError`: assert error wraps `ErrRouterOSAuthError` via `errors.Is` — not just message substring

---

### 2.3 — Runtime Tests: Fix & Complete

**Files:** `internal/runtime/runtime_test.go`, `internal/runtime/runtime.go`.

- [ ] **2.3a** — **Delete** the in-test copy `loadTLSCAFilesAt` (lines 94–123). Inject a package-level `var workspaceRoot = WorkspaceRoot` that tests swap. Rewrite both TLS CA tests to call the real `LoadTLSCAFiles`.

- [ ] **2.3b** — Add `TestLoadTLSCAFilesCaseInsensitiveExtension` — proves `.CRT`/`.PEM` uppercase is matched (production line 87 does `strings.ToLower`; the deleted test copy did NOT)

- [ ] **2.3c** — Fix `TestClearEmptyMikrotikEnvVars` — replace no-op comment block with `os.LookupEnv("MIKROTIK_USER")` to prove the var was removed

- [ ] **2.3d** — Add `TestLoadSettingsPassesDiscoveredTLSCAFiles` — verify returned client's TLS CA files contain discovered cert paths

- [ ] **2.3e** — Add `TestLoadSettingsRotatesPasswordWhenPasswordlessEnabled` — env with `API_PASSWORDLESS_ENABLED=true` + fingerprint set → client gets rotated password

- [ ] **2.3f** — Add `TestLoadSettingsRequiresFingerprintWhenPasswordlessEnabled` — passwordless enabled but no `SCP_HOST_FINGERPRINT_SHA256` → specific error

- [ ] **2.3g** — Add `TestLoadSettingsRaisesWhenStartupRotationFails` — rotation mock errors → wrapped message `"Startup password rotation failed: …"`

- [ ] **2.3h** — Add `TestRotateStartupAPIPasswordUsesRequestedLength` — env `LENGTH=40` → 40-char password + correct rotation call

- [ ] **2.3i** — Add `TestRotateStartupAPIPasswordRejectsInvalidLength` — `LENGTH=0` → `"must be at least 1"` error. Replaces the empty `TestGenerateAPIPasswordRejectsZeroLength`

- [ ] **2.3j** — **Delete** `TestGenerateAPIPasswordRejectsZeroLength` (asserts nothing)

- [ ] **2.3k** — **Delete** `TestLoadSettingsDotEnvDoesNotOverrideEnv` (ends with `_ = client`, zero assertions) or add proper assertions

- [ ] **2.3l** — Replace all `os.Chdir` with injected-root pattern (2.3a). Lines 261–265, 291–294, 311–313, 340–343 mutate process-global CWD — blocks `t.Parallel()`

---

### 2.4 — Add the List Handler Query Implementations *(depends on Phase 1.1)*

**Files:** `internal/server/tool_layer2.go`, `tool_security.go`, `tool_access.go`, `tool_files.go`.

The generic `listHandler(cl, menu)` ignores all filter args — queries always `nil`.

- [ ] Replace `listHandler` with `filteredListHandler` that reads declared filter params from `req.Params.Arguments` and builds equality queries via `helpers.BuildEqualityQueries`
- [ ] Create dedicated `fileListHandler` that additionally post-filters by directory prefix using `helpers.FileExistsInDirectory`

---

### 2.5 — Port Healthcheck Tests

**Files:** `internal/server/server_integration_test.go`, `internal/formatting/formatting_test.go`, `internal/runtime/runtime.go`.

Existing Go tests cover 3 of 13 Python scenarios. Add:

- [ ] **2.5a** — `TestIntegrationHealthcheckAPIAuthFailed` — `api.auth_failed` + `scp.config_missing` → overall `"failed"`
- [ ] **2.5b** — `TestIntegrationHealthcheckSCPConfigMissing` — verifies `scp.config_missing` code + fingerprint warning text
- [ ] **2.5c** — `TestIntegrationHealthcheckPasswordlessFingerprintMissing` — no fingerprint → `passwordless.fingerprint_missing`
- [ ] **2.5d** — `TestIntegrationHealthcheckPasswordlessStartupFailed` — startup rotation failed → `passwordless.startup_failed`
- [ ] **2.5e** — `TestIntegrationHealthcheckPasswordlessEnabled` — full passwordless probe succeeds
- [ ] **2.5f** — `TestIntegrationHealthcheckPasswordlessExecFailed` — SSH command probe fails
- [ ] **2.5g** — `TestIntegrationHealthcheckFingerprintProbeFailed` — server fingerprint probe fails
- [ ] **2.5h** — `TestFormatHealthcheckHighlightsMissingFingerprint` — diagnosis output
- [ ] **2.5i** — `TestFormatHealthcheckHighlightsFingerprintMismatch` — diagnosis output

**Production changes needed to support these tests:**

- [ ] **2.5j** — Add `startupPasswordlessState()` / `setStartupPasswordlessState()` to `runtime.go` (matching Python's runtime.py:100–116). Go already has `clearStartupPasswordlessState` but never sets the state
- [ ] **2.5k** — Add curated `formatHealthcheckResult` in `formatting.go` — flattened table + `"Likely issue:"` diagnosis lines. Switch `handlerHealthcheck` from generic `CallToolResultFromRecord` to this dedicated formatter
- [ ] **2.5l** — Assert `structuredContent` on all healthcheck results (ties into 2.1)

---

### 2.6 — Fix `system_backup_collect` Local-Dir + Backups-Dir Creation

**File:** `internal/server/tool_files.go` lines 153–238.

- [ ] Read `local_dir` arg: `localDirArg := argString(req, "local_dir", "")` → pass to `NormalizeLocalDirectory`
- [ ] Port router-side `backups` directory check/create: if `/file?name=backups` returns empty, call `cl.Add("/file", {"name": "backups", "type": "directory"})`

---

## Phase 3: Extended Coverage *(~10–15 days)*

Most of these tests cover behaviour correctly implemented in Go but untested. All new tests in `internal/server/server_integration_test.go`, table-driven.

### 3.1 — SCP/SSH Behavioural Tests *(~17 tests, ~3 days)*

**Files:** new `internal/downloads/downloads_integration_test.go`, `internal/downloads/downloads.go`.

- [ ] **3.1-seam** — Introduce in-memory SSH test server (package-level `var sshDial = ssh.Dial` pattern, or interface injection). Use `net.Pipe()` + `golang.org/x/crypto/ssh.Server` with minimal key + channel handler

- [ ] **3.1a** — `TestSCPFileDownloaderWritesDownloadedFile`
- [ ] **3.1b** — `TestSCPFileDownloaderCheckConnectionSucceeds`
- [ ] **3.1c** — `TestSCPFileDownloaderWrapsConnectFailure`
- [ ] **3.1d** — `TestSCPFileDownloaderWrapsDirectoryProbeFailure`
- [ ] **3.1e** — `TestSCPFileDownloaderWrapsLocalWriteFailure`
- [ ] **3.1f** — `TestSCPFileDownloaderPrefersPrivateKey`
- [ ] **3.1g** — `TestOpenSSHClientRejectsMismatchedHostKey`
- [ ] **3.1h** — `TestOpenSSHClientAllowsMissingHostFingerprint`
- [ ] **3.1i** — `TestLoadFileTransferSettingsUsesExplicitPrivateKey`
- [ ] **3.1j** — `TestLoadFileTransferSettingsDoesNotUseDefaultRouterKey`
- [ ] **3.1k** — `TestLoadFileTransferSettingsRequiresAuthWhenNoKeyOrPassword`
- [ ] **3.1l** — `TestLoadFileTransferSettingsRequiresHostKeyFingerprint` (unset → nil, not error)
- [ ] **3.1m** — `TestLoadFileTransferSettingsRejectsInvalidHostKeyFingerprint`
- [ ] **3.1n** — `TestRotateRouterOSPasswordRunsCommandOverSSH`
- [ ] **3.1o** — `TestRotateRouterOSPasswordWrapsRemoteCommandFailures`
- [ ] **3.1p** — `TestCheckPasswordRotationReadyRunsUserProbe`
- [ ] **3.1q** — `TestCheckPasswordRotationReadyRejectsMissingUser`

---

### 3.2 — Bridge, VLAN, Firewall, PPP, WireGuard Tool Tests *(~45 tests)*

Use table-driven tests. Each row: `name`, `handler`, `args`, `wantSentWords`, `wantErr`. Verify correct menu + attributes sent to fake conn.

- [ ] **bridge** — `list` (name+disabled filters), `add` (attrs + requires-attributes error), `remove` (item_id) (3 tests)
- [ ] **bridge_port** — `list` (bridge+interface+disabled), `add` + requires, `remove` (3)
- [ ] **bridge_vlan** — `list` (bridge+vlan_ids+disabled), `add`, `remove` (3)
- [ ] **vlan** — `list` (name+interface+disabled), `add` + requires, `remove` (3)
- [ ] **firewall_filter** — `list` (chain+action+disabled), `add` + requires, `set` (item_id+attrs + requires-attrs), `remove` (6)
- [ ] **firewall_nat** — same pattern (6)
- [ ] **firewall_rule_move** — success (verifies `/ip/firewall/{table}/move` path + `.id`+`destination` attrs), invalid table, requires destination, requires item_id (4)
- [ ] **firewall_address_list** — `list` (list_name+address+disabled), `add` + requires, `remove` (4)
- [ ] **ppp_active** — `list` (service+name filters) (1)
- [ ] **ppp_secret** — `list` (name+service+disabled), `add` + requires name+password, `remove` (3)
- [ ] **wireguard_interface** — `list` (name+disabled), `add` + requires name, `remove` (3)
- [ ] **wireguard_peer** — `list` (interface+disabled), `add` + requires interface+public-key, `remove` (3)

---

### 3.3 — File, Backup & Export Tests *(~13 tests)*

- [ ] **3.3a** — `TestIntegrationSystemBackupSaveShape` — result contains `success`, `name`, `path` keys with expected values
- [ ] **3.3b** — `TestIntegrationSystemBackupSaveRequiresName` — empty name → `"name is required"`
- [ ] **3.3c** — `TestIntegrationSystemExportShape` — result contains `success`, `name`, `path`, `include_sensitive`, `compact`
- [ ] **3.3d** — `TestIntegrationSystemExportRejectsTrailingSlash` — name `/` → error
- [ ] **3.3e** — `TestIntegrationFileListFiltersByDirectory` — type=script query + directory prefix post-filter
- [ ] **3.3f** — `TestIntegrationFileListRejectsEmptyDirectory` — whitespace → error
- [ ] **3.3g** — `TestIntegrationFileDownloadExplicitPath` — `local_path` provided → downloads to exact path
- [ ] **3.3h** — `TestIntegrationFileDownloadResolvesRelative` — relative path → resolved against workspace_root
- [ ] **3.3i** — `TestIntegrationBackupCollectBothArtifacts` — backup+export succeed, both downloaded with correct naming
- [ ] **3.3j** — `TestIntegrationBackupCollectSkipsDirectoryCreation` — dir exists → no `cl.Add` call
- [ ] **3.3k** — `TestIntegrationBackupCollectDownloadFailure` — both created, step 2 fails → error with paths, only first download called
- [ ] **3.3l** — `TestIntegrationBackupCollectResolvesRelativeLocalDir` — relative `local_dir` resolved from workspace_root
- [ ] **3.3m** — `TestIntegrationBackupCollectUsesCustomLocalDir` — *(depends on 2.6)* `local_dir` arg honoured

---

### 3.4 — Remaining Core Tool Tests *(~25 tests)*

- [ ] **3.4a** — `TestIntegrationResourcePrintAppliesJQFilter` — `map(select(.running=="true"))` → filtered result, JSON-rendered
- [ ] **3.4b** — `TestIntegrationResourcePrintInvalidJQFilter` — `"["` → `"Invalid jq_filter"` error
- [ ] **3.4c** — `TestIntegrationResourceListenPayload` — verify result shape (tag, events, cancelled, limit_reached)
- [ ] **3.4d** — `TestIntegrationCommandRun` — verify `cl.Run` called with correct path, attrs, queries
- [ ] **3.4e** — `TestIntegrationCommandCancel` — verify `cl.Cancel` called with tag
- [ ] **3.4f** — `TestIntegrationToolPingReturnsRecords` — records returned, markdown rendered
- [ ] **3.4g** — `TestIntegrationToolPingDoneOnly` — router returns `!done` only → empty list, `"0 probes"` markdown
- [ ] **3.4h** — `TestIntegrationToolPingPropagatesErrors` — run error propagated
- [ ] **3.4i** — `TestIntegrationToolTracerouteReturnsRecords`
- [ ] **3.4j** — `TestIntegrationToolTracerouteDoneOnly`
- [ ] **3.4k** — `TestIntegrationToolTraceroutePropagatesErrors`
- [ ] **3.4l** — `TestIntegrationDNSResolveReturnsResult` — server specified → `{name, address, server}`
- [ ] **3.4m** — `TestIntegrationDNSResolveRequiresName` — blank → error
- [ ] **3.4n** — `TestIntegrationDNSResolveRequiresAddress` — `ret=""` → error
- [ ] **3.4o** — `TestIntegrationDNSResolvePropagatesErrors`
- [ ] **3.4p** — `TestIntegrationInterfaceMonitorReturnsFirstRecord` — list result → first record
- [ ] **3.4q** — `TestIntegrationInterfaceMonitorAcceptsSingleDict`
- [ ] **3.4r** — `TestIntegrationInterfaceMonitorRequiresName` — blank → error
- [ ] **3.4s** — `TestIntegrationInterfaceMonitorNoResult` — empty list → error
- [ ] **3.4t** — `TestIntegrationInterfaceMonitorPropagatesErrors`
- [ ] **3.4u** — `TestIntegrationDNSSetRequiresAtLeastOne` — no params → error
- [ ] **3.4v** — `TestIntegrationDNSSetNormalizesAttributes` — `servers=["1.1.1.1"," 8.8.8.8 "]` → `"1.1.1.1,8.8.8.8"` on wire
- [ ] **3.4w** — `TestIntegrationIPRouteGetRequiresExactlyOneLocator` — neither dst_address nor item_id → error
- [ ] **3.4x** — `TestIntegrationIPAddressGetRequiresExactlyOneLocator`
- [ ] **3.4y** — `TestIntegrationInterfaceMonitorRequiresName`

> **Phase 1 review notes for Phase 3.4 (non-blocking):**
>
> - [x] **3.4-retro-a — `resource_print` jq output must be JSON:** `handlerResourcePrint` currently returns `fmt.Sprintf("%v", filtered)` for `jq_filter`; change to `helpers.JSONCompact(filtered)` so 3.4a can assert exact JSON shape. File: `internal/server/tool_core.go:307`.
>
> - [x] **3.4-retro-b — `handlerDNSSet` now trims whitespace on `cache_size`:** Fixed.
 
---
 
## Phase 4: Robustness, Hygiene & Go-Specific Dimensions *(~5–7 days)*


### 4.1 — Concurrency Safety

**Files:** `internal/client/client.go`.

`mcp-go` dispatches tool handlers concurrently. All share one `*RouterOSClient` whose `conn` read/write is unsynchronized.

- [ ] Add `execMu sync.Mutex` to `RouterOSClient`, lock in `execute()` and `Listen()`
- [ ] Add `TestConcurrentToolCallsDoNotInterleave` — 5 concurrent goroutines calling a handler; verify 5 complete non-interleaved sentences on the fake conn. Run with `go test -race`

### 4.2 — Read/Write Deadlines

**Files:** `internal/client/client.go`.

`client.timeout` is only used for `Dial`. Python sets `socket.settimeout(timeout)` for all I/O.

- [ ] In `execute()`, set `c.conn.SetDeadline(time.Now().Add(c.timeout))` before the write-and-read loop
- [ ] In `Listen()`, set per-iteration deadlines
- [ ] Add `TestClientHonoursDeadline` — fake conn blocks forever on Read; expect error within timeout window

### 4.3 — Context Propagation

**Files:** `internal/server/tool_core.go` and all handler files.

Handlers receive `ctx context.Context` but ignore it. An MCP client cancelling has no effect on in-flight RouterOS operations.

- [ ] Add `select { case <-ctx.Done(): return ctx.Err(); default: }` at the start of every handler
- [ ] (Longer-term) thread `ctx` through `cl.Run`/`cl.Print`/`cl.Listen`
- [ ] Add `TestContextCancellationAbortsCall` — cancel context before handler → `context.Canceled` propagated

### 4.4 — Network Guardrail

**File:** new `internal/testutil/network_guard.go`.

Python enforces `--disable-socket` via `pytest-socket`. Go has no equivalent — a test that accidentally calls `Isolated()` without a fake conn dials real TCP.

- [ ] Add `init()` guard in a `//go:build testing` file — panics unless `MIKROTIK_TEST_ALLOW_NETWORK=1`

### 4.5 — Proactive Cleanup & Hygiene Deletions

- [ ] **4.5a** — Delete dead code `sshHostKeySHA256` (`downloads.go:457-462`)
- [ ] **4.5b** — Delete dead code `headers` var block in `renderTable` (`formatting.go:72-79`)
- [ ] **4.5c** — Delete dead test code `mockSCPDownloader.wrap()` (`server_integration_test.go:491-496`)
- [ ] **4.5d** — Delete `TestGenerateAPIPasswordRejectsZeroLength` (empty test — "may or may not error")
- [ ] **4.5e** — Delete `TestLoadSettingsDotEnvDoesNotOverrideEnv` (zero assertions, `_ = client`)
- [ ] **4.5f** — Delete `TestGenerateAPIPasswordIsRandom` (near-zero information)
- [ ] **4.5g** — Deduplicate `clearMikrotikEnv`/`clearMikrotikOnly` across 3 test files → `internal/testutil/env.go`
- [ ] **4.5h** — Run `gofumpt -s .` or `gofmt -s -w .` on the whole tree

> **Phase 1 review note for Phase 4.5 (non-blocking):**
>
> - [ ] **4.5-retro — Revert unnecessary `go.mod` / `go.sum` bumps:** The change from `go 1.23.0` to `go 1.25.0` and the `golang.org/x/crypto`/`sys` bumps are not required by any Phase 1 fix. Revert them unless there is a concrete dependency requirement, to avoid raising the minimum Go toolchain unnecessarily.

### 4.6 — Deterministic Attr Word Order

**File:** `internal/client/client.go` lines 759–768.

`normalizeAttrs` iterates a map → random word order in built sentences. This forces `contains()` assertions instead of exact-byte comparison.

- [ ] Sort the keys before emitting attr words:
  ```go
  func normalizeAttrs(attrs map[string]any) []struct{ key, value string } {
      type kv struct{ k, v string }
      var sorted []kv
      for k, v := range attrs {
          if v != nil { sorted = append(sorted, kv{k, stringifyValue(v)}) }
      }
      sort.Slice(sorted, func(i, j int) bool { return sorted[i].k < sorted[j].k })
      return sorted
  }
  ```
- [ ] Adapt `buildMenuSentence`, `Print`, `buildCommandSentence` callers to use the sorted return type

---

## Dependency Graph

```
                  Phase 1.1 (schemas)
                  Phase 1.4 (panic recovery)
                  Phase 1.6 (private-key error)
                  Phase 2.0a-g (test infra)
                        |
   Phase 1.2 ──────────┤
   (attrs fix)         |
   Phase 1.3 (CA) ─────┤
   Phase 1.5 (ping) ───┤
                        |
        ┌───────────────┼───────────────┐
        v               v               v
 Phase 2.1         Phase 2.2        Phase 2.3
 (structured       (client          (runtime
  content)          tests)           tests)
        │               │               │
        └───────┬───────┘               │
                v                       │
         Phase 2.4 (list filters)       │
         Phase 2.5 (healthcheck) ───────┘
         Phase 2.6 (local dir fix)
                │
        ┌───────┼───────┐
        v       v       v
  Phase 3.1   Phase 3.2  Phase 3.3
  (SCP)       (bridge,   (file/backup)
               fw, etc.)
        │       │       │
        └───────┴───────┘
                │
          ┌─────┴─────┐
          v           v
   Phase 4.1–4.3  Phase 4.4–4.6
   (concurrency,  (guardrail,
    deadlines,     hygiene,
    context)       deterministic attrs)
```

**Phase 3** and **Phase 4** are independent and can be done in parallel.

---

## Effort Summary

| Phase | Description | Approx. Days |
|---|---|---|
| 0 | Audit review & decisions | 0.5 |
| 1 | Critical production fixes | 3–5 |
| 2 | Core test porting | 8–12 |
| 3 | Extended coverage | 10–15 |
| 4 | Robustness & hygiene | 5–7 |
| **Total** | | **27–40** |
