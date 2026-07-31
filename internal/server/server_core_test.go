package server

import (
	"strings"
	"testing"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
)

func TestIntegrationSystemIdentityGet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=my-router"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerSystemIdentityGet(cl)(ctx(), testutil.MkReq("system_identity_get"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertStructuredContent(t, result)
	if !strings.Contains(resultText(result), "my-router") {
		t.Error("missing router name")
	}
}

func TestIntegrationSystemResourceGet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=version=7.17", "=uptime=1d2h", "=board-name=RB5009"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerSystemResourceGet(cl)(ctx(), testutil.MkReq("system_resource_get"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertStructuredContent(t, result)
	text := resultText(result)
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

	result, err := handlerSystemClockGet(cl)(ctx(), testutil.MkReq("system_clock_get"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertStructuredContent(t, result)
	text := resultText(result)
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

	result, err := handlerInterfaceList(cl)(ctx(), testutil.MkReq("interface_list"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertStructuredContent(t, result)
	text := resultText(result)
	if !strings.Contains(text, "ether1") || !strings.Contains(text, "ether2") {
		t.Errorf("missing interfaces: %s", text)
	}
}

func TestIntegrationInterfaceGetSuccess(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=.id=*1", "=name=ether1", "=running=true"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerInterfaceGet(cl)(ctx(), testutil.MkReq("interface_get", "name", "ether1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertStructuredContent(t, result)
	if !strings.Contains(resultText(result), "ether1") {
		t.Error("missing interface name")
	}
}

func TestIntegrationInterfaceGetNoLocator(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	_, err := handlerInterfaceGet(cl)(ctx(), testutil.MkReq("interface_get"))
	if err == nil {
		t.Fatal("expected error for missing locator")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error message = %q", err.Error())
	}
}

func TestIntegrationIPAddressList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!re", "=address=192.168.1.1/24", "=interface=bridge", "=network=192.168.1.0"),
		enc("!done"),
	)
	cl.SetConn(fc)

	result, err := handlerIPAddressList(cl)(ctx(), testutil.MkReq("ip_address_list"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertStructuredContent(t, result)
	text := resultText(result)
	if !strings.Contains(text, "192.168.1.1/24") {
		t.Errorf("missing address: %s", text)
	}
}

func TestIntegrationDHCPLeaseList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!re", "=address=192.168.1.100", "=mac-address=00:11:22:33:44:55", "=host-name=client1", "=status=bound"),
		enc("!done"),
	)
	cl.SetConn(fc)

	result, err := handlerDHCPLeaseList(cl)(ctx(), testutil.MkReq("dhcp_lease_list"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertStructuredContent(t, result)
}

func TestIntegrationDHCPLeaseListActiveOnly(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!re", "=address=192.168.1.100", "=status=bound"),
		enc("!done"),
	)
	cl.SetConn(fc)

	result, err := handlerDHCPLeaseList(cl)(ctx(), testutil.MkReq("dhcp_lease_list", "active_only", true))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertSent(t, fc, "status=bound")
}

func TestIntegrationDNSGet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=servers=1.1.1.1,8.8.8.8", "=allow-remote-requests=true"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerDNSGet(cl)(ctx(), testutil.MkReq("dns_get"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
	assertStructuredContent(t, result)
	if !strings.Contains(resultText(result), "1.1.1.1") {
		t.Errorf("missing DNS servers: %s", resultText(result))
	}
}

func TestIntegrationDNSSet(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	result, err := handlerDNSSet(cl)(ctx(), testutil.MkReq("dns_set", "servers", []any{"8.8.8.8", "1.1.1.1"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", resultText(result))
	}
}

func TestIntegrationDNSSetNormalizesAttributes(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	// Whitespace-padded servers should be cleaned and joined with commas
	result, err := handlerDNSSet(cl)(ctx(), testutil.MkReq("dns_set", "servers", []any{"1.1.1.1", " 8.8.8.8 "}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", resultText(result))
	}
	assertSent(t, fc, "=servers=1.1.1.1,8.8.8.8")
}

func TestIntegrationDNSSetRequiresAtLeastOne(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	result, err := handlerDNSSet(cl)(ctx(), testutil.MkReq("dns_set"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for empty dns_set")
	}
}

func TestIntegrationDNSGetEmptyServers(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=servers="), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerDNSGet(cl)(ctx(), testutil.MkReq("dns_get"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Errorf("empty servers should not be an error")
	}
}

func TestIntegrationIPRouteGetRequiresExactlyOneLocator(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	_, err := handlerIPRouteGet(cl)(ctx(), testutil.MkReq("ip_route_get"))
	if err == nil {
		t.Fatal("expected error for no locator")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestIntegrationIPAddressGetRequiresExactlyOneLocator(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	_, err := handlerIPAddressGet(cl)(ctx(), testutil.MkReq("ip_address_get"))
	if err == nil {
		t.Fatal("expected error for no locator")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestIntegrationSystemIdentityGetNoMatch(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	_, err := handlerSystemIdentityGet(cl)(ctx(), testutil.MkReq("system_identity_get"))
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

	_, err := handlerSystemClockGet(cl)(ctx(), testutil.MkReq("system_clock_get"))
	if err == nil {
		t.Fatal("expected error for multiple matching records")
	}
	if !strings.Contains(err.Error(), "multiple system clock records matched") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestIntegrationSystemBackupSave(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	result, err := backupSaveHandler(cl)(ctx(), testutil.MkReq("system_backup_save", "name", "test-backup"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
}

func TestIntegrationSystemBackupSaveShape(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	result, err := backupSaveHandler(cl)(ctx(), testutil.MkReq("system_backup_save", "name", "nightly"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !strings.Contains(text, "success") || !strings.Contains(text, "nightly") {
		t.Errorf("backup save result missing expected keys: %s", text)
	}
}

func TestIntegrationSystemBackupSaveRequiresName(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	_, err := backupSaveHandler(cl)(ctx(), testutil.MkReq("system_backup_save"))
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestIntegrationSystemExportShape(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	result, err := exportHandler(cl)(ctx(), testutil.MkReq("system_export", "name", "config", "include_sensitive", true, "compact", true))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !strings.Contains(text, "success") || !strings.Contains(text, "config.rsc") {
		t.Errorf("export result missing expected keys: %s", text)
	}
}

func TestIntegrationSystemExportRejectsTrailingSlash(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	_, err := exportHandler(cl)(ctx(), testutil.MkReq("system_export", "name", "/"))
	if err == nil {
		t.Fatal("expected error for trailing slash")
	}
	if !strings.Contains(err.Error(), "must not end with") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestIntegrationSystemExportWithFlags(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	result, err := exportHandler(cl)(ctx(),
		testutil.MkReq("system_export", "name", "config-export", "include_sensitive", true, "compact", true))
	if err != nil {
		t.Fatalf("export error: %v", err)
	}
	assertResult(t, result)
	assertSent(t, fc, "show-sensitive")
	assertSent(t, fc, "compact")
}

func TestIntegrationResourceAddRequiresMenu(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing menu")
		}
	}()
	handlerResourceAdd(cl)(ctx(), testutil.MkReq("resource_add"))
}
