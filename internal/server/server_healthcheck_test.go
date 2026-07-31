package server

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/downloads"
	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
)

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

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=lab-router"), enc("!done"))
	cl.SetConn(fc)

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
			"host": host, "port": port, "key_type": "ssh-ed25519",
			"fingerprint_sha256": "SHA256:server-fingerprint",
		}, nil
	}
	hcCheckPasswordRotationReady = func(host, username string) (map[string]any, error) {
		return map[string]any{"host": host, "port": 22, "username": username, "target_exists": true}, nil
	}
	hcResolveSCPPrivateKeyPath = func() (string, error) { return "", nil }

	result, err := handlerHealthcheck(cl)(ctx(), mkHealthcheckReq())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
}

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

	testutil.Setenv(t, "MIKROTIK_USER", "api-user")
	testutil.Setenv(t, "MIKROTIK_PASSWORD", "api-pass")
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

	result, err := handlerHealthcheck(cl)(ctx(), mkHealthcheckReq())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Healthcheck: degraded") {
		t.Errorf("expected degraded, got: %s", text)
	}
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

	testutil.Setenv(t, "MIKROTIK_USER", "api-user")
	testutil.Setenv(t, "MIKROTIK_PASSWORD", "api-pass")
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return nil, fmt.Errorf("SCP config missing")
	}
	hcProbeSSHFingerprint = func(host string, port int, timeout time.Duration) (map[string]any, error) {
		return map[string]any{"status": "ok"}, nil
	}
	hcResolveSCPPrivateKeyPath = func() (string, error) { return "", nil }

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!trap", "=message=api unavailable"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerHealthcheck(cl)(ctx(), mkHealthcheckReq())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !strings.Contains(text, "Healthcheck: failed") {
		t.Errorf("expected failed, got: %s", text)
	}
}

func TestIntegrationHealthcheckPasswordlessFingerprintMissing(t *testing.T) {
	origResolve := hcResolveSCPPrivateKeyPath
	origLoad := hcLoadFileTransferSettings
	defer func() {
		hcResolveSCPPrivateKeyPath = origResolve
		hcLoadFileTransferSettings = origLoad
	}()

	testutil.Setenv(t, "MIKROTIK_USER", "api-user")
	testutil.Setenv(t, "MIKROTIK_PASSWORD", "api-pass")
	testutil.Setenv(t, "MIKROTIK_API_PASSWORDLESS_ENABLED", "true")
	hcResolveSCPPrivateKeyPath = func() (string, error) { return "/path/to/key", nil }
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22}, nil
	}

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=lab-router"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerHealthcheck(cl)(ctx(), mkHealthcheckReq())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if strings.Contains(text, "Healthcheck: healthy") {
		t.Errorf("expected non-healthy healthcheck: %s", text)
	}
	if !strings.Contains(text, "passwordless.") {
		t.Errorf("expected passwordless status: %s", text)
	}
}

func TestIntegrationHealthcheckPasswordlessStartupFailed(t *testing.T) {
	origResolve := hcResolveSCPPrivateKeyPath
	origLoad := hcLoadFileTransferSettings
	origProbe := hcProbeSSHFingerprint
	defer func() {
		hcResolveSCPPrivateKeyPath = origResolve
		hcLoadFileTransferSettings = origLoad
		hcProbeSSHFingerprint = origProbe
	}()

	testutil.Setenv(t, "MIKROTIK_USER", "api-user")
	testutil.Setenv(t, "MIKROTIK_PASSWORD", "api-pass")
	testutil.Setenv(t, "MIKROTIK_API_PASSWORDLESS_ENABLED", "true")
	testutil.Setenv(t, "MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", "SHA256:test-key")
	hcResolveSCPPrivateKeyPath = func() (string, error) { return "/path/to/key", nil }
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22}, nil
	}
	hcProbeSSHFingerprint = func(host string, port int, timeout time.Duration) (map[string]any, error) {
		return map[string]any{"status": "ok"}, nil
	}

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=lab-router"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerHealthcheck(cl)(ctx(), mkHealthcheckReq())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if strings.Contains(text, "Healthcheck: healthy") {
		t.Errorf("expected non-healthy healthcheck: %s", text)
	}
	if !strings.Contains(text, "passwordless.") {
		t.Errorf("expected passwordless status: %s", text)
	}
}

func TestIntegrationHealthcheckSCPConfigMissing(t *testing.T) {
	origLoad := hcLoadFileTransferSettings
	defer func() { hcLoadFileTransferSettings = origLoad }()

	testutil.Setenv(t, "MIKROTIK_USER", "api-user")
	testutil.Setenv(t, "MIKROTIK_PASSWORD", "api-pass")
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return nil, fmt.Errorf("MIKROTIK_SCP_PRIVATE_KEY must be set")
	}

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=lab-router"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerHealthcheck(cl)(ctx(), mkHealthcheckReq())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !strings.Contains(text, "scp.config_missing") {
		t.Errorf("expected scp.config_missing, got: %s", text)
	}
	if !strings.Contains(text, "Likely issue") || !strings.Contains(text, "SCP configuration is incomplete") {
		t.Errorf("expected Likely issue diagnosis for missing SCP config, got: %s", text)
	}
}

func TestIntegrationHealthcheckAPIAuthFailed(t *testing.T) {
	origResolve := hcResolveSCPPrivateKeyPath
	origLoad := hcLoadFileTransferSettings
	origDownloader := hcNewSCPFileDownloader
	origProbe := hcProbeSSHFingerprint
	defer func() {
		hcResolveSCPPrivateKeyPath = origResolve
		hcLoadFileTransferSettings = origLoad
		hcNewSCPFileDownloader = origDownloader
		hcProbeSSHFingerprint = origProbe
	}()

	testutil.Setenv(t, "MIKROTIK_USER", "api-user")
	testutil.Setenv(t, "MIKROTIK_PASSWORD", "api-pass")
	testutil.Setenv(t, "MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", "SHA256:test-key")
	hcResolveSCPPrivateKeyPath = func() (string, error) { return "/path/to/key", nil }
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22}, nil
	}
	hcNewSCPFileDownloader = func(s *downloads.FileTransferSettings) scpChecker {
		return &mockSCPDownloader{checkResult: map[string]any{"ok": true}}
	}
	hcProbeSSHFingerprint = func(host string, port int, timeout time.Duration) (map[string]any, error) {
		return map[string]any{"status": "ok"}, nil
	}

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!trap", "=message=invalid user name or password (auth)"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerHealthcheck(cl)(ctx(), mkHealthcheckReq())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !strings.Contains(text, "api.auth_failed") {
		t.Errorf("expected api.auth_failed, got: %s", text)
	}
	if !strings.Contains(text, "Likely issue") || !strings.Contains(text, "API authentication failed") {
		t.Errorf("expected Likely issue diagnosis for auth failure, got: %s", text)
	}
}

func TestIntegrationHealthcheckPasswordlessEnabled(t *testing.T) {
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

	testutil.Setenv(t, "MIKROTIK_USER", "api-user")
	testutil.Setenv(t, "MIKROTIK_PASSWORD", "api-pass")
	testutil.Setenv(t, "MIKROTIK_API_PASSWORDLESS_ENABLED", "true")
	testutil.Setenv(t, "MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", "SHA256:test-key")
	testutil.Setenv(t, "MIKROTIK_SCP_PRIVATE_KEY", "/path/to/key")
	hcResolveSCPPrivateKeyPath = func() (string, error) { return "/path/to/key", nil }
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22}, nil
	}
	hcNewSCPFileDownloader = func(s *downloads.FileTransferSettings) scpChecker {
		return &mockSCPDownloader{checkResult: map[string]any{"ok": true}}
	}
	hcProbeSSHFingerprint = func(host string, port int, timeout time.Duration) (map[string]any, error) {
		return map[string]any{"status": "ok"}, nil
	}
	hcCheckPasswordRotationReady = func(host, username string) (map[string]any, error) {
		return map[string]any{"host": host, "port": 22, "target_exists": true}, nil
	}

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=lab-router"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerHealthcheck(cl)(ctx(), mkHealthcheckReq())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !strings.Contains(text, "passwordless.ok") {
		t.Errorf("expected passwordless.ok, got: %s", text)
	}
}

func TestIntegrationHealthcheckPasswordlessExecFailed(t *testing.T) {
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

	testutil.Setenv(t, "MIKROTIK_USER", "api-user")
	testutil.Setenv(t, "MIKROTIK_PASSWORD", "api-pass")
	testutil.Setenv(t, "MIKROTIK_API_PASSWORDLESS_ENABLED", "true")
	testutil.Setenv(t, "MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", "SHA256:test-key")
	testutil.Setenv(t, "MIKROTIK_SCP_PRIVATE_KEY", "/path/to/key")
	hcResolveSCPPrivateKeyPath = func() (string, error) { return "/path/to/key", nil }
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22}, nil
	}
	hcNewSCPFileDownloader = func(s *downloads.FileTransferSettings) scpChecker {
		return &mockSCPDownloader{checkResult: map[string]any{"ok": true}}
	}
	hcProbeSSHFingerprint = func(host string, port int, timeout time.Duration) (map[string]any, error) {
		return map[string]any{"status": "ok"}, nil
	}
	hcCheckPasswordRotationReady = func(host, username string) (map[string]any, error) {
		return nil, fmt.Errorf("ssh command failed")
	}

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=lab-router"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerHealthcheck(cl)(ctx(), mkHealthcheckReq())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !strings.Contains(text, "passwordless.exec_failed") {
		t.Errorf("expected passwordless.exec_failed, got: %s", text)
	}
	if !strings.Contains(text, "Likely issue") || !strings.Contains(text, "Passwordless SSH command execution failed") {
		t.Errorf("expected Likely issue diagnosis for exec failure, got: %s", text)
	}
}

func TestIntegrationHealthcheckFingerprintProbeFailed(t *testing.T) {
	origLoad := hcLoadFileTransferSettings
	origDownloader := hcNewSCPFileDownloader
	origProbe := hcProbeSSHFingerprint
	origResolve := hcResolveSCPPrivateKeyPath
	defer func() {
		hcLoadFileTransferSettings = origLoad
		hcNewSCPFileDownloader = origDownloader
		hcProbeSSHFingerprint = origProbe
		hcResolveSCPPrivateKeyPath = origResolve
	}()

	testutil.Setenv(t, "MIKROTIK_USER", "api-user")
	testutil.Setenv(t, "MIKROTIK_PASSWORD", "api-pass")
	hcResolveSCPPrivateKeyPath = func() (string, error) { return "", nil }
	hcLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22}, nil
	}
	hcNewSCPFileDownloader = func(s *downloads.FileTransferSettings) scpChecker {
		return &mockSCPDownloader{checkResult: map[string]any{"ok": true}}
	}
	hcProbeSSHFingerprint = func(host string, port int, timeout time.Duration) (map[string]any, error) {
		return nil, fmt.Errorf("connection refused")
	}

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=lab-router"), enc("!done"))
	cl.SetConn(fc)

	result, err := handlerHealthcheck(cl)(ctx(), mkHealthcheckReq())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !strings.Contains(text, "connection refused") {
		t.Errorf("expected fingerprint probe failure message, got: %s", text)
	}
}
