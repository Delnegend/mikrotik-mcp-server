package formatting

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func assertStructuredContent(t *testing.T, result *mcp.CallToolResult, want any) {
	t.Helper()
	if result.Meta == nil {
		t.Fatal("missing result.Meta")
	}
	sc, ok := result.Meta.AdditionalFields["structuredContent"]
	if !ok {
		t.Fatal("missing structuredContent in result.Meta")
	}
	if !reflect.DeepEqual(sc, want) {
		t.Errorf("structuredContent = %v, want %v", sc, want)
	}
}

func TestCallToolResultFromRecordWithSystemResource(t *testing.T) {
	record := map[string]any{
		"version":    "7.17",
		"uptime":     "1d2h",
		"board-name": "RB5009",
	}

	result, err := CallToolResultFromRecord(
		"System Resource",
		"System resource: RouterOS 7.17, uptime 1d2h",
		record,
		[]string{"platform", "board-name", "version", "uptime", "cpu-load",
			"free-memory", "total-memory", "free-hdd-space", "total-hdd-space"},
	)
	if err != nil {
		t.Fatalf("CallToolResultFromRecord error: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	text := result.Content[0].(mcp.TextContent).Text

	if !strings.Contains(text, "System resource: RouterOS 7.17, uptime 1d2h") {
		t.Errorf("missing summary line")
	}
	if !strings.Contains(text, "| Field | Value |") {
		t.Errorf("missing table header")
	}
	if !strings.Contains(text, "| board-name | RB5009 |") {
		t.Errorf("missing board-name row")
	}
	assertStructuredContent(t, result, record)
}

func TestCallToolResultFromRecordsWithInterfaceList(t *testing.T) {
	items := []map[string]any{
		{
			"name":        "ether1",
			"type":        "ether",
			"running":     "true",
			"disabled":    "false",
			"actual-mtu":  "1500",
			"mac-address": "00:11:22:33:44:55",
		},
	}

	result, err := CallToolResultFromRecords(
		"Interfaces",
		items,
		"interface",
		[][2]string{
			{"name", "Name"},
			{"type", "Type"},
			{"running", "Running"},
			{"disabled", "Disabled"},
			{"actual-mtu", "MTU"},
			{"mac-address", "MAC Address"},
		},
	)
	if err != nil {
		t.Fatalf("CallToolResultFromRecords error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text

	if !strings.Contains(text, "Interfaces: 1 interface") {
		t.Errorf("missing summary, got: %s", text)
	}
	if !strings.Contains(text, "| Name | Type | Running | Disabled | MTU | MAC Address |") {
		t.Errorf("missing table header, got: %s", text)
	}
	if !strings.Contains(text, "| ether1 | ether | yes | no | 1500 | 00:11:22:33:44:55 |") {
		t.Errorf("missing data row, got: %s", text)
	}
	assertStructuredContent(t, result, map[string]any{"result": items})
}

func TestCallToolResultFromRecordsFormatsEmptyValues(t *testing.T) {
	items := []map[string]any{
		{
			"address":    "192.0.2.0/24",
			"gateway":    "192.0.2.1",
			"dns-server": "",
			"domain":     "",
			"ntp-server": "",
		},
	}

	result, err := CallToolResultFromRecords(
		"DHCP Networks",
		items,
		"DHCP network",
		[][2]string{
			{"address", "Address"},
			{"gateway", "Gateway"},
			{"dns-server", "DNS Server"},
			{"domain", "Domain"},
			{"ntp-server", "NTP Server"},
		},
	)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text

	if !strings.Contains(text, "| 192.0.2.0/24 | 192.0.2.1 | - | - | - |") {
		t.Errorf("empty fields should show '-', got: %s", text)
	}
	assertStructuredContent(t, result, map[string]any{"result": items})
}

func TestCallToolResultFromRecordsPingFormat(t *testing.T) {
	items := []map[string]any{
		{
			"seq":    "0",
			"host":   "192.0.2.1",
			"size":   "56",
			"ttl":    "64",
			"time":   "4ms",
			"status": "reachable",
		},
	}

	result, err := CallToolResultFromRecords(
		"Ping 192.0.2.1",
		items,
		"probe",
		[][2]string{
			{"seq", "Seq"},
			{"host", "Host"},
			{"size", "Size"},
			{"ttl", "TTL"},
			{"time", "Time"},
			{"status", "Status"},
		},
	)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text

	if !strings.Contains(text, "Ping 192.0.2.1: 1 probe") {
		t.Errorf("missing ping summary, got: %s", text)
	}
	if !strings.Contains(text, "| Seq | Host | Size | TTL | Time | Status |") {
		t.Errorf("missing ping table header, got: %s", text)
	}
	if !strings.Contains(text, "| 0 | 192.0.2.1 | 56 | 64 | 4ms | reachable |") {
		t.Errorf("missing ping data row, got: %s", text)
	}
	assertStructuredContent(t, result, map[string]any{"result": items})
}

func TestCallToolResultFromRecordsTracerouteFormat(t *testing.T) {
	items := []map[string]any{
		{
			"hop":     "1",
			"host":    "edge-router",
			"address": "198.51.100.1",
			"loss":    "0%",
			"last":    "2ms",
			"avg":     "2ms",
			"best":    "2ms",
			"worst":   "3ms",
			"status":  "reachable",
		},
	}

	result, err := CallToolResultFromRecords(
		"Traceroute example.com",
		items,
		"hop",
		[][2]string{
			{"hop", "Hop"}, {"host", "Host"}, {"address", "Address"},
			{"loss", "Loss"}, {"last", "Last"}, {"avg", "Avg"},
			{"best", "Best"}, {"worst", "Worst"}, {"status", "Status"},
		},
	)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text

	if !strings.Contains(text, "Traceroute example.com: 1 hop") {
		t.Errorf("missing traceroute summary, got: %s", text)
	}
	if !strings.Contains(text, "| Hop | Host | Address | Loss | Last | Avg | Best | Worst | Status |") {
		t.Errorf("missing traceroute table header, got: %s", text)
	}
	if !strings.Contains(text, "| 1 | edge-router | 198.51.100.1 | 0% | 2ms | 2ms | 2ms | 3ms | reachable |") {
		t.Errorf("missing traceroute data row, got: %s", text)
	}
	assertStructuredContent(t, result, map[string]any{"result": items})
}

func TestCallToolResultFromRecordDNSResolve(t *testing.T) {
	record := map[string]any{
		"name":    "example.com",
		"address": "93.184.216.34",
	}

	result, err := CallToolResultFromRecord(
		"DNS Resolve",
		"DNS resolve: example.com -> 93.184.216.34",
		record,
		[]string{"name", "address", "server"},
	)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text

	if !strings.Contains(text, "DNS resolve: example.com -> 93.184.216.34") {
		t.Errorf("missing DNS summary, got: %s", text)
	}
	if !strings.Contains(text, "| name | example.com |") {
		t.Errorf("missing name row, got: %s", text)
	}
	if !strings.Contains(text, "| address | 93.184.216.34 |") {
		t.Errorf("missing address row, got: %s", text)
	}
	assertStructuredContent(t, result, record)
}

func TestCallToolResultFromRecordDNSGet(t *testing.T) {
	record := map[string]any{
		"servers":               "1.1.1.1,8.8.8.8",
		"allow-remote-requests": "true",
		"cache-size":            "2048KiB",
	}

	result, err := CallToolResultFromRecord(
		"DNS Settings",
		"DNS settings: servers 1.1.1.1,8.8.8.8, remote requests yes",
		record,
		[]string{"servers", "allow-remote-requests", "cache-size", "dynamic-servers", "use-doh-server"},
	)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text

	if !strings.Contains(text, "DNS settings: servers 1.1.1.1,8.8.8.8, remote requests yes") {
		t.Errorf("missing DNS summary, got: %s", text)
	}
	if !strings.Contains(text, "| servers | 1.1.1.1,8.8.8.8 |") {
		t.Errorf("missing servers row, got: %s", text)
	}
	assertStructuredContent(t, result, record)
}

func TestDisplayValueTrueFalse(t *testing.T) {
	if v := displayValue("true"); v != "yes" {
		t.Errorf("displayValue(true) = %q", v)
	}
	if v := displayValue("false"); v != "no" {
		t.Errorf("displayValue(false) = %q", v)
	}
	if v := displayValue(true); v != "yes" {
		t.Errorf("displayValue(true) = %q", v)
	}
	if v := displayValue(false); v != "no" {
		t.Errorf("displayValue(false) = %q", v)
	}
	if v := displayValue(nil); v != EMPTY_DISPLAY {
		t.Errorf("displayValue(nil) = %q", v)
	}
	if v := displayValue(""); v != EMPTY_DISPLAY {
		t.Errorf("displayValue('') = %q", v)
	}
}

func TestCallToolResultFromRecordInterfaceGetPreferredFieldsOrder(t *testing.T) {
	record := map[string]any{
		"name":        "ether1",
		"type":        "ether",
		"running":     "true",
		"disabled":    "false",
		"actual-mtu":  "1500",
		"mac-address": "00:11:22:33:44:55",
		"comment":     "",
	}

	result, err := CallToolResultFromRecord(
		"Interface",
		"Interface: ether1",
		record,
		[]string{"name", "type", "running", "disabled", "actual-mtu", "mac-address", "comment"},
	)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text

	if !strings.Contains(text, "Interface: ether1") {
		t.Errorf("missing summary, got: %s", text)
	}
	// Check preferred field order: name should come before type
	namePos := strings.Index(text, "| name |")
	typePos := strings.Index(text, "| type |")
	if namePos < 0 || typePos < 0 {
		t.Errorf("missing name or type row")
	} else if namePos > typePos {
		t.Errorf("expected name before type in preferred fields")
	}
	assertStructuredContent(t, result, record)
}

func TestCallToolResultError(t *testing.T) {
	errResult := CallToolResultError("some error message")
	if !errResult.IsError {
		t.Error("expected IsError true")
	}
	if !strings.Contains(errResult.Content[0].(mcp.TextContent).Text, "some error message") {
		t.Error("missing error message")
	}
}

func TestCallToolResultText(t *testing.T) {
	textResult := CallToolResultText("plain text output")
	if textResult.IsError {
		t.Error("expected IsError false")
	}
	text := textResult.Content[0].(mcp.TextContent).Text
	if text != "plain text output" {
		t.Errorf("text = %q", text)
	}
}
