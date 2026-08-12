package client

import (
	"bytes"
	"reflect"
	"testing"
)

func TestEncodeLengthRouterOSPrefixes(t *testing.T) {
	// Full boundary table from the official RouterOS API spec
	// (manual.mikrotik.com/docs/developer-guides/api/):
	//   0 <= len <= 0x7F          -> 1 byte
	//   0x80 <= len <= 0x3FFF     -> 2 bytes, len|0x8000
	//   0x4000 <= len <= 0x1FFFFF -> 3 bytes, len|0xC00000
	//   0x200000 <= len <= 0xFFFFFFF -> 4 bytes, len|0xE0000000
	//   len >= 0x10000000         -> 0xF0 + 4-byte len
	tests := []struct {
		length   int
		expected []byte
	}{
		{0, []byte{0x00}},
		{127, []byte{0x7f}},
		{128, []byte{0x80, 0x80}},
		{16383, []byte{0xbf, 0xff}},
		{16384, []byte{0xc0, 0x40, 0x00}},
		{0x1FFFFF, []byte{0xdf, 0xff, 0xff}},
		{0x200000, []byte{0xe0, 0x20, 0x00, 0x00}},
		{0x0FFFFFFF, []byte{0xef, 0xff, 0xff, 0xff}},
		{0x10000000, []byte{0xf0, 0x10, 0x00, 0x00, 0x00}},
		{0xFFFFFFFF, []byte{0xf0, 0xff, 0xff, 0xff, 0xff}},
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

func TestEncodeLengthTooLarge(t *testing.T) {
	_, err := encodeLength(0x100000000)
	if err == nil {
		t.Error("expected error for length beyond the API's 4-byte limit")
	}
}

func TestDecodeLengthReservedControlBytes(t *testing.T) {
	// The spec reserves first bytes >= 0xF8 as control bytes; only 0xF0
	// (long length) is defined. Anything else must be rejected.
	for _, b := range []byte{0xF1, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF} {
		if _, err := decodeLength(bytes.NewReader([]byte{b})); err == nil {
			t.Errorf("decodeLength(0x%02x) expected error for reserved control byte", b)
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

// officialLengthPrefix and officialSentence are an independent transcription
// of the reference implementation published by MikroTik in the official API
// Python3 example (manual.mikrotik.com/docs/developer-guides/api/python3-example).
// Used to cross-check our encoder without testing it against itself.
func officialLengthPrefix(length int) []byte {
	switch {
	case length < 0x80:
		return []byte{byte(length)}
	case length < 0x4000:
		l := length | 0x8000
		return []byte{byte(l >> 8), byte(l)}
	case length < 0x200000:
		l := length | 0xC00000
		return []byte{byte(l >> 16), byte(l >> 8), byte(l)}
	case length < 0x10000000:
		l := length | 0xE0000000
		return []byte{byte(l >> 24), byte(l >> 16), byte(l >> 8), byte(l)}
	default:
		return append([]byte{0xF0},
			byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	}
}

func officialSentence(words ...string) []byte {
	var buf []byte
	for _, w := range words {
		buf = append(buf, officialLengthPrefix(len(w))...)
		buf = append(buf, w...)
	}
	return append(buf, 0)
}

func TestEncodeSentenceMatchesOfficialReference(t *testing.T) {
	cases := [][]string{
		{"/user/getall"},
		{"/login", "=name=admin", "=password="},
		{"/interface/print", "?type=ether", "=.proplist=.id"},
		{"/cancel", "=tag=2", ".tag=7"},
		{"!re", "=.id=*1", "=name=ether1", "=disabled=false", ".tag=2"},
	}
	for _, words := range cases {
		want := officialSentence(words...)
		if got := encodeSentence(words); !bytes.Equal(got, want) {
			t.Errorf("encodeSentence(%v)\n got %x\nwant %x", words, got, want)
		}
	}
}

func TestParseOfficialGetallTrace(t *testing.T) {
	// Exact reply shown in the official API docs, "Example client" section.
	sentences := [][]string{
		{"!re", "=.id=*1", "=disabled=no", "=name=admin", "=group=full", "=address=0.0.0.0/0", "=netmask=0.0.0.0"},
		{"!done"},
	}
	bundle := parseReplySentences(sentences)
	if len(bundle.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(bundle.Records))
	}
	want := map[string]string{
		".id": "*1", "disabled": "no", "name": "admin", "group": "full",
		"address": "0.0.0.0/0", "netmask": "0.0.0.0",
	}
	if !reflect.DeepEqual(bundle.Records[0], want) {
		t.Errorf("record = %v, want %v", bundle.Records[0], want)
	}
	if bundle.Done == nil {
		t.Error("expected !done attributes")
	}
}

func TestParseOfficialCancelTrace(t *testing.T) {
	// Exact replies shown in the official API docs, "/cancel, simultaneous
	// commands" example (listen tag 2, cancel tag 7).
	sentences := [][]string{
		{"!trap", "=category=2", "=message=interrupted", ".tag=2"},
		{"!done", ".tag=7"},
		{"!done", ".tag=2"},
	}
	bundle := parseReplySentences(sentences)
	if len(bundle.Traps) != 1 {
		t.Fatalf("got %d traps, want 1", len(bundle.Traps))
	}
	trap := bundle.Traps[0]
	if trap["category"] != "2" || trap["message"] != "interrupted" || trap[".tag"] != "2" {
		t.Errorf("trap = %v", trap)
	}
	if bundle.Tag != "2" {
		t.Errorf("bundle tag = %q, want 2", bundle.Tag)
	}
	if bundle.Done == nil {
		t.Error("expected !done attributes")
	}
}

func TestParseOfficialOIDPrintTrace(t *testing.T) {
	// Exact reply shown in the official API docs, "OID" section.
	sentences := [][]string{
		{"!re", "=uptime=01:22:53", "=cpu-load=0",
			"=uptime.oid=.1.3.6.1.2.1.1.3.0", "=cpu-load.oid=.1.3.6.1.2.1.25.3.3.1.2.1"},
		{"!done"},
	}
	bundle := parseReplySentences(sentences)
	if len(bundle.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(bundle.Records))
	}
	rec := bundle.Records[0]
	if rec["uptime"] != "01:22:53" || rec["uptime.oid"] != ".1.3.6.1.2.1.1.3.0" {
		t.Errorf("record = %v", rec)
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

func TestReadWordFallsBackToLatin1ForNonUTF8(t *testing.T) {
	word, err := readWord(bytes.NewReader([]byte{0x01, 0xf3}))
	if err != nil {
		t.Fatalf("readWord error: %v", err)
	}
	if word != "ó" {
		t.Errorf("word = %q, want ó", word)
	}
}

func TestTLSSessionInfoReturnsNilForPlainSocket(t *testing.T) {
	client := NewRouterOSClient("router.test", "admin", "secret")
	if info := client.TLSSessionInfo(); info != nil {
		t.Errorf("expected nil for non-TLS socket, got %v", info)
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
