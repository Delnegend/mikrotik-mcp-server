package client

import (
	"bytes"
	"net"
	"testing"
	"time"
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
	if !contains(sent, "/interface/print") {
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
	if !contains(err.Error(), "RouterOS login failed") {
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
	if !contains(sent, "/ip/address/add") {
		t.Errorf("sent missing path: %q", sent)
	}
	if !contains(sent, "=address=192.0.2.10/24") {
		t.Errorf("sent missing address: %q", sent)
	}
	if !contains(sent, "=interface=ether1") {
		t.Errorf("sent missing interface: %q", sent)
	}
	if !contains(sent, "=disabled=false") {
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
	if !contains(sent, "/ip/address/set") {
		t.Errorf("sent missing set: %q", sent)
	}
	if !contains(sent, "=.id=*3") {
		t.Errorf("sent missing .id: %q", sent)
	}
	if !contains(sent, "=disabled=true") {
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
	if !contains(sent, "=.id=*3") {
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
	if !contains(sent, "/interface/listen") {
		t.Errorf("listen sent missing: %q", sent)
	}
	if !contains(sent, "/cancel") {
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
	if !contains(err.Error(), "max_events must be at least 1") {
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
	if !contains(err.Error(), "connection closing") {
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
	if !contains(sent, "/cancel") {
		t.Errorf("missing /cancel: %q", sent)
	}
	if !contains(sent, "=tag=listen-1") {
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
	if !contains(err.Error(), "RouterOS command failed (1): failure") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestNormalizeMenuRejectsEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty menu")
		}
	}()
	normalizeMenu("")
}

func TestNormalizeItemIDRejectsEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for empty item_id")
		}
	}()
	normalizeItemID("")
}

// ---------------------------------------------------------------------------
// fakeConn: net.Conn for testing RouterOSClient
// ---------------------------------------------------------------------------

type fakeConn struct {
	buf    bytes.Buffer
	sent   bytes.Buffer
	closed bool
}

var _ net.Conn = (*fakeConn)(nil)

func newFakeConn() *fakeConn {
	return &fakeConn{}
}

func (f *fakeConn) WriteResponse(data []byte) {
	f.buf.Write(data)
}

func (f *fakeConn) Read(b []byte) (int, error) {
	return f.buf.Read(b)
}

func (f *fakeConn) Write(b []byte) (int, error) {
	return f.sent.Write(b)
}

func (f *fakeConn) Sent() []byte {
	return f.sent.Bytes()
}

func (f *fakeConn) Close() error {
	f.closed = true
	return nil
}

func (f *fakeConn) LocalAddr() net.Addr  { return nil }
func (f *fakeConn) RemoteAddr() net.Addr { return nil }
func (f *fakeConn) SetDeadline(t time.Time) error      { return nil }
func (f *fakeConn) SetReadDeadline(t time.Time) error  { return nil }
func (f *fakeConn) SetWriteDeadline(t time.Time) error { return nil }

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
