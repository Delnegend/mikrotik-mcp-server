package server

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
)

// Ping, traceroute, DNS resolve, and interface monitor use Isolated().
// These tests inject client.NetDial to return the fake conn.

func TestIntegrationResourceListenPayload(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!done"), enc("!re", ".tag=listen-1", "=name=ether1"),
		enc("!done"), enc("!done", ".tag=listen-1", "=ret=interrupted"),
	)
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	result, err := handlerResourceListen(cl)(ctx(), testutil.MkReq("resource_listen", "menu", "/interface", "max_events", float64(1), "tag", "listen-1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !containsAll(text, "tag=listen-1", "events=1") {
		t.Errorf("unexpected result: %s", text)
	}
}

func TestIntegrationCommandRun(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done", "=ret=ok"))
	cl.SetConn(fc)
	_, err := handlerCommandRun(cl)(ctx(), testutil.MkReq("command_run", "command", "/system/backup/save", "attributes", map[string]any{"name": "test"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/system/backup/save", "=name=test"})
}

func TestIntegrationCommandCancel(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := handlerCommandCancel(cl)(ctx(), testutil.MkReq("command_cancel", "tag", "test-tag"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSentExact(t, fc, []string{"/cancel", "=tag=test-tag"})
}

func TestIntegrationToolPingReturnsRecords(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!re", "=seq=1", "=host=10.0.0.1", "=time=5ms", "=status=reachable"), enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	result, err := handlerToolPing(cl)(ctx(), testutil.MkReq("tool_ping", "address", "10.0.0.1", "count", float64(1)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertStructuredContent(t, result)
	if !containsAll(resultText(result), "10.0.0.1") {
		t.Errorf("missing ping target: %s", resultText(result))
	}
}

func TestIntegrationToolPingDoneOnly(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	result, err := handlerToolPing(cl)(ctx(), testutil.MkReq("tool_ping", "address", "10.0.0.1", "count", float64(1)))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !containsAll(resultText(result), "0 probes") {
		t.Errorf("expected '0 probes': %s", resultText(result))
	}
}

func TestIntegrationToolPingPropagatesErrors(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!trap", "=message=network unreachable"), enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	_, err := handlerToolPing(cl)(ctx(), testutil.MkReq("tool_ping", "address", "10.0.0.1"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "network unreachable") {
		t.Errorf("error should propagate router trap: %v", err)
	}
}

func TestIntegrationToolTracerouteReturnsRecords(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!re", "=hop=1", "=host=10.0.0.1", "=status=reachable"), enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	result, err := handlerToolTraceroute(cl)(ctx(), testutil.MkReq("tool_traceroute", "address", "10.0.0.1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !containsAll(resultText(result), "10.0.0.1") {
		t.Errorf("missing traceroute target: %s", resultText(result))
	}
}

func TestIntegrationToolTracerouteDoneOnly(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	_, err := handlerToolTraceroute(cl)(ctx(), testutil.MkReq("tool_traceroute", "address", "10.0.0.1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
}

func TestIntegrationToolTraceroutePropagatesErrors(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!trap", "=message=no route to host"), enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	_, err := handlerToolTraceroute(cl)(ctx(), testutil.MkReq("tool_traceroute", "address", "10.0.0.1"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no route to host") {
		t.Errorf("error should propagate router trap: %v", err)
	}
}

func TestIntegrationDNSResolveReturnsResult(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!done", "=ret=10.0.0.1"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	result, err := handlerDNSResolve(cl)(ctx(), testutil.MkReq("dns_resolve", "name", "test.example.com", "server", "8.8.8.8"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertStructuredContent(t, result)
	if !containsAll(resultText(result), "10.0.0.1") {
		t.Errorf("missing resolved address: %s", resultText(result))
	}
}

func TestIntegrationDNSResolveRequiresName(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	_, err := handlerDNSResolve(cl)(ctx(), testutil.MkReq("dns_resolve"))
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestIntegrationDNSResolvePropagatesErrors(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!trap", "=message=dns server failure"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	_, err := handlerDNSResolve(cl)(ctx(), testutil.MkReq("dns_resolve", "name", "test.example.com"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIntegrationDNSResolveRequiresAddress(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!done", "=ret="))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	result, err := handlerDNSResolve(cl)(ctx(), testutil.MkReq("dns_resolve", "name", "test.example.com"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError when resolve returns empty address")
	}
	if !strings.Contains(resultText(result), "did not return an address") {
		t.Errorf("unexpected error message: %s", resultText(result))
	}
}

func TestIntegrationInterfaceMonitorReturnsFirstRecord(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!re", "=name=ether1", "=rx-bits-per-second=1000000", "=tx-bits-per-second=500000"), enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	result, err := handlerInterfaceMonitor(cl)(ctx(), testutil.MkReq("interface_monitor", "name", "ether1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertStructuredContent(t, result)
	if !containsAll(resultText(result), "1000000") {
		t.Errorf("missing rx rate: %s", resultText(result))
	}
}

func TestIntegrationInterfaceMonitorRequiresName(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	_, err := handlerInterfaceMonitor(cl)(ctx(), testutil.MkReq("interface_monitor"))
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestIntegrationInterfaceMonitorNoResult(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	result, err := handlerInterfaceMonitor(cl)(ctx(), testutil.MkReq("interface_monitor", "name", "nonexistent"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsAll(resultText(result), "nonexistent") {
		t.Errorf("expected name in output: %s", resultText(result))
	}
}

func TestIntegrationInterfaceMonitorAcceptsSingleDict(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	// !done with attrs → normalizeMutationResult returns a single map
	fc := newFakeConn(enc("!done"), enc("!done", "=rx-bits-per-second=42"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	result, err := handlerInterfaceMonitor(cl)(ctx(), testutil.MkReq("interface_monitor", "name", "ether1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsAll(resultText(result), "ether1") {
		t.Errorf("expected name in output: %s", resultText(result))
	}
}

func TestIntegrationInterfaceMonitorPropagatesErrors(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"), enc("!trap", "=message=interface not found"), enc("!done"))
	origDial := client.NetDial
	client.NetDial = fc.Dialer()
	defer func() { client.NetDial = origDial }()

	_, err := handlerInterfaceMonitor(cl)(ctx(), testutil.MkReq("interface_monitor", "name", "nonexistent"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "interface not found") {
		t.Errorf("error should propagate router trap: %v", err)
	}
}

// TestConcurrentToolCallsDoNotInterleave verifies execMu serializes
// concurrent handler calls on a shared client: 5 goroutines produce
// 5 complete, non-interleaved sentences on the fake conn.
func TestConcurrentToolCallsDoNotInterleave(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn()
	for i := 0; i < 5; i++ {
		fc.WriteResponse(enc("!done"))
	}
	cl.SetConn(fc)

	commands := []string{"/cmd/a", "/cmd/b", "/cmd/c", "/cmd/d", "/cmd/e"}
	var wg sync.WaitGroup
	for _, cmd := range commands {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			_, err := handlerCommandRun(cl)(ctx(), testutil.MkReq("command_run", "command", c))
			if err != nil {
				t.Errorf("handler error for %s: %v", c, err)
			}
		}(cmd)
	}
	wg.Wait()

	sentences, err := decodeSentences(fc.Sent())
	if err != nil {
		t.Fatalf("decode sent data: %v", err)
	}
	if len(sentences) != 5 {
		t.Fatalf("got %d sentences, want 5 (interleaving detected)", len(sentences))
	}
	found := make(map[string]bool)
	for _, s := range sentences {
		if len(s) == 0 || !foundSentWord(s[0], commands) {
			t.Errorf("sentence has unexpected first word: %v", s)
			continue
		}
		if found[s[0]] {
			t.Errorf("command %s appeared more than once", s[0])
		}
		found[s[0]] = true
	}
	for _, cmd := range commands {
		if !found[cmd] {
			t.Errorf("missing sentence for %s", cmd)
		}
	}
}

func foundSentWord(w string, commands []string) bool {
	for _, c := range commands {
		if w == c {
			return true
		}
	}
	return false
}

// TestContextCancellationAbortsCall verifies a cancelled context propagates
// context.Canceled before the handler executes.
func TestContextCancellationAbortsCall(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	cancelledCtx, cancel := context.WithCancel(ctx())
	cancel()

	// Handlers are wrapped with recoverHandler at registration; mirror that here.
	_, err := recoverHandler(handlerCommandRun(cl))(cancelledCtx, testutil.MkReq("command_run", "command", "/cmd/x"))
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error should be context.Canceled, got: %v", err)
	}
	if len(fc.Sent()) != 0 {
		t.Errorf("no sentence should be sent for cancelled context, got: %q", string(fc.Sent()))
	}
}
