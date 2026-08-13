package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBinaryRunsAsStdioMCPServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}

	// Build the binary
	binary := "mikrotik-mcp-test"
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	defer os.Remove(binary)

	// Launch the binary as a subprocess with stdio pipes
	cmd := exec.Command("./"+binary, "192.168.88.1")
	cmd.Env = append(os.Environ(),
		"MIKROTIK_USER=test-user",
		"MIKROTIK_PASSWORD=test-pass",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("StderrPipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cmd.Process.Kill()

	// Read stderr in background (for debugging)
	stderrCh := make(chan string, 1)
	go func() {
		b, _ := readAllTimeout(stderr, 2*time.Second)
		stderrCh <- string(b)
	}()

	stdoutReader := bufio.NewReader(stdout)

	// Send initialize request
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "0.0.1",
			},
		},
	}
	writeJSON(t, stdin, req)

	// Read initialize response
	resp := readJSONLine(t, stdoutReader)
	if resp["id"] != float64(1) {
		t.Errorf("initialize response id = %v, want 1", resp["id"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize missing result, got: %v", resp)
	}
	serverInfo, _ := result["serverInfo"].(map[string]any)
	serverName, _ := serverInfo["name"].(string)
	if serverName != "mikrotik" {
		t.Errorf("server name = %q, want mikrotik", serverName)
	}

	// Send initialized notification (required after initialize)
	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	writeJSON(t, stdin, notif)

	// Send tools/list request
	req2 := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	}
	writeJSON(t, stdin, req2)

	// Read tools/list response
	resp2 := readJSONLine(t, stdoutReader)
	if resp2["id"] != float64(2) {
		t.Errorf("tools/list response id = %v, want 2", resp2["id"])
	}
	result2, ok := resp2["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list missing result, got: %v", resp2)
	}
	tools, ok := result2["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list missing tools array, got: %v", result2)
	}

	// Verify at least a few expected tools
	found := make(map[string]bool)
	for _, t := range tools {
		tool, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tool["name"].(string)
		found[name] = true
	}

	expected := []string{"resource_print", "system_identity_get", "interface_list", "tool_ping", "healthcheck"}
	for _, name := range expected {
		if !found[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
	if len(tools) < 55 {
		t.Errorf("got %d tools, want >= 55", len(tools))
	}

	// Graceful shutdown — send shutdown request
	shutdown := map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "shutdown",
	}
	writeJSON(t, stdin, shutdown)
	readJSONLine(t, stdoutReader) // read shutdown response

	stdin.Close()
	cmd.Wait()

	// Check stderr for any unexpected errors
	select {
	case errMsg := <-stderrCh:
		if errMsg != "" && !strings.Contains(errMsg, "Server error") && !strings.Contains(errMsg, "shutdown") {
			t.Logf("stderr: %s", errMsg)
		}
	default:
	}
}

func writeJSON(t *testing.T, w io.WriteCloser, v map[string]any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func readJSONLine(t *testing.T, r *bufio.Reader) map[string]any {
	t.Helper()
	lineCh := make(chan map[string]any, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := r.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			errCh <- err
			return
		}
		lineCh <- data
	}()

	select {
	case data := <-lineCh:
		return data
	case err := <-errCh:
		t.Fatalf("stdout error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for stdout response")
	}
	return nil
}

func readAllTimeout(r io.ReadCloser, timeout time.Duration) ([]byte, error) {
	result := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		b, err := io.ReadAll(r)
		if err != nil {
			errCh <- err
		} else {
			result <- b
		}
	}()
	select {
	case b := <-result:
		return b, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(timeout):
		return nil, nil
	}
}
