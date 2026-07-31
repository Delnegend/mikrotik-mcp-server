package server

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
	"github.com/mark3labs/mcp-go/mcp"
)

// Wire-protocol helpers

func newFakeConn(responses ...[]byte) *testutil.FakeConn {
	return testutil.NewFakeConn(responses...)
}

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

// Mocks

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

type mockDownloader struct {
	downloadErr error
	failOnCall  int
	callCount   int
}

func (m *mockDownloader) DownloadFile(routerPath, localPath string) error {
	m.callCount++
	if m.failOnCall > 0 && m.callCount == m.failOnCall {
		return fmt.Errorf("simulated download failure for %s", routerPath)
	}
	if m.downloadErr != nil {
		return m.downloadErr
	}
	return nil
}

// Test-scoped client + fakeConn bundle

type toolTest struct {
	CL *client.RouterOSClient
	FC *testutil.FakeConn
}

func newToolTest(t *testing.T, responses ...[]byte) toolTest {
	t.Helper()
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(responses...)
	cl.SetConn(fc)
	return toolTest{CL: cl, FC: fc}
}

// Convenience helpers

func mkHealthcheckReq() mcp.CallToolRequest {
	return testutil.MkReq("healthcheck")
}

func assertResult(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if result.IsError {
		t.Fatalf("handler error: %s", result.Content[0].(mcp.TextContent).Text)
	}
}

func assertStructuredContent(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if result.Meta == nil {
		t.Fatal("missing result.Meta")
	}
	if _, ok := result.Meta.AdditionalFields["structuredContent"]; !ok {
		t.Fatal("missing structuredContent in result.Meta")
	}
}

func resultText(result *mcp.CallToolResult) string {
	return result.Content[0].(mcp.TextContent).Text
}

func ctx() context.Context {
	return context.Background()
}

// mapToArgs converts a map to alternating key/value arguments for mkReq
func mapToArgs(m map[string]any) []any {
	var args []any
	for k, v := range m {
		args = append(args, k, v)
	}
	return args
}

// assertSentExact decodes the sent data and asserts it is exactly one
// sentence with the exact word list (D9 deterministic-wire assertions).
func assertSentExact(t *testing.T, fc *testutil.FakeConn, want []string) {
	t.Helper()
	sentences, err := testutil.DecodeSentences(fc.Sent())
	if err != nil {
		t.Fatalf("decode sent data: %v", err)
	}
	if len(sentences) != 1 {
		t.Fatalf("got %d sentences, want 1: %v", len(sentences), sentences)
	}
	if !reflect.DeepEqual(sentences[0], want) {
		t.Errorf("sent words = %v, want %v", sentences[0], want)
	}
}

// assertSentContainsExact asserts that at least one decoded sentence exactly
// equals the given word list (for multi-operation flows).
func assertSentContainsExact(t *testing.T, fc *testutil.FakeConn, want []string) {
	t.Helper()
	sentences, err := testutil.DecodeSentences(fc.Sent())
	if err != nil {
		t.Fatalf("decode sent data: %v", err)
	}
	for _, s := range sentences {
		if reflect.DeepEqual(s, want) {
			return
		}
	}
	t.Errorf("no sent sentence matches %v; got %v", want, sentences)
}

// containsAll checks that s contains all of the given substrings
func containsAll(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// decodeSentences splits raw RouterOS wire bytes into complete sentences.
func decodeSentences(raw []byte) ([][]string, error) {
	return testutil.DecodeSentences(raw)
}
