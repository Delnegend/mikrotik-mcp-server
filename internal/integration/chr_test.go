// Package integration runs the client and the real MCP tool handlers against
// a live RouterOS instance (the QEMU CHR from scripts/chr/up.sh).
//
// Gating: skipped when MIKROTIK_TEST_HOST is unset; when it is set but the
// router is unreachable the suite FAILS (so a misconfigured run is loud, not
// silently green).
//
//	MIKROTIK_TEST_HOST=127.0.0.1 MIKROTIK_TEST_USER=admin MIKROTIK_TEST_PASSWORD=admin \
//	  go test ./internal/integration/ -v
//
// Optional: MIKROTIK_TEST_PORT (8728), MIKROTIK_TEST_SSH_PORT (2222).
// Opt-in:   MIKROTIK_TEST_PASSWORDLESS=1 exercises real SSH password rotation.
package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/downloads"
	"github.com/Delnegend/mikrotik-mcp/internal/inventory"
	mcpapp "github.com/Delnegend/mikrotik-mcp/internal/server"
	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"golang.org/x/crypto/ssh"
)

var (
	chrHost    string
	chrUser    string
	chrPass    string
	chrPort    int
	chrSSHPort int
	chrClient  *client.RouterOSClient
	mcpSrv     *mcpserver.MCPServer
)

func TestMain(m *testing.M) {
	chrHost = os.Getenv("MIKROTIK_TEST_HOST")
	if chrHost != "" {
		chrUser = envOr("MIKROTIK_TEST_USER", "admin")
		chrPass = os.Getenv("MIKROTIK_TEST_PASSWORD")
		chrPort = envInt("MIKROTIK_TEST_PORT", 8728)
		chrSSHPort = envInt("MIKROTIK_TEST_SSH_PORT", 2222)

		var err error
		chrClient, err = connectWithRetry()
		if err != nil {
			fmt.Fprintf(os.Stderr, "integration: cannot reach RouterOS at %s:%d: %v\n", chrHost, chrPort, err)
			os.Exit(1)
		}
		reg := inventory.Single(inventory.Device{
			Title: chrHost, Host: chrHost, Port: chrPort,
			Username: chrUser, Password: chrPass,
			APISSL: false, TLSVerify: true, Timeout: 20 * time.Second,
		})
		mcpSrv = mcpapp.NewMCPServer(reg)
	}
	code := m.Run()
	if chrClient != nil {
		chrClient.Close()
	}
	os.Exit(code)
}

func requireRouter(t *testing.T) {
	t.Helper()
	if chrClient == nil {
		t.Skip("set MIKROTIK_TEST_HOST to run the RouterOS integration suite")
	}
}

// callTool invokes a tool through the real MCP server tool registry.
func callTool(t *testing.T, name string, args ...any) *mcp.CallToolResult {
	t.Helper()
	return callToolOn(t, mcpSrv, name, args...)
}

func callToolOn(t *testing.T, srv *mcpserver.MCPServer, name string, args ...any) *mcp.CallToolResult {
	t.Helper()
	requireRouter(t)
	tool := srv.GetTool(name)
	if tool == nil {
		t.Fatalf("tool %q is not registered", name)
	}
	res, err := tool.Handler(context.Background(), testutil.MkReq(name, args...))
	if err != nil {
		t.Fatalf("tool %s: %v", name, err)
	}
	return res
}

func toolText(t *testing.T, name string, args ...any) string {
	t.Helper()
	return toolTextOn(t, mcpSrv, name, args...)
}

func toolTextOn(t *testing.T, srv *mcpserver.MCPServer, name string, args ...any) string {
	t.Helper()
	res := callToolOn(t, srv, name, args...)
	if res.IsError {
		t.Fatalf("tool %s returned an error: %v", name, res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatalf("tool %s returned no content", name)
	}
	return res.Content[0].(mcp.TextContent).Text
}

func newTestClient(t *testing.T) *client.RouterOSClient {
	t.Helper()
	cl := client.NewRouterOSClient(chrHost, chrUser, chrPass,
		client.WithTLS(false), client.WithPort(chrPort), client.WithTimeout(15*time.Second))
	if err := cl.Open(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl
}

var (
	sshFingerprintOnce sync.Once
	sshFingerprint     string
)

func chrSSHSettings(t *testing.T) *downloads.FileTransferSettings {
	t.Helper()
	fp := os.Getenv("MIKROTIK_TEST_SSH_FINGERPRINT")
	if fp == "" {
		// Probe once per test binary: each probe is a deliberately bogus
		// SSH login that the router logs as a failure.
		sshFingerprintOnce.Do(func() {
			probe, err := downloads.ProbeSSHFingerprint("127.0.0.1", chrSSHPort, 10*time.Second)
			if err != nil {
				sshFingerprint = ""
				return
			}
			sshFingerprint, _ = probe["fingerprint_sha256"].(string)
		})
		fp = sshFingerprint
		if fp == "" {
			t.Fatal("no SSH fingerprint from probe (is the CHR SSH service up?)")
		}
	}
	return &downloads.FileTransferSettings{
		Host:                 "127.0.0.1",
		Port:                 chrSSHPort,
		Username:             chrUser,
		Password:             chrPass,
		SSHFingerprintSHA256: fp,
		Timeout:              15 * time.Second,
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func connectWithRetry() (*client.RouterOSClient, error) {
	var lastErr error
	for range 6 {
		cl := client.NewRouterOSClient(chrHost, chrUser, chrPass,
			client.WithTLS(false), client.WithPort(chrPort), client.WithTimeout(20*time.Second))
		if err := cl.Open(); err == nil {
			return cl, nil
		} else {
			lastErr = err
		}
		time.Sleep(5 * time.Second)
	}
	return nil, lastErr
}

// --- Client-level tests ----------------------------------------------------

func TestClientLoginAndPrint(t *testing.T) {
	requireRouter(t)

	identity, err := chrClient.Print("/system/identity", nil, nil, nil)
	if err != nil {
		t.Fatalf("print identity: %v", err)
	}
	if len(identity) != 1 || identity[0]["name"] == "" {
		t.Fatalf("identity = %v", identity)
	}
	t.Logf("identity: %q", identity[0]["name"])

	ifaces, err := chrClient.Print("/interface", nil, nil, nil)
	if err != nil {
		t.Fatalf("print interfaces: %v", err)
	}
	if len(ifaces) == 0 {
		t.Error("expected at least one interface")
	}
}

func TestClientMutationLifecycle(t *testing.T) {
	requireRouter(t)

	id, err := chrClient.Add("/ip/firewall/address-list",
		map[string]any{"list": "mcp-itest", "address": "198.51.100.77"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	retID := fmt.Sprint(id["ret"])
	t.Cleanup(func() { chrClient.Remove("/ip/firewall/address-list", retID) })

	items, err := chrClient.Print("/ip/firewall/address-list", nil,
		[]string{"list=mcp-itest"}, nil)
	if err != nil {
		t.Fatalf("print: %v", err)
	}
	if len(items) != 1 || items[0]["address"] != "198.51.100.77" {
		t.Fatalf("after add, items = %v", items)
	}

	if _, err := chrClient.Set("/ip/firewall/address-list", retID,
		map[string]any{"comment": "mcp-itest"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := chrClient.Remove("/ip/firewall/address-list", retID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	t.Cleanup(func() {})
	items, err = chrClient.Print("/ip/firewall/address-list", nil,
		[]string{"list=mcp-itest"}, nil)
	if err != nil {
		t.Fatalf("print after remove: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected list empty after remove, got %v", items)
	}
}

func TestClientListenSeesChange(t *testing.T) {
	requireRouter(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The listener holds the shared connection's command lock, so trigger the
	// change from a second connection.
	cl2 := newTestClient(t)
	ch := make(chan *client.ListenResult, 1)
	errCh := make(chan error, 1)
	go func() {
		r, err := chrClient.ListenContext(ctx, "/ip/firewall/address-list", nil, nil, nil, "itest-listen", 1)
		if err != nil {
			errCh <- err
			return
		}
		ch <- r
	}()

	time.Sleep(2 * time.Second) // let the listen establish
	added, err := cl2.Add("/ip/firewall/address-list",
		map[string]any{"list": "mcp-listen", "address": "198.51.100.80"})
	if err != nil {
		t.Fatalf("trigger add: %v", err)
	}
	retID := fmt.Sprint(added["ret"])
	t.Cleanup(func() { cl2.Remove("/ip/firewall/address-list", retID) })

	select {
	case r := <-ch:
		if len(r.Records) == 0 {
			t.Error("expected at least one listen event")
		}
		t.Logf("listen events=%d cancelled=%v", len(r.Records), r.Cancelled)
	case err := <-errCh:
		t.Fatalf("listen: %v", err)
	case <-time.After(25 * time.Second):
		t.Fatal("timeout waiting for listen event")
	}
}

func TestClientBadLoginFails(t *testing.T) {
	requireRouter(t)
	// A clearly-fake username keeps the deliberate failed login out of the
	// router's admin-account logs.
	bad := client.NewRouterOSClient(chrHost, "mcp-nonexistent-user", "definitely-wrong",
		client.WithTLS(false), client.WithPort(chrPort), client.WithTimeout(5*time.Second))
	if err := bad.Open(); err == nil {
		t.Fatal("expected error for wrong password")
	} else if !errors.Is(err, client.ErrRouterOSAuthError) && !strings.Contains(err.Error(), "login") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientIsolatedPing(t *testing.T) {
	requireRouter(t)
	var records []map[string]string
	err := chrClient.Isolated(func(iso *client.RouterOSClient) error {
		res, err := iso.Run("/tool/ping", map[string]any{"address": "127.0.0.1", "count": 1}, nil, "")
		if err != nil {
			return err
		}
		if recs, ok := res.([]map[string]string); ok {
			records = recs
		}
		return nil
	})
	if err != nil {
		t.Fatalf("isolated ping: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("no ping records returned")
	}
}

// --- Tool-level tests (through the real MCP registry) ----------------------

func TestToolReadOnlyFamilies(t *testing.T) {
	requireRouter(t)

	if text := toolText(t, "system_identity_get"); text == "" {
		t.Error("system_identity_get returned empty")
	} else {
		t.Logf("identity: %s", strings.TrimSpace(strings.SplitN(text, "\n", 2)[0]))
	}
	if text := toolText(t, "interface_list"); !strings.Contains(text, "ether") {
		t.Errorf("interface_list missing interfaces: %s", text)
	}
	if text := toolText(t, "ip_address_list"); !strings.Contains(text, "IP Addresses") {
		t.Errorf("ip_address_list: %s", text)
	}
	if text := toolText(t, "system_resource_get"); !strings.Contains(text, "RouterOS") {
		t.Errorf("system_resource_get: %s", text)
	}
	if text := toolText(t, "dhcp_server_list"); !strings.Contains(text, "DHCP Servers") {
		t.Errorf("dhcp_server_list: %s", text)
	}
}

func TestToolMutationLifecycle(t *testing.T) {
	requireRouter(t)

	// firewall_address_list_add -> verify via client -> remove via tool.
	_ = toolText(t, "firewall_address_list_add",
		"attributes", map[string]any{"list": "mcp-tool", "address": "198.51.100.78"})

	items, err := chrClient.Print("/ip/firewall/address-list", nil, []string{"list=mcp-tool"}, nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("after add, items = %v (err=%v)", items, err)
	}
	id := items[0][".id"]
	t.Cleanup(func() { chrClient.Remove("/ip/firewall/address-list", id) })

	_ = toolText(t, "firewall_address_list_remove", "item_id", id)
	items, err = chrClient.Print("/ip/firewall/address-list", nil, []string{"list=mcp-tool"}, nil)
	if err != nil || len(items) != 0 {
		t.Fatalf("expected empty list after tool remove, got %v (err=%v)", items, err)
	}

	// Generic resource_add/set/remove on the same menu.
	_ = toolText(t, "resource_add",
		"menu", "ip/firewall/address-list",
		"attributes", map[string]any{"list": "mcp-tool", "address": "198.51.100.79"})
	items, err = chrClient.Print("/ip/firewall/address-list", nil, []string{"list=mcp-tool"}, nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("resource_add result items = %v (err=%v)", items, err)
	}
	gid := items[0][".id"]
	_ = toolText(t, "resource_set", "menu", "ip/firewall/address-list", "item_id", gid,
		"attributes", map[string]any{"comment": "mcp-tool"})
	_ = toolText(t, "resource_remove", "menu", "ip/firewall/address-list", "item_id", gid)
	items, err = chrClient.Print("/ip/firewall/address-list", nil, []string{"list=mcp-tool"}, nil)
	if err != nil || len(items) != 0 {
		t.Fatalf("expected empty list after resource_remove, got %v (err=%v)", items, err)
	}
}

func TestToolNetwork(t *testing.T) {
	requireRouter(t)

	if text := toolText(t, "tool_ping", "address", "127.0.0.1", "count", 1); !strings.Contains(text, "Ping") {
		t.Errorf("tool_ping: %s", text)
	}
	if text := toolText(t, "dns_resolve", "name", "localhost"); !strings.Contains(text, "127.0.0.1") {
		t.Errorf("dns_resolve: %s", text)
	}
	if text := toolText(t, "interface_monitor", "name", "ether1"); !strings.Contains(text, "Interface monitor") {
		t.Errorf("interface_monitor: %s", text)
	}
}

func TestToolHealthcheckHealthy(t *testing.T) {
	requireRouter(t)
	s := chrSSHSettings(t)
	t.Setenv("MIKROTIK_SCP_HOST", s.Host)
	t.Setenv("MIKROTIK_SCP_PORT", strconv.Itoa(s.Port))
	t.Setenv("MIKROTIK_SCP_USER", s.Username)
	t.Setenv("MIKROTIK_SCP_PASSWORD", s.Password)
	t.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", s.SSHFingerprintSHA256)
	t.Setenv("MIKROTIK_USER", chrUser)
	t.Setenv("MIKROTIK_PASSWORD", chrPass)

	text := toolText(t, "healthcheck")
	if !strings.Contains(text, "healthy") {
		t.Errorf("healthcheck not healthy: %s", text)
	}
}

// --- Downloads (real SFTP/SSH against the CHR) -----------------------------

func TestDownloadsSFTP(t *testing.T) {
	requireRouter(t)
	if chrPass == "" {
		t.Skip("downloads need a non-empty password (provision with cmd/chrprovision)")
	}

	// Create a router file to download.
	_, err := chrClient.Run("/system/backup/save", map[string]any{"name": "chr-itest"}, nil, "")
	if err != nil {
		t.Fatalf("backup save: %v", err)
	}
	t.Cleanup(func() {
		if items, e := chrClient.Print("/file", nil, []string{"name=chr-itest.backup"}, nil); e == nil && len(items) > 0 {
			chrClient.Remove("/file", items[0][".id"])
		}
	})

	dl := downloads.NewSCPFileDownloader(chrSSHSettings(t))
	check, err := dl.CheckConnection()
	if err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
	t.Logf("sftp check: %v", check)

	local := filepath.Join(t.TempDir(), "chr-itest.backup")
	if err := dl.DownloadFile("chr-itest.backup", local); err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if fi, err := os.Stat(local); err != nil || fi.Size() == 0 {
		t.Fatalf("downloaded file missing/empty: %v", err)
	}
}

func TestDownloadsHostKeyMismatchFails(t *testing.T) {
	requireRouter(t)
	settings := chrSSHSettings(t)
	settings.SSHFingerprintSHA256 = "SHA256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	dl := downloads.NewSCPFileDownloader(settings)
	if _, err := dl.CheckConnection(); err == nil {
		t.Fatal("expected host key mismatch error")
	} else if !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestPasswordlessRotation exercises the real SSH-based password rotation
// used by the passwordless startup feature. Opt-in: MIKROTIK_TEST_PASSWORDLESS=1.
func TestPasswordlessRotation(t *testing.T) {
	if os.Getenv("MIKROTIK_TEST_PASSWORDLESS") != "1" {
		t.Skip("set MIKROTIK_TEST_PASSWORDLESS=1 to run the SSH password rotation test")
	}
	requireRouter(t)
	if chrPass == "" {
		t.Skip("rotation needs a non-empty password to restore")
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("ssh public key: %v", err)
	}
	pubAuth := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	// Upload the public key to the admin user via the API.
	if _, err := chrClient.Add("/user/ssh-keys", map[string]any{
		"user": chrUser, "key": pubAuth,
	}); err != nil {
		t.Fatalf("add ssh key: %v", err)
	}
	t.Cleanup(func() {
		if items, e := chrClient.Print("/user/ssh-keys", nil, nil, nil); e == nil && len(items) > 0 {
			for _, it := range items {
				chrClient.Remove("/user/ssh-keys", it[".id"])
			}
		}
	})

	keyPath := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(keyPath, privPEM, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	fp := chrSSHSettings(t).SSHFingerprintSHA256
	t.Setenv("MIKROTIK_SCP_PRIVATE_KEY", keyPath)
	t.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", fp)
	t.Setenv("MIKROTIK_SCP_HOST", "127.0.0.1")
	t.Setenv("MIKROTIK_SCP_PORT", strconv.Itoa(chrSSHPort))
	t.Setenv("MIKROTIK_USER", chrUser)
	t.Setenv("MIKROTIK_PASSWORD", chrPass)

	newPass, err := downloads.RotateRouterOSPassword("127.0.0.1", chrUser)
	if err != nil {
		t.Fatalf("RotateRouterOSPassword: %v", err)
	}
	if newPass == "" || newPass == chrPass {
		t.Fatalf("rotation returned %q", newPass)
	}

	// Restore the original password and drop the key.
	restore := client.NewRouterOSClient(chrHost, chrUser, newPass,
		client.WithTLS(false), client.WithPort(chrPort), client.WithTimeout(10*time.Second))
	if err := restore.Open(); err != nil {
		t.Fatalf("login with rotated password: %v", err)
	}
	users, err := restore.Print("/user", nil, []string{"name=" + chrUser}, nil)
	if err != nil || len(users) != 1 {
		t.Fatalf("user lookup: %v (%v)", users, err)
	}
	if _, err := restore.Set("/user", users[0][".id"], map[string]any{"password": chrPass}); err != nil {
		t.Fatalf("restore password: %v", err)
	}
	restore.Close()
	t.Logf("password rotated and restored")
}

// --- Fleet / multi-device -------------------------------------------------

// newFleetServer builds a server managing a two-device fleet, both targeting
// the live CHR so per-command connections are exercised for real.
func newFleetServer(t *testing.T) *mcpserver.MCPServer {
	t.Helper()
	devs := []map[string]any{
		{"title": "RouterA", "host": chrHost, "port": chrPort, "username": chrUser, "password": chrPass, "api_ssl": false},
		{"title": "RouterB", "host": chrHost, "port": chrPort, "username": chrUser, "password": chrPass, "api_ssl": false},
	}
	data, err := json.Marshal(devs)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	reg, err := inventory.Parse(data)
	if err != nil {
		t.Fatalf("parse inventory: %v", err)
	}
	return mcpapp.NewMCPServer(reg)
}

func TestFleetListDevices(t *testing.T) {
	requireRouter(t)
	// A throwaway user with a distinctive password so we can prove credentials
	// never leak into list_devices.
	fleetUser := fmt.Sprintf("fleet-%d", time.Now().UnixNano())
	secret := "fleet-secret-pw"
	added, err := chrClient.Add("/user", map[string]any{
		"name": fleetUser, "password": secret, "group": "full",
	})
	if err != nil {
		t.Fatalf("create fleet user: %v", err)
	}
	t.Cleanup(func() { chrClient.Remove("/user", fmt.Sprint(added["ret"])) })

	devs := []map[string]any{
		{"title": "RouterA", "host": chrHost, "port": chrPort, "username": chrUser, "password": chrPass, "api_ssl": false},
		{"title": "RouterB", "host": chrHost, "port": chrPort, "username": fleetUser, "password": secret, "api_ssl": false},
	}
	data, _ := json.Marshal(devs)
	reg, err := inventory.Parse(data)
	if err != nil {
		t.Fatalf("parse inventory: %v", err)
	}
	text := toolTextOn(t, mcpapp.NewMCPServer(reg), "list_devices")
	for _, want := range []string{"RouterA", "RouterB", chrHost} {
		if !strings.Contains(text, want) {
			t.Errorf("list_devices missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, secret) {
		t.Errorf("list_devices must not leak credentials: %s", text)
	}
}

func TestFleetListDevicesSingle(t *testing.T) {
	requireRouter(t)
	text := toolText(t, "list_devices")
	if !strings.Contains(text, "127.0.0.1") {
		t.Errorf("single-device list_devices should show the host: %s", text)
	}
}

func TestFleetDeviceRouting(t *testing.T) {
	requireRouter(t)
	srv := newFleetServer(t)

	if text := toolTextOn(t, srv, "system_identity_get", "device", "RouterA"); text == "" {
		t.Error("system_identity_get with device=RouterA returned empty")
	}

	// Unknown device: handler returns an error that lists the fleet.
	tool := srv.GetTool("system_identity_get")
	if _, err := tool.Handler(context.Background(), testutil.MkReq("system_identity_get", "device", "Nope")); err == nil {
		t.Fatal("expected error for unknown device")
	} else if !strings.Contains(err.Error(), "RouterA") || !strings.Contains(err.Error(), "RouterB") {
		t.Errorf("unknown-device error should list the fleet: %v", err)
	}

	// Omitting device on a fleet: handler returns an error.
	if _, err := tool.Handler(context.Background(), testutil.MkReq("system_identity_get")); err == nil {
		t.Fatal("expected error when device omitted on a fleet")
	} else if !strings.Contains(err.Error(), "device is required") {
		t.Errorf("missing-device error = %v", err)
	}
}

func TestFleetDeviceNotForwarded(t *testing.T) {
	requireRouter(t)
	srv := newFleetServer(t)

	// A mutation with device= must succeed; if "device" leaked into the
	// RouterOS attributes, the router would reject it as an unknown parameter.
	_ = toolTextOn(t, srv, "firewall_address_list_add",
		"device", "RouterA",
		"attributes", map[string]any{"list": "mcp-fleet", "address": "198.51.100.90"})

	items, err := chrClient.Print("/ip/firewall/address-list", nil, []string{"list=mcp-fleet"}, nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("mutation not applied via device routing: %v (items=%v)", err, items)
	}
	t.Cleanup(func() { chrClient.Remove("/ip/firewall/address-list", items[0][".id"]) })
}

// --- Safe mode -------------------------------------------------------------

// newSafeModeServer builds a server whose single device carries the CHR SSH
// settings (port 2222 + real host-key fingerprint), so safe mode can open a
// console session.
func newSafeModeServer(t *testing.T) *mcpserver.MCPServer {
	t.Helper()
	ssh := chrSSHSettings(t)
	reg := inventory.Single(inventory.Device{
		Title:                "chr",
		Host:                 "127.0.0.1",
		Port:                 chrPort,
		Username:             chrUser,
		Password:             chrPass,
		APISSL:               false,
		Timeout:              20 * time.Second,
		SSHPort:              chrSSHPort,
		SSHUsername:          chrUser,
		SSHFingerprintSHA256: ssh.SSHFingerprintSHA256,
	})
	return mcpapp.NewMCPServer(reg)
}

func waitFor(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// rollbackBestEffort leaves safe mode if it is still active; tolerates the
// already-inactive state so cleanups are safe to run after a successful
// rollback.
func rollbackBestEffort(t *testing.T, srv *mcpserver.MCPServer) {
	t.Helper()
	tool := srv.GetTool("rollback_safe_mode")
	if tool == nil {
		return
	}
	_, _ = tool.Handler(context.Background(), testutil.MkReq("rollback_safe_mode"))
}

func TestSafeModeRollbackRevertsChanges(t *testing.T) {
	requireRouter(t)
	srv := newSafeModeServer(t)

	if text := toolTextOn(t, srv, "safe_mode_status"); !strings.Contains(text, "NOT active") {
		t.Fatalf("expected safe mode inactive at start: %s", text)
	}
	_ = toolTextOn(t, srv, "enable_safe_mode")
	if text := toolTextOn(t, srv, "safe_mode_status"); !strings.Contains(text, "ACTIVE") {
		t.Fatalf("expected safe mode active: %s", text)
	}
	t.Cleanup(func() { rollbackBestEffort(t, srv) })

	// A change made through the tool is routed via the safe-mode CLI session.
	_ = toolTextOn(t, srv, "firewall_address_list_add",
		"attributes", map[string]any{"list": "mcp-safe", "address": "198.51.100.91"})

	// In-memory changes are visible to the API.
	waitFor(t, "safe-mode change to appear", func() bool {
		items, err := chrClient.Print("/ip/firewall/address-list", nil, []string{"list=mcp-safe"}, nil)
		return err == nil && len(items) == 1
	})

	// Rollback drops the session; RouterOS reverts everything.
	_ = toolTextOn(t, srv, "rollback_safe_mode")
	waitFor(t, "rollback to revert the change", func() bool {
		items, err := chrClient.Print("/ip/firewall/address-list", nil, []string{"list=mcp-safe"}, nil)
		return err == nil && len(items) == 0
	})
	if text := toolTextOn(t, srv, "safe_mode_status"); !strings.Contains(text, "NOT active") {
		t.Errorf("expected safe mode inactive after rollback: %s", text)
	}
}

func TestSafeModeCommitPersistsChanges(t *testing.T) {
	requireRouter(t)
	srv := newSafeModeServer(t)

	_ = toolTextOn(t, srv, "enable_safe_mode")
	t.Cleanup(func() { rollbackBestEffort(t, srv) })

	_ = toolTextOn(t, srv, "firewall_address_list_add",
		"attributes", map[string]any{"list": "mcp-commit", "address": "198.51.100.92"})

	_ = toolTextOn(t, srv, "commit_safe_mode")
	if text := toolTextOn(t, srv, "safe_mode_status"); !strings.Contains(text, "NOT active") {
		t.Errorf("expected safe mode inactive after commit: %s", text)
	}

	// Committed changes persist (visible via the API).
	var items []map[string]string
	waitFor(t, "committed change to persist", func() bool {
		var err error
		items, err = chrClient.Print("/ip/firewall/address-list", nil, []string{"list=mcp-commit"}, nil)
		return err == nil && len(items) == 1
	})
	t.Cleanup(func() { chrClient.Remove("/ip/firewall/address-list", items[0][".id"]) })
}

// --- Enthusiast workflows: interface/IP/DHCP setup -------------------------

func waitForOne(t *testing.T, menu, query string) map[string]string {
	t.Helper()
	var items []map[string]string
	waitFor(t, fmt.Sprintf("%s %q to appear", menu, query), func() bool {
		var err error
		items, err = chrClient.Print(menu, nil, []string{query}, nil)
		return err == nil && len(items) == 1
	})
	return items[0]
}

func waitForGone(t *testing.T, menu, query string) {
	t.Helper()
	waitFor(t, fmt.Sprintf("%s %q to disappear", menu, query), func() bool {
		items, err := chrClient.Print(menu, nil, []string{query}, nil)
		return err == nil && len(items) == 0
	})
}

func TestToolVLANLifecycle(t *testing.T) {
	requireRouter(t)

	_ = toolText(t, "vlan_add",
		"attributes", map[string]any{"name": "mcp-vlan", "vlan-id": 3999, "interface": "ether1"})
	vlanID := waitForOne(t, "/interface/vlan", "name=mcp-vlan")[".id"]
	t.Cleanup(func() { chrClient.Remove("/interface/vlan", vlanID) })

	if text := toolText(t, "vlan_list"); !strings.Contains(text, "mcp-vlan") {
		t.Errorf("vlan_list missing mcp-vlan: %s", text)
	}

	_ = toolText(t, "vlan_remove", "item_id", vlanID)
	waitForGone(t, "/interface/vlan", "name=mcp-vlan")
}

func TestToolBridgeWithVLANPort(t *testing.T) {
	requireRouter(t)

	_ = toolText(t, "bridge_add", "attributes", map[string]any{"name": "mcp-br"})
	brID := waitForOne(t, "/interface/bridge", "name=mcp-br")[".id"]
	t.Cleanup(func() { chrClient.Remove("/interface/bridge", brID) })

	_ = toolText(t, "vlan_add",
		"attributes", map[string]any{"name": "mcp-vlan", "vlan-id": 3998, "interface": "ether1"})
	vlanID := waitForOne(t, "/interface/vlan", "name=mcp-vlan")[".id"]
	t.Cleanup(func() { chrClient.Remove("/interface/vlan", vlanID) })

	_ = toolText(t, "bridge_port_add",
		"attributes", map[string]any{"bridge": "mcp-br", "interface": "mcp-vlan"})
	portID := waitForOne(t, "/interface/bridge/port", "interface=mcp-vlan")[".id"]
	t.Cleanup(func() { chrClient.Remove("/interface/bridge/port", portID) })

	if text := toolText(t, "bridge_port_list", "bridge", "mcp-br"); !strings.Contains(text, "mcp-vlan") {
		t.Errorf("bridge_port_list missing mcp-vlan: %s", text)
	}

	_ = toolText(t, "bridge_port_remove", "item_id", portID)
	waitForGone(t, "/interface/bridge/port", "interface=mcp-vlan")
}

func TestToolFullIPSetup(t *testing.T) {
	requireRouter(t)

	// A complete small-network setup an enthusiast would build: a VLAN, an
	// address on it, an address pool, a DHCP network/server, and a static
	// route via the VLAN gateway. Teardown (LIFO) reverses the setup order.
	_ = toolText(t, "vlan_add",
		"attributes", map[string]any{"name": "mcp-net", "vlan-id": 3997, "interface": "ether1"})
	vlanID := waitForOne(t, "/interface/vlan", "name=mcp-net")[".id"]
	t.Cleanup(func() { chrClient.Remove("/interface/vlan", vlanID) })

	_ = toolText(t, "resource_add", "menu", "ip/address",
		"attributes", map[string]any{"address": "10.77.0.1/24", "interface": "mcp-net"})
	addrID := waitForOne(t, "/ip/address", "address=10.77.0.1/24")[".id"]
	t.Cleanup(func() { chrClient.Remove("/ip/address", addrID) })

	_ = toolText(t, "resource_add", "menu", "ip/pool",
		"attributes", map[string]any{"name": "mcp-pool", "ranges": "10.77.0.100-10.77.0.200"})
	poolID := waitForOne(t, "/ip/pool", "name=mcp-pool")[".id"]
	t.Cleanup(func() { chrClient.Remove("/ip/pool", poolID) })

	_ = toolText(t, "resource_add", "menu", "ip/dhcp-server/network",
		"attributes", map[string]any{"address": "10.77.0.0/24", "gateway": "10.77.0.1", "dns-server": "1.1.1.1"})
	netID := waitForOne(t, "/ip/dhcp-server/network", "address=10.77.0.0/24")[".id"]
	t.Cleanup(func() { chrClient.Remove("/ip/dhcp-server/network", netID) })

	_ = toolText(t, "resource_add", "menu", "ip/dhcp-server",
		"attributes", map[string]any{"name": "mcp-dhcp", "interface": "mcp-net", "address-pool": "mcp-pool"})
	dhcpID := waitForOne(t, "/ip/dhcp-server", "name=mcp-dhcp")[".id"]
	t.Cleanup(func() { chrClient.Remove("/ip/dhcp-server", dhcpID) })

	if text := toolText(t, "dhcp_server_list"); !strings.Contains(text, "mcp-dhcp") {
		t.Errorf("dhcp_server_list missing mcp-dhcp: %s", text)
	}

	_ = toolText(t, "resource_add", "menu", "ip/route",
		"attributes", map[string]any{"dst-address": "198.51.100.0/24", "gateway": "10.77.0.1"})
	routeID := waitForOne(t, "/ip/route", "dst-address=198.51.100.0/24")[".id"]
	t.Cleanup(func() { chrClient.Remove("/ip/route", routeID) })
}

// TestToolRoutedMultiVLANSubnets turns a blank router into a multi-subnet
// LAN router: two VLANs on the WAN interface, both wired into a bridge, each
// with its own routed subnet, address pool, DHCP network (with gateway) and
// DHCP server. WAN connectivity is proven by the connected routes and a ping
// to the upstream gateway.
func TestToolRoutedMultiVLANSubnets(t *testing.T) {
	requireRouter(t)

	// L2: bridge with two VLANs wired into it.
	_ = toolText(t, "bridge_add", "attributes", map[string]any{"name": "mcp-fab-br"})
	brID := waitForOne(t, "/interface/bridge", "name=mcp-fab-br")[".id"]
	t.Cleanup(func() { chrClient.Remove("/interface/bridge", brID) })

	for _, v := range []struct {
		name string
		id   int
	}{
		{"mcp-fab10", 10}, {"mcp-fab20", 20},
	} {
		_ = toolText(t, "vlan_add",
			"attributes", map[string]any{"name": v.name, "vlan-id": v.id, "interface": "ether1"})
		vlanID := waitForOne(t, "/interface/vlan", "name="+v.name)[".id"]
		t.Cleanup(func() { chrClient.Remove("/interface/vlan", vlanID) })

		_ = toolText(t, "bridge_port_add",
			"attributes", map[string]any{"bridge": "mcp-fab-br", "interface": v.name})
		portID := waitForOne(t, "/interface/bridge/port", "interface="+v.name)[".id"]
		t.Cleanup(func() { chrClient.Remove("/interface/bridge/port", portID) })
	}

	if text := toolText(t, "bridge_port_list", "bridge", "mcp-fab-br"); !strings.Contains(text, "mcp-fab10") || !strings.Contains(text, "mcp-fab20") {
		t.Errorf("bridge_port_list missing wired VLANs: %s", text)
	}

	// L3: per-subnet address, pool, DHCP network (with gateway) and DHCP server.
	for _, s := range []struct{ subnet, gw, iface, pool, dhcp, range_ string }{
		{"10.11.0.0/24", "10.11.0.1", "mcp-fab10", "mcp-fab10-pool", "mcp-fab-dhcp10", "10.11.0.100-10.11.0.200"},
		{"10.12.0.0/24", "10.12.0.1", "mcp-fab20", "mcp-fab20-pool", "mcp-fab-dhcp20", "10.12.0.100-10.12.0.200"},
	} {
		_ = toolText(t, "resource_add", "menu", "ip/address",
			"attributes", map[string]any{"address": s.gw + "/24", "interface": s.iface})
		addrID := waitForOne(t, "/ip/address", "address="+s.gw+"/24")[".id"]
		t.Cleanup(func() { chrClient.Remove("/ip/address", addrID) })

		_ = toolText(t, "resource_add", "menu", "ip/pool",
			"attributes", map[string]any{"name": s.pool, "ranges": s.range_})
		poolID := waitForOne(t, "/ip/pool", "name="+s.pool)[".id"]
		t.Cleanup(func() { chrClient.Remove("/ip/pool", poolID) })

		_ = toolText(t, "resource_add", "menu", "ip/dhcp-server/network",
			"attributes", map[string]any{"address": s.subnet, "gateway": s.gw, "dns-server": "1.1.1.1"})
		netID := waitForOne(t, "/ip/dhcp-server/network", "address="+s.subnet)[".id"]
		t.Cleanup(func() { chrClient.Remove("/ip/dhcp-server/network", netID) })

		_ = toolText(t, "resource_add", "menu", "ip/dhcp-server",
			"attributes", map[string]any{"name": s.dhcp, "interface": s.iface, "address-pool": s.pool})
		dhcpID := waitForOne(t, "/ip/dhcp-server", "name="+s.dhcp)[".id"]
		t.Cleanup(func() { chrClient.Remove("/ip/dhcp-server", dhcpID) })
	}

	if text := toolText(t, "dhcp_network_list"); !strings.Contains(text, "10.11.0.0/24") || !strings.Contains(text, "10.12.0.0/24") {
		t.Errorf("dhcp_network_list missing routed subnets: %s", text)
	}

	// Routing: both subnets are connected and the router can reach its WAN
	// gateway (the default route provided by the ether1 DHCP client).
	for _, subnet := range []string{"10.11.0.0/24", "10.12.0.0/24"} {
		waitForOne(t, "/ip/route", "dst-address="+subnet)
	}
	if text := toolText(t, "tool_ping", "address", "10.0.2.2", "count", 1); !strings.Contains(text, "Ping") {
		t.Errorf("tool_ping to WAN gateway: %s", text)
	}
}

// TestToolIPv6Setup builds a ULA network: global address on a VLAN, an
// address pool, a DHCPv6 server, and a static route via the VLAN gateway.
func TestToolIPv6Setup(t *testing.T) {
	requireRouter(t)

	_ = toolText(t, "vlan_add",
		"attributes", map[string]any{"name": "mcp-vlan6", "vlan-id": 6, "interface": "ether1"})
	vlanID := waitForOne(t, "/interface/vlan", "name=mcp-vlan6")[".id"]
	t.Cleanup(func() { chrClient.Remove("/interface/vlan", vlanID) })

	_ = toolText(t, "resource_add", "menu", "ipv6/address",
		"attributes", map[string]any{"address": "fd00:11::1/64", "interface": "mcp-vlan6"})
	addrID := waitForOne(t, "/ipv6/address", "address=fd00:11::1/64")[".id"]
	t.Cleanup(func() { chrClient.Remove("/ipv6/address", addrID) })

	_ = toolText(t, "resource_add", "menu", "ipv6/pool",
		"attributes", map[string]any{"name": "mcp6pool", "prefix": "fd00:11::/64", "prefix-length": 64})
	poolID := waitForOne(t, "/ipv6/pool", "name=mcp6pool")[".id"]
	t.Cleanup(func() { chrClient.Remove("/ipv6/pool", poolID) })

	_ = toolText(t, "resource_add", "menu", "ipv6/dhcp-server",
		"attributes", map[string]any{"name": "mcp6dhcp", "interface": "mcp-vlan6", "address-pool": "mcp6pool"})
	dhcpID := waitForOne(t, "/ipv6/dhcp-server", "name=mcp6dhcp")[".id"]
	t.Cleanup(func() { chrClient.Remove("/ipv6/dhcp-server", dhcpID) })

	_ = toolText(t, "resource_add", "menu", "ipv6/route",
		"attributes", map[string]any{"dst-address": "2001:db8:1::/48", "gateway": "fd00:11::2"})
	route := waitForOne(t, "/ipv6/route", "dst-address=2001:db8:1::/48")
	if route["static"] != "true" {
		t.Errorf("expected static ipv6 route, got %v", route)
	}
	t.Cleanup(func() { chrClient.Remove("/ipv6/route", route[".id"]) })

	// The ULA subnet must be directly connected via the VLAN.
	waitForOne(t, "/ipv6/route", "dst-address=fd00:11::/64")
}

// TestToolPPPoEServerSetup builds a PPPoE server for an ISP-style access
// concentrator: address pool, PPP profile (local + remote addresses), the
// server itself on the WAN interface, and a client secret.
func TestToolPPPoEServerSetup(t *testing.T) {
	requireRouter(t)

	_ = toolText(t, "resource_add", "menu", "ip/pool",
		"attributes", map[string]any{"name": "mcp-ppp-pool", "ranges": "10.88.0.2-10.88.0.100"})
	poolID := waitForOne(t, "/ip/pool", "name=mcp-ppp-pool")[".id"]
	t.Cleanup(func() { chrClient.Remove("/ip/pool", poolID) })

	_ = toolText(t, "resource_add", "menu", "ppp/profile",
		"attributes", map[string]any{"name": "mcp-ppp", "local-address": "10.88.0.1", "remote-address": "mcp-ppp-pool"})
	profID := waitForOne(t, "/ppp/profile", "name=mcp-ppp")[".id"]
	t.Cleanup(func() { chrClient.Remove("/ppp/profile", profID) })

	_ = toolText(t, "resource_add", "menu", "interface/pppoe-server/server",
		"attributes", map[string]any{"service-name": "mcp-pppoe", "interface": "ether1", "default-profile": "mcp-ppp", "disabled": false})
	server := waitForOne(t, "/interface/pppoe-server/server", "service-name=mcp-pppoe")
	if server["disabled"] != "false" {
		t.Errorf("expected pppoe server enabled, got %v", server)
	}
	t.Cleanup(func() { chrClient.Remove("/interface/pppoe-server/server", server[".id"]) })

	_ = toolText(t, "ppp_secret_add", "attributes",
		map[string]any{"name": "mcp-ppp-user", "password": "mcp-ppp-pw", "service": "pppoe", "profile": "mcp-ppp"})
	secret := waitForOne(t, "/ppp/secret", "name=mcp-ppp-user")
	t.Cleanup(func() { chrClient.Remove("/ppp/secret", secret[".id"]) })

	if text := toolText(t, "ppp_secret_list"); !strings.Contains(text, "mcp-ppp-user") {
		t.Errorf("ppp_secret_list missing pppoe user: %s", text)
	}
}
