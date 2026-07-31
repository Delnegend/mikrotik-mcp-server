package client

import (
	"bytes"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
)

func TestEncodeLengthRouterOSPrefixes(t *testing.T) {
	tests := []struct {
		length   int
		expected []byte
	}{
		{0, []byte{0x00}},
		{127, []byte{0x7f}},
		{128, []byte{0x80, 0x80}},
		{16383, []byte{0xbf, 0xff}},
		{16384, []byte{0xc0, 0x40, 0x00}},
	}
	for _, tt := range tests {
		result, err := encodeLength(tt.length)
		if err != nil {
			t.Errorf("encodeLength(%d) unexpected error: %v", tt.length, err)
			continue
		}
		if !bytes.Equal(result, tt.expected) {
			t.Errorf("encodeLength(%d) = %v, want %v", tt.length, result, tt.expected)
		}
	}
}

func TestDecodeLengthRoundTrip(t *testing.T) {
	lengths := []int{0, 1, 127, 128, 4096, 16383, 16384, 70000}
	for _, length := range lengths {
		encoded, err := encodeLength(length)
		if err != nil {
			t.Fatalf("encodeLength(%d) error: %v", length, err)
		}
		reader := bytes.NewReader(encoded)
		decoded, err := decodeLength(reader)
		if err != nil {
			t.Errorf("decodeLength round-trip failed for %d: %v", length, err)
			continue
		}
		if decoded != length {
			t.Errorf("decodeLength round-trip: got %d, want %d", decoded, length)
		}
	}
}

func TestEncodeLengthNegative(t *testing.T) {
	_, err := encodeLength(-1)
	if err == nil {
		t.Error("expected error for negative length")
	}
}

func TestParseReplySentencesCollectsRecordsAndDone(t *testing.T) {
	reply := parseReplySentences([][]string{
		{"!re", "=.id=*1", "=name=ether1", "=running=true"},
		{"!re", "=.id=*2", "=name=ether2", "=running=false"},
		{"!done", "=ret=ok"},
	})

	wantRecords := []map[string]string{
		{".id": "*1", "name": "ether1", "running": "true"},
		{".id": "*2", "name": "ether2", "running": "false"},
	}
	if len(reply.Records) != len(wantRecords) {
		t.Fatalf("got %d records, want %d", len(reply.Records), len(wantRecords))
	}
	for i := range wantRecords {
		for k, v := range wantRecords[i] {
			if reply.Records[i][k] != v {
				t.Errorf("record[%d][%s] = %q, want %q", i, k, reply.Records[i][k], v)
			}
		}
	}
	if reply.Done["ret"] != "ok" {
		t.Errorf("done ret = %q, want ok", reply.Done["ret"])
	}
	if len(reply.Traps) != 0 {
		t.Errorf("expected no traps, got %d", len(reply.Traps))
	}
	if reply.Fatal != nil {
		t.Errorf("expected no fatal, got %v", reply.Fatal)
	}
}

func TestParseReplySentencesPreservesTagMetadata(t *testing.T) {
	reply := parseReplySentences([][]string{
		{"!re", ".tag=listen-1", "=name=ether1"},
		{"!done", ".tag=listen-1", "=ret=ok"},
	})

	if reply.Tag != "listen-1" {
		t.Errorf("tag = %q, want listen-1", reply.Tag)
	}
	if len(reply.Records) != 1 || reply.Records[0][".tag"] != "listen-1" {
		t.Errorf("records = %v", reply.Records)
	}
	if reply.Done[".tag"] != "listen-1" || reply.Done["ret"] != "ok" {
		t.Errorf("done = %v", reply.Done)
	}
}

func TestPrintBuildsSentenceAndReturnsRecords(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	reResponse := encodeSentence([]string{"!re", "=.id=*1", "=name=ether1", "=disabled=false"})
	doneResponse := encodeSentence([]string{"!done"})
	fake.WriteResponse(reResponse)
	fake.WriteResponse(doneResponse)
	client.conn = fake

	records, err := client.Print("/interface", []string{"name", "disabled"}, []string{"disabled=false", "?#|"}, map[string]any{"detail": true})
	if err != nil {
		t.Fatalf("Print error: %v", err)
	}
	if len(records) != 1 || records[0][".id"] != "*1" {
		t.Errorf("records = %v", records)
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, "/interface/print") {
		t.Errorf("sent missing /interface/print: %q", sent)
	}
}

func TestLoginTrapRaisesCredentialError(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	trapResponse := encodeSentence([]string{"!trap", "=message=invalid user name or password"})
	doneResponse := encodeSentence([]string{"!done"})
	fake.WriteResponse(trapResponse)
	fake.WriteResponse(doneResponse)
	client.conn = fake

	err := client.Login()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "RouterOS login failed") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestAddBuildsSentenceAndReturnsDonePayload(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	doneResponse := encodeSentence([]string{"!done", "=ret=*3"})
	fake.WriteResponse(doneResponse)
	client.conn = fake

	result, err := client.Add("/ip/address", map[string]any{"address": "192.0.2.10/24", "interface": "ether1", "disabled": false})
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}
	if result["ret"] != "*3" {
		t.Errorf("result ret = %v, want *3", result["ret"])
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, "/ip/address/add") {
		t.Errorf("sent missing path: %q", sent)
	}
	if !strings.Contains(sent, "=address=192.0.2.10/24") {
		t.Errorf("sent missing address: %q", sent)
	}
	if !strings.Contains(sent, "=interface=ether1") {
		t.Errorf("sent missing interface: %q", sent)
	}
	if !strings.Contains(sent, "=disabled=false") {
		t.Errorf("sent missing disabled: %q", sent)
	}
}

func TestSetBuildsSentenceWithExplicitItemID(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	doneResponse := encodeSentence([]string{"!done"})
	fake.WriteResponse(doneResponse)
	client.conn = fake

	result, err := client.Set("/ip/address", "*3", map[string]any{"disabled": true})
	if err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if result["success"] != true {
		t.Errorf("result = %v, want success=true", result)
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, "/ip/address/set") {
		t.Errorf("sent missing set: %q", sent)
	}
	if !strings.Contains(sent, "=.id=*3") {
		t.Errorf("sent missing .id: %q", sent)
	}
	if !strings.Contains(sent, "=disabled=true") {
		t.Errorf("sent missing disabled=true: %q", sent)
	}
}

func TestRemoveBuildsSentenceAndReturnsEmptyResult(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	emptyResponse := encodeSentence([]string{"!empty"})
	doneResponse := encodeSentence([]string{"!done"})
	fake.WriteResponse(emptyResponse)
	fake.WriteResponse(doneResponse)
	client.conn = fake

	result, err := client.Remove("/ip/address", "*3")
	if err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if result["empty"] != true {
		t.Errorf("result = %v, want empty=true", result)
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, "=.id=*3") {
		t.Errorf("sent missing .id: %q", sent)
	}
}

func TestRunReturnsRecords(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	reResponse := encodeSentence([]string{"!re", "=host=192.0.2.1", "=status=reachable"})
	doneResponse := encodeSentence([]string{"!done", "=ret=ok"})
	fake.WriteResponse(reResponse)
	fake.WriteResponse(doneResponse)
	client.conn = fake

	result, err := client.Run("/tool/ping", map[string]any{"address": "192.0.2.1", "count": "1"}, []string{"status=reachable"}, "")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	records, ok := result.([]map[string]string)
	if !ok || len(records) != 1 {
		t.Fatalf("expected []records, got %T %v", result, result)
	}
	if records[0]["host"] != "192.0.2.1" || records[0]["status"] != "reachable" {
		t.Errorf("records = %v", records)
	}
}

func TestListenReturnsBoundedRecordsAndCancelsByTag(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	enc := func(words []string) []byte {
		return encodeSentence(words)
	}
	fake.WriteResponse(enc([]string{"!re", ".tag=listen-1", "=name=ether1"}))
	fake.WriteResponse(enc([]string{"!re", ".tag=listen-1", "=name=ether2"}))
	fake.WriteResponse(enc([]string{"!done"}))
	fake.WriteResponse(enc([]string{"!done", ".tag=listen-1", "=ret=interrupted"}))
	client.conn = fake

	result, err := client.Listen("/interface", nil, []string{"running=true"}, nil, "listen-1", 2)
	if err != nil {
		t.Fatalf("Listen error: %v", err)
	}

	if result.Tag != "listen-1" {
		t.Errorf("tag = %q", result.Tag)
	}
	if len(result.Records) != 2 {
		t.Fatalf("got %d records, want 2", len(result.Records))
	}
	if !result.Cancelled {
		t.Error("expected cancelled=true")
	}
	if !result.LimitReached {
		t.Error("expected limit_reached=true")
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, "/interface/listen") {
		t.Errorf("listen sent missing: %q", sent)
	}
	if !strings.Contains(sent, "/cancel") {
		t.Errorf("missing cancel: %q", sent)
	}
}

func TestListenRequiresPositiveMaxEvents(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	client.conn = fake

	_, err := client.Listen("/interface", nil, nil, nil, "", 0)
	if err == nil {
		t.Fatal("expected error for max_events=0")
	}
	if !strings.Contains(err.Error(), "max_events must be at least 1") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestListenRaisesWhenCancelReturnsFatal(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	enc := func(words []string) []byte {
		return encodeSentence(words)
	}
	fake.WriteResponse(enc([]string{"!re", ".tag=listen-1", "=name=ether1"}))
	fake.WriteResponse(enc([]string{"!fatal", "=message=connection closing"}))
	client.conn = fake

	_, err := client.Listen("/interface", nil, nil, nil, "listen-1", 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "connection closing") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestCancelBuildsCancelSentence(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	doneResponse := encodeSentence([]string{"!done", "=ret=ok"})
	fake.WriteResponse(doneResponse)
	client.conn = fake

	result, err := client.Cancel("listen-1")
	if err != nil {
		t.Fatalf("Cancel error: %v", err)
	}
	if result["ret"] != "ok" {
		t.Errorf("result = %v", result)
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, "/cancel") {
		t.Errorf("missing /cancel: %q", sent)
	}
	if !strings.Contains(sent, "=tag=listen-1") {
		t.Errorf("missing tag: %q", sent)
	}
}

func TestCloneCopiesConnectionSettings(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret",
		WithTLS(true),
		WithTLSVerify(true),
		WithTLSCAFiles([]string{"/tmp/router-ca.pem"}),
		WithPort(8729),
	)

	cloned := client.Clone()

	if cloned == client {
		t.Error("cloned should not be the same pointer")
	}
	if cloned.host != client.host {
		t.Errorf("host = %q, want %q", cloned.host, client.host)
	}
	if cloned.port != client.port {
		t.Errorf("port = %d, want %d", cloned.port, client.port)
	}
	if cloned.useSSL != client.useSSL {
		t.Errorf("useSSL = %v", cloned.useSSL)
	}
	if cloned.tlsVerify != client.tlsVerify {
		t.Errorf("tlsVerify = %v", cloned.tlsVerify)
	}
	if len(cloned.tlsCAFiles) != len(client.tlsCAFiles) || cloned.tlsCAFiles[0] != client.tlsCAFiles[0] {
		t.Errorf("tlsCAFiles = %v", cloned.tlsCAFiles)
	}
}

func TestReadWordFallsBackToLatin1ForNonUTF8(t *testing.T) {
	fake := newFakeConn()
	fake.WriteResponse([]byte{0x01, 0xf3})
	client := NewRouterOSClient("router.test", "admin", "secret")
	client.conn = fake

	word, err := readWord(client.conn)
	if err != nil {
		t.Fatalf("readWord error: %v", err)
	}
	if word != "ó" {
		t.Errorf("word = %q, want ó", word)
	}
}

func TestMutationTrapRaisesClearRouterOSError(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	trapResponse := encodeSentence([]string{"!trap", "=category=1", "=message=failure"})
	doneResponse := encodeSentence([]string{"!done"})
	fake.WriteResponse(trapResponse)
	fake.WriteResponse(doneResponse)
	client.conn = fake

	_, err := client.Remove("/ip/address", "*3")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "RouterOS command failed (1): failure") {
		t.Errorf("error = %q", err.Error())
	}
}

// ---- Phase 2.2: Missing & Strengthened Tests ----

func TestRunReturnsDonePayloadWithoutRecords(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	fake.WriteResponse(encodeSentence([]string{"!done", "=ret=ok"}))
	client.conn = fake

	result, err := client.Run("/system/backup/save", map[string]any{"name": "test"}, nil, "")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["ret"] != "ok" {
		t.Errorf("ret = %v, want ok", m["ret"])
	}
}

func TestRunSupportsExplicitTag(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	fake.WriteResponse(encodeSentence([]string{"!done", "=ret=ok"}))
	client.conn = fake

	_, err := client.Run("/tool/ping", map[string]any{"address": "10.0.0.1"}, nil, "ping-1")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, ".tag=ping-1") {
		t.Errorf("sent missing tag: %q", sent)
	}
}

func TestExecuteOpensConnectionLazily(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	// conn is nil; execute should call Open() which dials.
	// Unit test can't mock Open() without network, so verify that
	// setting conn beforehand avoids the Open() call.
	fake.WriteResponse(encodeSentence([]string{"!done", "=ret=ok"}))
	client.conn = fake

	_, err := client.Run("/system/identity/print", nil, nil, "")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	sent := string(fake.Sent())
	if !strings.Contains(sent, "/system/identity/print") {
		t.Errorf("expected print sentence, got: %q", sent)
	}
	// No login sentence should appear — Open() was skipped because conn was set
	if strings.Contains(sent, "/login") {
		t.Errorf("unexpected login sentence; Open() should not be called when conn is set: %q", sent)
	}
}

func TestListenGeneratesTagWhenNotProvided(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	enc := func(words []string) []byte { return encodeSentence(words) }
	fake.WriteResponse(enc([]string{"!done"}))
	fake.WriteResponse(enc([]string{"!done", "=ret=interrupted"}))
	client.conn = fake

	result, err := client.Listen("/interface", nil, nil, nil, "", 1)
	if err != nil {
		t.Fatalf("Listen error: %v", err)
	}
	if !strings.HasPrefix(result.Tag, "listen-") {
		t.Errorf("expected auto-generated tag, got %q", result.Tag)
	}
	sent := string(fake.Sent())
	if !strings.Contains(sent, ".tag="+result.Tag) {
		t.Errorf("sent missing .tag=%s: %q", result.Tag, sent)
	}
}

func TestListenUsesRouterOSDotTagWord(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	enc := func(words []string) []byte { return encodeSentence(words) }
	fake.WriteResponse(enc([]string{"!re", ".tag=test-tag", "=name=ether1"}))
	fake.WriteResponse(enc([]string{"!done"}))
	fake.WriteResponse(enc([]string{"!done", ".tag=test-tag", "=ret=interrupted"}))
	client.conn = fake

	result, err := client.Listen("/interface", nil, nil, nil, "test-tag", 1)
	if err != nil {
		t.Fatalf("Listen error: %v", err)
	}
	if result.Tag != "test-tag" {
		t.Errorf("tag = %q, want test-tag", result.Tag)
	}
}

func TestListenReturnsErrorOnEmptyStream(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	// No response data — Read returns io.EOF immediately
	client.conn = fake

	_, err := client.Listen("/interface", nil, nil, nil, "listen-1", 10)
	if err == nil {
		t.Fatal("expected error for empty stream")
	}
	// Should not hang — returns promptly with an error
	sent := string(fake.Sent())
	if !strings.Contains(sent, "/interface/listen") {
		t.Errorf("expected listen sentence, got: %q", sent)
	}
}

func TestTLSSessionInfoReturnsNilForPlainSocket(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	client.conn = fake

	info := client.TLSSessionInfo()
	if info != nil {
		t.Errorf("expected nil for plain socket, got %v", info)
	}
}

func TestSetRequiresNonEmptyItemID(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	_, err := client.Set("/ip/address", "", map[string]any{"disabled": true})
	if err == nil {
		t.Fatal("expected error for empty item_id")
	}
	if !strings.Contains(err.Error(), "item_id is required") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestRemoveRequiresNonEmptyItemID(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	_, err := client.Remove("/ip/address", "")
	if err == nil {
		t.Fatal("expected error for empty item_id")
	}
	if !strings.Contains(err.Error(), "item_id is required") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestCancelRequiresNonEmptyTag(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	_, err := client.Cancel("")
	if err == nil {
		t.Fatal("expected error for empty tag")
	}
	if !strings.Contains(err.Error(), "tag is required") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestPrintRequiresNonEmptyMenu(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	client.conn = fake

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty menu in Print")
		}
	}()
	client.Print("", nil, nil, nil)
}

// ---- Strengthened existing tests ----

func TestPrintBuildsSentenceAndReturnsRecords_Strengthened(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	reResponse := encodeSentence([]string{"!re", "=.id=*1", "=name=ether1", "=disabled=false"})
	doneResponse := encodeSentence([]string{"!done"})
	fake.WriteResponse(reResponse)
	fake.WriteResponse(doneResponse)
	client.conn = fake

	records, err := client.Print("/interface", []string{"name", "disabled"}, []string{"disabled=false", "?#|"}, map[string]any{"detail": true})
	if err != nil {
		t.Fatalf("Print error: %v", err)
	}
	if len(records) != 1 || records[0][".id"] != "*1" {
		t.Errorf("records = %v", records)
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, "=.proplist=name,disabled") {
		t.Errorf("sent missing proplist: %q", sent)
	}
	if !strings.Contains(sent, "=detail=true") {
		t.Errorf("sent missing detail=true: %q", sent)
	}
	if !strings.Contains(sent, "?disabled=false") {
		t.Errorf("sent missing query: %q", sent)
	}
	if !strings.Contains(sent, "?#|") {
		t.Errorf("sent missing OR query: %q", sent)
	}
}

func TestRemoveBuildsSentenceAndReturnsEmptyResult_Strengthened(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	emptyResponse := encodeSentence([]string{"!empty"})
	doneResponse := encodeSentence([]string{"!done"})
	fake.WriteResponse(emptyResponse)
	fake.WriteResponse(doneResponse)
	client.conn = fake

	result, err := client.Remove("/ip/address", "*3")
	if err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if result["empty"] != true {
		t.Errorf("result = %v, want empty=true", result)
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, "/ip/address/remove") {
		t.Errorf("sent missing path: %q", sent)
	}
	if !strings.Contains(sent, "=.id=*3") {
		t.Errorf("sent missing .id=*3: %q", sent)
	}
}

func TestRunReturnsRecords_Strengthened(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	reResponse := encodeSentence([]string{"!re", "=host=192.0.2.1", "=status=reachable"})
	doneResponse := encodeSentence([]string{"!done", "=ret=ok"})
	fake.WriteResponse(reResponse)
	fake.WriteResponse(doneResponse)
	client.conn = fake

	result, err := client.Run("/tool/ping", map[string]any{"address": "192.0.2.1", "count": "1"}, []string{"status=reachable"}, "")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	records, ok := result.([]map[string]string)
	if !ok || len(records) != 1 {
		t.Fatalf("expected []records, got %T %v", result, result)
	}
	if records[0]["host"] != "192.0.2.1" || records[0]["status"] != "reachable" {
		t.Errorf("records = %v", records)
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, "?status=reachable") {
		t.Errorf("sent missing query: %q", sent)
	}
	if !strings.Contains(sent, "=count=1") {
		t.Errorf("sent missing count: %q", sent)
	}
}

func TestListenReturnsBoundedRecordsAndCancelsByTag_Strengthened(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	enc := func(words []string) []byte { return encodeSentence(words) }
	fake.WriteResponse(enc([]string{"!re", ".tag=listen-1", "=name=ether1"}))
	fake.WriteResponse(enc([]string{"!done"}))
	fake.WriteResponse(enc([]string{"!done", ".tag=listen-1", "=ret=interrupted"}))
	client.conn = fake

	result, err := client.Listen("/interface", nil, []string{"running=true"}, nil, "listen-1", 1)
	if err != nil {
		t.Fatalf("Listen error: %v", err)
	}

	if result.Tag != "listen-1" {
		t.Errorf("tag = %q", result.Tag)
	}
	if len(result.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(result.Records))
	}
	if !result.Cancelled {
		t.Error("expected cancelled=true")
	}
	if !result.LimitReached {
		t.Error("expected limit_reached=true")
	}
	if result.CancelDone == nil {
		t.Error("expected CancelDone to be non-nil")
	}

	sent := string(fake.Sent())
	if !strings.Contains(sent, ".tag=listen-1") {
		t.Errorf("sent missing .tag=listen-1: %q", sent)
	}
	if !strings.Contains(sent, "/cancel") {
		t.Errorf("missing cancel: %q", sent)
	}
}

func TestLoginTrapRaisesCredentialError_Strengthened(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()

	trapResponse := encodeSentence([]string{"!trap", "=message=invalid user name or password"})
	doneResponse := encodeSentence([]string{"!done"})
	fake.WriteResponse(trapResponse)
	fake.WriteResponse(doneResponse)
	client.conn = fake

	err := client.Login()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrRouterOSAuthError) {
		t.Errorf("error should wrap ErrRouterOSAuthError, got: %T %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// fakeConn wraps testutil.FakeConn for backward compatibility
// ---------------------------------------------------------------------------

func newFakeConn() *testutil.FakeConn {
	return testutil.NewFakeConn()
}

// blockingConn is a net.Conn whose Read blocks until a deadline is set,
// then returns a timeout error.
type blockingConn struct {
	sent     bytes.Buffer
	closed   bool
	deadline time.Time
}

func (b *blockingConn) Read(p []byte) (int, error) {
	for {
		if !b.deadline.IsZero() && time.Now().After(b.deadline) {
			return 0, os.ErrDeadlineExceeded
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (b *blockingConn) Write(p []byte) (int, error) {
	return b.sent.Write(p)
}

func (b *blockingConn) Close() error                       { b.closed = true; return nil }
func (b *blockingConn) LocalAddr() net.Addr                { return nil }
func (b *blockingConn) RemoteAddr() net.Addr               { return nil }
func (b *blockingConn) SetDeadline(t time.Time) error      { b.deadline = t; return nil }
func (b *blockingConn) SetReadDeadline(t time.Time) error  { b.deadline = t; return nil }
func (b *blockingConn) SetWriteDeadline(t time.Time) error { return nil }

func TestClientHonoursDeadline(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret", WithTimeout(50*time.Millisecond))
	bc := &blockingConn{}
	client.conn = bc

	done := make(chan error, 1)
	go func() {
		_, err := client.Run("/system/identity/print", nil, nil, "")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from blocked read")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("deadline was not honoured — Run blocked forever")
	}
}

func TestNormalizeAttrsSortsKeys(t *testing.T) {
	attrs := normalizeAttrs(map[string]any{
		"zebra": "z",
		"alpha": "a",
		"mike":  "m",
		"nil":   nil, // skipped
	})
	var keys []string
	for _, attr := range attrs {
		keys = append(keys, attr.key)
	}
	want := []string{"alpha", "mike", "zebra"}
	if len(keys) != len(want) {
		t.Fatalf("got %d attrs, want %d: %v", len(keys), len(want), keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("attr[%d] = %q, want %q (full: %v)", i, keys[i], want[i], keys)
		}
	}
}

func TestConcurrentExecutesSerializeSentences(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	fake := newFakeConn()
	// 5 responses, one per concurrent call
	for i := 0; i < 5; i++ {
		fake.WriteResponse(encodeSentence([]string{"!done"}))
	}
	client.conn = fake

	commands := []string{"/cmd/a", "/cmd/b", "/cmd/c", "/cmd/d", "/cmd/e"}
	var wg sync.WaitGroup
	for _, cmd := range commands {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			_, err := client.Run(c, map[string]any{"name": c}, nil, "")
			if err != nil {
				t.Errorf("Run(%s) error: %v", c, err)
			}
		}(cmd)
	}
	wg.Wait()

	// Decode the sent buffer back into sentences and verify each is complete
	// and contains exactly one command path.
	sentences, err := testutil.DecodeSentences(fake.Sent())
	if err != nil {
		t.Fatalf("decode sent data: %v", err)
	}
	if len(sentences) != 5 {
		t.Fatalf("got %d sentences, want 5 (interleaving detected)", len(sentences))
	}
	found := make(map[string]bool)
	for _, s := range sentences {
		matched := false
		for _, cmd := range commands {
			if len(s) > 0 && s[0] == cmd {
				matched = true
				if found[cmd] {
					t.Errorf("command %s appeared more than once", cmd)
				}
				found[cmd] = true
			}
		}
		if !matched {
			t.Errorf("sentence has unexpected first word: %v", s)
		}
	}
	for _, cmd := range commands {
		if !found[cmd] {
			t.Errorf("missing sentence for %s", cmd)
		}
	}
}
