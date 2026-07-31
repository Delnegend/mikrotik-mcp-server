package server

import (
	"context"
	"errors"
	"fmt"
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

// assertSent checks that the fake conn's sent data contains the given substring
func assertSent(t *testing.T, fc *testutil.FakeConn, substr string) {
	t.Helper()
	sent := string(fc.Sent())
	if !strings.Contains(sent, substr) {
		t.Errorf("expected sent data to contain %q, got: %s", substr, sent)
	}
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
	var sentences [][]string
	pos := 0
	for pos < len(raw) {
		var words []string
		for {
			length, n, err := decodeWordLength(raw[pos:])
			if err != nil {
				return nil, err
			}
			pos += n
			if length == 0 {
				break
			}
			words = append(words, string(raw[pos:pos+length]))
			pos += length
		}
		sentences = append(sentences, words)
	}
	return sentences, nil
}

func decodeWordLength(b []byte) (int, int, error) {
	if len(b) < 1 {
		return 0, 0, errors.New("truncated length prefix")
	}
	v := b[0]
	switch {
	case v&0x80 == 0:
		return int(v), 1, nil
	case v&0xC0 == 0x80:
		if len(b) < 2 {
			return 0, 0, errors.New("truncated length prefix")
		}
		return int(uint16(v&0x3F)<<8 | uint16(b[1])), 2, nil
	default:
		return 0, 0, errors.New("unsupported length prefix")
	}
}
