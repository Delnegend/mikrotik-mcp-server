package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pheoxy/mikrotik-mcp/internal/client"
	"github.com/pheoxy/mikrotik-mcp/internal/downloads"
)

type fakeConn struct {
	response []byte
	pos      int
	sent     []byte
	closed   bool
}

func newFakeConn(responses ...[]byte) *fakeConn {
	var resp []byte
	for _, r := range responses {
		resp = append(resp, r...)
	}
	return &fakeConn{response: resp}
}

func (f *fakeConn) Read(b []byte) (int, error) {
	if f.pos >= len(f.response) {
		return 0, nil
	}
	n := copy(b, f.response[f.pos:])
	f.pos += n
	return n, nil
}
func (f *fakeConn) Write(b []byte) (int, error) {
	f.sent = append(f.sent, b...)
	return len(b), nil
}
func (f *fakeConn) Close() error                  { f.closed = true; return nil }
func (f *fakeConn) LocalAddr() net.Addr           { return nil }
func (f *fakeConn) RemoteAddr() net.Addr          { return nil }
func (f *fakeConn) SetDeadline(t time.Time) error { return nil }
func (f *fakeConn) SetReadDeadline(t time.Time) error { return nil }
func (f *fakeConn) SetWriteDeadline(t time.Time) error { return nil }

func enc(words ...string) []byte {
	var buf []byte
	for _, w := range words {
		b := []byte(w)
		buf = append(buf, encLen(len(b))...)
		buf = append(buf, b...)
	}
	buf = append(buf, 0)
	return buf
}

func encLen(length int) []byte {
	if length < 0x80 {
		return []byte{byte(length)}
	}
	v := length | 0x8000
	return []byte{byte(v >> 8), byte(v)}
}

var _ net.Conn = (*fakeConn)(nil)

func TestIntegrationSystemIdentityGet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=my-router"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerSystemIdentityGet(cl)(context.Background(), mkReq("system_identity_get"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(result.Content[0].(mcp.TextContent).Text, "my-router") {
		t.Error("missing router name")
	}
}

func TestIntegrationSystemResourceGet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=version=7.17", "=uptime=1d2h", "=board-name=RB5009"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerSystemResourceGet(cl)(context.Background(), mkReq("system_resource_get"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "RouterOS 7.17, uptime 1d2h") {
		t.Errorf("missing resource summary: %s", text)
	}
	if !strings.Contains(text, "| board-name | RB5009 |") {
		t.Errorf("missing board-name row: %s", text)
	}
}

func TestIntegrationSystemClockGet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=date=Jul/25/2026", "=time=10:30:00"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerSystemClockGet(cl)(context.Background(), mkReq("system_clock_get"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Jul/25/2026") || !strings.Contains(text, "10:30:00") {
		t.Errorf("missing clock data: %s", text)
	}
}

func TestIntegrationInterfaceList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!re", "=.id=*1", "=name=ether1", "=running=true", "=type=ether"),
		enc("!re", "=.id=*2", "=name=ether2", "=running=false", "=type=ether"),
		enc("!done"),
	)
	cl.SetConn(fc)

	result, err := handlerInterfaceList(cl)(context.Background(), mkReq("interface_list"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "ether1") || !strings.Contains(text, "ether2") {
		t.Errorf("missing interfaces: %s", text)
	}
}

func TestIntegrationResourceAdd(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done", "=ret=*42"))
	cl.SetConn(fc)

	result, err := handlerResourceAdd(cl)(context.Background(), mkReq("bridge_add", "menu", "/interface/bridge", "name", "br-test"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(string(fc.sent), "/interface/bridge/add") {
		t.Errorf("missing add path")
	}
}

func TestIntegrationResourceSet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	result, err := handlerResourceSet(cl)(context.Background(), mkReq("resource_set", "menu", "/ip/address", "item_id", "*4", "disabled", true))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(string(fc.sent), "/ip/address/set") {
		t.Errorf("missing set path")
	}
	if !strings.Contains(string(fc.sent), "=.id=*4") {
		t.Errorf("missing .id")
	}
}

func TestIntegrationResourceRemove(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!empty"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerResourceRemove(cl)(context.Background(), mkReq("resource_remove", "menu", "/ip/address", "item_id", "*5"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(string(fc.sent), "/ip/address/remove") {
		t.Errorf("missing remove path")
	}
}

func TestIntegrationDNSGet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=servers=1.1.1.1,8.8.8.8", "=allow-remote-requests=true"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerDNSGet(cl)(context.Background(), mkReq("dns_get"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "1.1.1.1") {
		t.Errorf("missing DNS servers: %s", text)
	}
}

func TestIntegrationDNSSet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	result, err := handlerDNSSet(cl)(context.Background(), mkReq("dns_set", "servers", []any{"8.8.8.8", "1.1.1.1"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestIntegrationIPAddressList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!re", "=address=192.168.1.1/24", "=interface=bridge", "=network=192.168.1.0"),
		enc("!done"),
	)
	cl.SetConn(fc)

	result, err := handlerIPAddressList(cl)(context.Background(), mkReq("ip_address_list"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "192.168.1.1/24") {
		t.Errorf("missing address: %s", text)
	}
}

func TestIntegrationBridgeRemove(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	_, err := removeHandler(cl, "/interface/bridge")(context.Background(), mkReq("bridge_remove", "item_id", "*7"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !strings.Contains(string(fc.sent), "/interface/bridge/remove") {
		t.Errorf("missing remove path")
	}
	if !strings.Contains(string(fc.sent), "=.id=*7") {
		t.Errorf("missing .id")
	}
}

func TestIntegrationInterfaceGetSuccess(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=.id=*1", "=name=ether1", "=running=true"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerInterfaceGet(cl)(context.Background(), mkReq("interface_get", "name", "ether1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	if !strings.Contains(result.Content[0].(mcp.TextContent).Text, "ether1") {
		t.Error("missing interface name")
	}
}

func TestIntegrationInterfaceGetNoLocator(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	_, err := handlerInterfaceGet(cl)(context.Background(), mkReq("interface_get"))
	if err == nil {
		t.Fatal("expected error for missing locator")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error message = %q", err.Error())
	}
}

func TestIntegrationSystemBackupSave(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	result, err := backupSaveHandler(cl)(context.Background(), mkReq("system_backup_save", "name", "test-backup"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestIntegrationDHCPLeaseList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!re", "=address=192.168.1.100", "=mac-address=00:11:22:33:44:55", "=host-name=client1", "=status=bound"),
		enc("!done"),
	)
	cl.SetConn(fc)

	result, err := handlerDHCPLeaseList(cl)(context.Background(), mkReq("dhcp_lease_list"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func TestIntegrationDHCPLeaseListActiveOnly(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!re", "=address=192.168.1.100", "=status=bound"),
		enc("!done"),
	)
	cl.SetConn(fc)

	result, err := handlerDHCPLeaseList(cl)(context.Background(), mkReq("dhcp_lease_list", "active_only", true))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content[0].(mcp.TextContent).Text)
	}
	sent := string(fc.sent)
	if !strings.Contains(sent, "status=bound") {
		t.Errorf("expected status=bound query, got: %s", sent)
	}
}

func TestIntegrationDNSGetEmptyServers(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=servers="), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerDNSGet(cl)(context.Background(), mkReq("dns_get"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Errorf("empty servers should not be an error")
	}
}

func TestIntegrationResourceAddRequiresMenu(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing menu")
		}
	}()
	handlerResourceAdd(cl)(context.Background(), mkReq("resource_add"))
}

func TestIntegrationHealthcheckAllHealthy(t *testing.T) {
	origLoad := hcLoadFileTransferSettings
	origDownloader := hcNewSCPFileDownloader
	origProbe := hcProbeSSHFingerprint
	origCheck := hcCheckPasswordRotationReady
	origResolve := hcResolveSCPPrivateKeyPath
	defer func() {
		hcLoadFileTransferSettings = origLoad
		hcNewSCPFileDownloader = origDownloader
		hcProbeSSHFingerprint = origProbe
		hcCheckPasswordRotationReady = origCheck
		hcResolveSCPPrivateKeyPath = origResolve
	}()

	// Set env vars
	origUser := os.Getenv("MIKROTIK_USER")
	origPass := os.Getenv("MIKROTIK_PASSWORD")
	origScpUser := os.Getenv("MIKROTIK_SCP_USER")
	origFingerprint := os.Getenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256")
	origScpHost := os.Getenv("MIKROTIK_SCP_HOST")
	defer func() {
		os.Setenv("MIKROTIK_USER", origUser)
		os.Setenv("MIKROTIK_PASSWORD", origPass)
		os.Setenv("MIKROTIK_SCP_USER", origScpUser)
		os.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", origFingerprint)
		os.Setenv("MIKROTIK_SCP_HOST", origScpHost)
	}()
	os.Setenv("MIKROTIK_USER", "api-user")
	os.Setenv("MIKROTIK_PASSWORD", "api-pass")
	os.Setenv("MIKROTIK_SCP_USER", "scp-user")
	os.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", "SHA256:test-host-key")
	os.Setenv("MIKROTIK_SCP_HOST", "files.router.test")

	// Set up a mock client with identity response
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=lab-router"), enc("!done"))
	cl.SetConn(fc)

	// Swap dependencies with mocks
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: "files.router.test", Port: 21}, nil
	}

	hcNewSCPFileDownloader = func(s *downloads.FileTransferSettings) scpChecker {
		return &mockSCPDownloader{checkResult: map[string]any{
			"working_directory": "/",
			"listing_count":     2,
			"listing_sample":    []any{"backups", "flash"},
			"operation":         "normalize+listdir_attr",
		}}
	}

	hcProbeSSHFingerprint = func(host string, port int, timeout time.Duration) (map[string]any, error) {
		return map[string]any{
			"host":               host,
			"port":               port,
			"key_type":           "ssh-ed25519",
			"fingerprint_sha256": "SHA256:server-fingerprint",
		}, nil
	}

	hcCheckPasswordRotationReady = func(host, username string) (map[string]any, error) {
		return map[string]any{
			"host":          host,
			"port":          22,
			"username":      username,
			"target_exists": true,
		}, nil
	}

	hcResolveSCPPrivateKeyPath = func() (string, error) { return "", nil }

	result, err := handlerHealthcheck(cl)(context.Background(), mkReq("healthcheck"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler error: %s", result.Content[0].(mcp.TextContent).Text)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Healthcheck: healthy") {
		t.Errorf("expected healthy, got: %s", text)
	}
	if !strings.Contains(text, "name:lab-router") {
		t.Errorf("missing identity in: %s", text)
	}
	if !strings.Contains(text, "scp.ok") {
		t.Errorf("missing scp code in: %s", text)
	}
	if !strings.Contains(text, "passwordless.disabled") {
		t.Errorf("missing passwordless in: %s", text)
	}
}

// mockSCPDownloader implements scpChecker for testing (defined in tool_core.go).
type mockSCPDownloader struct {
	checkResult map[string]any
	checkErr    error
}

func (m *mockSCPDownloader) CheckConnection() (map[string]any, error) {
	if m.checkErr != nil {
		return nil, m.checkErr
	}
	return m.checkResult, nil
}

func (m *mockSCPDownloader) DownloadFile(routerPath, localPath string) error {
	return nil
}

func (m *mockSCPDownloader) wrap() *downloads.SCPFileDownloader {
	// We return nil since the handler calls CheckConnection on the result,
	// but we use the mock interface pattern through our variable. This is safe
	// because our mock only needs to satisfy the downloader interface.
	return nil
}

// ============================================================
// Input validation tests (table-driven)
// ============================================================

func TestIntegrationPingValidatesInputs(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{"blank address", map[string]any{"address": "   "}, "address is required"},
		{"count zero", map[string]any{"address": "1.1.1.1", "count": float64(0)}, "count must be at least 1"},
		{"blank interval", map[string]any{"address": "1.1.1.1", "interval": "   "}, "interval is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			cl := client.NewRouterOSClient("router.test", "admin", "secret")
			_, err := handlerToolPing(cl)(context.Background(), mkReq("tool_ping", mapToArgs(tt.args)...))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestIntegrationTracerouteValidatesInputs(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{"blank address", map[string]any{"address": "   "}, "address is required"},
		{"count zero", map[string]any{"address": "1.1.1.1", "count": float64(0)}, "count must be at least 1"},
		{"max_hops zero", map[string]any{"address": "1.1.1.1", "max_hops": float64(0)}, "max_hops must be at least 1"},
		{"packet_size zero", map[string]any{"address": "1.1.1.1", "packet_size": float64(0)}, "packet_size must be at least 1"},
		{"blank interval", map[string]any{"address": "1.1.1.1", "interval": "   "}, "interval is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			cl := client.NewRouterOSClient("router.test", "admin", "secret")
			_, err := handlerToolTraceroute(cl)(context.Background(), mkReq("tool_traceroute", mapToArgs(tt.args)...))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// ============================================================
// Healthcheck error-state tests
// ============================================================

func TestIntegrationHealthcheckAPIOkSCPFailed(t *testing.T) {
	origLoad := hcLoadFileTransferSettings
	origDownloader := hcNewSCPFileDownloader
	origProbe := hcProbeSSHFingerprint
	origCheck := hcCheckPasswordRotationReady
	origResolve := hcResolveSCPPrivateKeyPath
	defer func() {
		hcLoadFileTransferSettings = origLoad
		hcNewSCPFileDownloader = origDownloader
		hcProbeSSHFingerprint = origProbe
		hcCheckPasswordRotationReady = origCheck
		hcResolveSCPPrivateKeyPath = origResolve
	}()

	saveAndSetEnv(t, "MIKROTIK_USER", "api-user")
	saveAndSetEnv(t, "MIKROTIK_PASSWORD", "api-pass")
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: "files.test", Port: 21}, nil
	}
	hcNewSCPFileDownloader = func(s *downloads.FileTransferSettings) scpChecker {
		return &mockSCPDownloader{checkErr: fmt.Errorf("scp unavailable")}
	}
	hcProbeSSHFingerprint = func(host string, port int, timeout time.Duration) (map[string]any, error) {
		return map[string]any{"status": "ok"}, nil
	}
	hcResolveSCPPrivateKeyPath = func() (string, error) { return "", nil }

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=lab-router"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerHealthcheck(cl)(context.Background(), mkReq("healthcheck"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Healthcheck: degraded") {
		t.Errorf("expected degraded, got: %s", text)
	}
	_ = result
}

func TestIntegrationHealthcheckAPIFailed(t *testing.T) {
	origLoad := hcLoadFileTransferSettings
	origProbe := hcProbeSSHFingerprint
	origResolve := hcResolveSCPPrivateKeyPath
	defer func() {
		hcLoadFileTransferSettings = origLoad
		hcProbeSSHFingerprint = origProbe
		hcResolveSCPPrivateKeyPath = origResolve
	}()

	saveAndSetEnv(t, "MIKROTIK_USER", "api-user")
	saveAndSetEnv(t, "MIKROTIK_PASSWORD", "api-pass")
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return nil, fmt.Errorf("SCP config missing")
	}
	hcProbeSSHFingerprint = func(host string, port int, timeout time.Duration) (map[string]any, error) {
		return map[string]any{"status": "ok"}, nil
	}
	hcResolveSCPPrivateKeyPath = func() (string, error) { return "", nil }

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	// Return a trap to make API fail
	fc := newFakeConn(enc("!trap", "=message=api unavailable"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerHealthcheck(cl)(context.Background(), mkReq("healthcheck"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	// Both API and SCP failed → overall status is "failed"
	if !strings.Contains(text, "Healthcheck: failed") {
		t.Errorf("expected failed, got: %s", text)
	}
	_ = result
}

// DNS resolve handlers use Isolated() which can't be tested via fakeConn.
// The validation logic is tested via code review and manual testing.

// ============================================================
// Single-record error tests
// ============================================================

func TestIntegrationSystemIdentityGetNoMatch(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	_, err := handlerSystemIdentityGet(cl)(context.Background(), mkReq("system_identity_get"))
	if err == nil {
		t.Fatal("expected error for no matching record")
	}
	if !strings.Contains(err.Error(), "no matching system identity found") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestIntegrationSystemClockGetMultipleMatches(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!re", "=date=Jan/01/2026", "=time=10:00"),
		enc("!re", "=date=Feb/01/2026", "=time=11:00"),
		enc("!done"),
	)
	cl.SetConn(fc)

	_, err := handlerSystemClockGet(cl)(context.Background(), mkReq("system_clock_get"))
	if err == nil {
		t.Fatal("expected error for multiple matching records")
	}
	if !strings.Contains(err.Error(), "multiple system clock records matched") {
		t.Errorf("error = %q", err.Error())
	}
}

// ============================================================
// Helper updates
// ============================================================

// mapToArgs converts a map to alternating key/value arguments for mkReq
func mapToArgs(m map[string]any) []any {
	var args []any
	for k, v := range m {
		args = append(args, k, v)
	}
	return args
}

// saveAndSetEnv saves an env var and sets a new value, returning a func to restore
func saveAndSetEnv(t *testing.T, key, value string) {
	t.Helper()
	orig := os.Getenv(key)
	os.Setenv(key, value)
	t.Cleanup(func() { os.Setenv(key, orig) })
}

// ============================================================
// File operation tests
// ============================================================

// TestIntegrationBackupCollectExportFailure tests that when the export
// step fails during backup collect, the error is propagated correctly.
func TestIntegrationBackupCollectExportFailure(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	// First two responses: files list shows backups dir exists
	// Then backup save succeeds, then export fails with trap
	fc := newFakeConn(
		enc("!done"),                           // backup save succeeds
		enc("!trap", "=message=disk full"),     // export fails
		enc("!done"),                           // done for export
	)
	cl.SetConn(fc)

	_, err := backupCollectHandler(cl)(context.Background(),
		mkReq("system_backup_collect", "name_prefix", "nightly", "include_sensitive", true, "compact", true))
	if err == nil {
		t.Fatal("expected export failure error")
	}
	if !strings.Contains(err.Error(), "export") {
		t.Errorf("error should mention export: %v", err)
	}
}

func TestIntegrationSystemExportWithFlags(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	result, err := exportHandler(cl)(context.Background(),
		mkReq("system_export", "name", "config-export", "include_sensitive", true, "compact", true))
	if err != nil {
		t.Fatalf("export error: %v", err)
	}
	if result.IsError {
		t.Errorf("export failed: %s", result.Content[0].(mcp.TextContent).Text)
	}
	sent := string(fc.sent)
	if !strings.Contains(sent, "show-sensitive") {
		t.Errorf("missing show-sensitive flag: %s", sent)
	}
	if !strings.Contains(sent, "compact") {
		t.Errorf("missing compact flag: %s", sent)
	}
}

// --- Helpers ---
func mkReq(name string, args ...any) mcp.CallToolRequest {
	argMap := make(map[string]any)
	for i := 0; i+1 < len(args); i += 2 {
		if key, ok := args[i].(string); ok {
			argMap[key] = args[i+1]
		}
	}
	return mcp.CallToolRequest{
		Params: struct {
			Name      string         "json:\"name\""
			Arguments map[string]any "json:\"arguments,omitempty\""
			Meta      *struct {
				ProgressToken mcp.ProgressToken "json:\"progressToken,omitempty\""
			} "json:\"_meta,omitempty\""
		}{Name: name, Arguments: argMap},
	}
}
