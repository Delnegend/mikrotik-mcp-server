package downloads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
)

func TestNormalizeSSHFingerprint(t *testing.T) {
	// SHA256:NcOAVsJfVFKzDYKROAuSx8N0ODtJuwPgdFDMbOo5AeQ
	input := "SHA256:NcOAVsJfVFKzDYKROAuSx8N0ODtJuwPgdFDMbOo5AeQ"
	result, err := NormalizeSSHFingerprint(input)
	if err != nil {
		t.Fatalf("NormalizeSSHFingerprint error: %v", err)
	}
	if result != input {
		t.Errorf("result = %q, want %q", result, input)
	}
}

func TestNormalizeSSHFingerprintWithoutPrefix(t *testing.T) {
	input := "NcOAVsJfVFKzDYKROAuSx8N0ODtJuwPgdFDMbOo5AeQ"
	result, err := NormalizeSSHFingerprint(input)
	if err != nil {
		t.Fatalf("NormalizeSSHFingerprint error: %v", err)
	}
	expected := "SHA256:NcOAVsJfVFKzDYKROAuSx8N0ODtJuwPgdFDMbOo5AeQ"
	if result != expected {
		t.Errorf("result = %q, want %q", result, expected)
	}
}

func TestNormalizeSSHFingerprintRejectsEmpty(t *testing.T) {
	_, err := NormalizeSSHFingerprint("")
	if err == nil {
		t.Error("expected error for empty fingerprint")
	}
}

func TestNormalizeSSHFingerprintRejectsInvalid(t *testing.T) {
	_, err := NormalizeSSHFingerprint("not-a-valid-base64!!")
	if err == nil {
		t.Error("expected error for invalid fingerprint")
	}
}

func TestLoadFileTransferSettingsFallsBackToAPICredentials(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")
	os.Setenv("MIKROTIK_API_SSL", "true")

	settings, err := LoadFileTransferSettings("router.test")
	if err != nil {
		t.Fatalf("LoadFileTransferSettings error: %v", err)
	}
	if settings.Host != "router.test" {
		t.Errorf("host = %q", settings.Host)
	}
	if settings.Username != "admin" {
		t.Errorf("username = %q", settings.Username)
	}
	if settings.Password != "secret" {
		t.Errorf("password = %q", settings.Password)
	}
}

func TestLoadFileTransferSettingsUsesSCPOverrides(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "api-user")
	os.Setenv("MIKROTIK_PASSWORD", "api-pass")
	os.Setenv("MIKROTIK_SCP_HOST", "files.router.test")
	os.Setenv("MIKROTIK_SCP_USER", "scp-user")
	os.Setenv("MIKROTIK_SCP_PASSWORD", "scp-pass")
	os.Setenv("MIKROTIK_SCP_PORT", "2222")
	os.Setenv("MIKROTIK_SCP_TIMEOUT", "12.5")

	settings, err := LoadFileTransferSettings("router.test")
	if err != nil {
		t.Fatalf("LoadFileTransferSettings error: %v", err)
	}
	if settings.Host != "files.router.test" {
		t.Errorf("host = %q", settings.Host)
	}
	if settings.Username != "scp-user" {
		t.Errorf("username = %q", settings.Username)
	}
	if settings.Password != "scp-pass" {
		t.Errorf("password = %q", settings.Password)
	}
	if settings.Port != 2222 {
		t.Errorf("port = %d", settings.Port)
	}
	if settings.Timeout != 12500000000 {
		t.Errorf("timeout = %v", settings.Timeout)
	}
}

func TestLoadFileTransferSettingsRequiresCredentials(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_SCP_USER", "mcprw")

	_, err := LoadFileTransferSettings("router.test")
	if err == nil {
		t.Error("expected error when no password or key is provided")
	}
}

func TestResolveSCPPrivateKeyPath(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "mikrotik-test-key")
	if err := os.WriteFile(keyPath, []byte("private-key-content"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	defer os.Remove(keyPath)

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_SCP_PRIVATE_KEY", keyPath)

	result, err := ResolveSCPPrivateKeyPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != keyPath {
		t.Errorf("result = %q, want %q", result, keyPath)
	}
}

func TestResolveSCPPrivateKeyPathMissing(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_SCP_PRIVATE_KEY", "/nonexistent/path/key")

	_, err := ResolveSCPPrivateKeyPath()
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
	if !strings.Contains(err.Error(), "is inaccessible") {
		t.Errorf("expected 'is inaccessible' error, got: %v", err)
	}
}

func TestResolveSCPPrivateKeyPathUnset(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	result, err := ResolveSCPPrivateKeyPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty when unset, got %q", result)
	}
}

func TestLoadPasswordRotationSettingsRequiresPrivateKey(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")

	_, err := LoadPasswordRotationSettings("router.test")
	if err == nil {
		t.Error("expected error when MIKROTIK_SCP_PRIVATE_KEY is not set")
	}
}

func TestGenerateAPIPasswordProducesCorrectLength(t *testing.T) {
	password, err := generateAPIPassword(32)
	if err != nil {
		t.Fatalf("generateAPIPassword error: %v", err)
	}
	if len(password) != 32 {
		t.Errorf("password length = %d, want 32", len(password))
	}
}

func TestGenerateAPIPasswordIsRandom(t *testing.T) {
	p1, _ := generateAPIPassword(16)
	p2, _ := generateAPIPassword(16)
	if p1 == p2 {
		t.Error("two generated passwords should be different")
	}
}

func TestShellQuote(t *testing.T) {
	result := shellQuote("admin")
	if result != "'admin'" {
		t.Errorf("shellQuote = %q", result)
	}
	result = shellQuote("it's/ok")
	if result != "'it'\\''s/ok'" {
		t.Errorf("shellQuote with apostrophe = %q", result)
	}
}
