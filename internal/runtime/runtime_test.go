package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
)

func TestLoadTLSCAFilesReturnsSortedActiveFiles(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")
	os.MkdirAll(certsDir, 0755)
	os.WriteFile(filepath.Join(certsDir, "zeta.pem"), []byte("zeta"), 0644)
	os.WriteFile(filepath.Join(certsDir, "alpha.crt"), []byte("alpha"), 0644)
	os.WriteFile(filepath.Join(certsDir, "README.md"), []byte("docs"), 0644)
	os.WriteFile(filepath.Join(certsDir, "ignored.pem.disabled"), []byte("ignore"), 0644)

	orig := workspaceRoot
	workspaceRoot = func() string { return dir }
	defer func() { workspaceRoot = orig }()

	result := LoadTLSCAFiles()

	if len(result) != 2 {
		t.Fatalf("got %d files, want 2: %v", len(result), result)
	}
	if !strings.HasSuffix(result[0], "alpha.crt") {
		t.Errorf("first file = %q, want alpha.crt", result[0])
	}
	if !strings.HasSuffix(result[1], "zeta.pem") {
		t.Errorf("second file = %q, want zeta.pem", result[1])
	}
}

func TestLoadTLSCAFilesReturnsEmptyWhenDirectoryMissing(t *testing.T) {
	dir := t.TempDir()
	orig := workspaceRoot
	workspaceRoot = func() string { return dir }
	defer func() { workspaceRoot = orig }()

	result := LoadTLSCAFiles()
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestLoadTLSCAFilesCaseInsensitiveExtension(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")
	os.MkdirAll(certsDir, 0755)
	os.WriteFile(filepath.Join(certsDir, "server.CRT"), []byte("server"), 0644)
	os.WriteFile(filepath.Join(certsDir, "ca.PEM"), []byte("ca"), 0644)

	orig := workspaceRoot
	workspaceRoot = func() string { return dir }
	defer func() { workspaceRoot = orig }()

	result := LoadTLSCAFiles()
	if len(result) != 2 {
		t.Fatalf("got %d files, want 2: %v", len(result), result)
	}
}

func TestPasswordlessEnabled(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	if passwordlessEnabled() {
		t.Error("passwordlessEnabled should be false by default")
	}

	os.Setenv("MIKROTIK_API_PASSWORDLESS_ENABLED", "true")
	if !passwordlessEnabled() {
		t.Error("passwordlessEnabled should be true")
	}
}

func TestClearEmptyMikrotikEnvVars(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "")
	os.Setenv("MIKROTIK_PASSWORD", "secret")
	os.Setenv("UNRELATED_VAR", "")

	clearEmptyMikrotikEnvVars()

	if _, exists := os.LookupEnv("MIKROTIK_USER"); exists {
		t.Error("MIKROTIK_USER should be unset after clearEmptyMikrotikEnvVars")
	}
	if v := os.Getenv("MIKROTIK_PASSWORD"); v != "secret" {
		t.Errorf("MIKROTIK_PASSWORD = %q", v)
	}
	if v := os.Getenv("UNRELATED_VAR"); v != "" {
		t.Errorf("UNRELATED_VAR = %q", v)
	}
}

func TestGenerateAPIPassword(t *testing.T) {
	pwd, err := GenerateAPIPassword(32)
	if err != nil {
		t.Fatalf("GenerateAPIPassword error: %v", err)
	}
	if len(pwd) != 32 {
		t.Errorf("length = %d, want 32", len(pwd))
	}
}



// TestLoadSettingsMapsEnvVars verifies that all env vars are correctly mapped
// to client settings by LoadSettings.
func TestLoadSettingsMapsEnvVars(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret123")
	os.Setenv("MIKROTIK_API_SSL", "true")
	os.Setenv("MIKROTIK_API_PORT", "8729")
	os.Setenv("MIKROTIK_TLS_VERIFY", "true")

	client, err := LoadSettings("192.168.88.1")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if host := client.Host(); host != "192.168.88.1" {
		t.Errorf("Host() = %q", host)
	}
	if port := client.Port(); port != 8729 {
		t.Errorf("Port() = %d, want 8729", port)
	}
	if ssl := client.UseSSL(); ssl != true {
		t.Errorf("UseSSL() = %v", ssl)
	}
}

func TestLoadSettingsDefaults(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")

	client, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if port := client.Port(); port != 8729 {
		t.Errorf("Port() = %d, want 8729 (TLS default)", port)
	}
	if ssl := client.UseSSL(); ssl != true {
		t.Errorf("UseSSL() = %v, want true (SSL default)", ssl)
	}
}

func TestLoadSettingsPlainText(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")
	os.Setenv("MIKROTIK_API_SSL", "false")

	client, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if port := client.Port(); port != 8728 {
		t.Errorf("Port() = %d, want 8728 (non-TLS)", port)
	}
	if ssl := client.UseSSL(); ssl != false {
		t.Errorf("UseSSL() = %v", ssl)
	}
}

func TestLoadSettingsCustomPort(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")
	os.Setenv("MIKROTIK_API_PORT", "9999")

	client, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if port := client.Port(); port != 9999 {
		t.Errorf("Port() = %d, want 9999", port)
	}
}

func TestLoadSettingsTLSVerifyDisabled(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")
	os.Setenv("MIKROTIK_TLS_VERIFY", "false")

	_, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
}

func TestLoadSettingsRequiresUser(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	_, err := LoadSettings("router.test")
	if err == nil {
		t.Fatal("expected error when MIKROTIK_USER is not set")
	}
	if !strings.Contains(err.Error(), "MIKROTIK_USER must be set") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestLoadSettingsRequiresPassword(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	_, err := LoadSettings("router.test")
	if err == nil {
		t.Fatal("expected error when MIKROTIK_PASSWORD is not set")
	}
	if !strings.Contains(err.Error(), "MIKROTIK_USER and MIKROTIK_PASSWORD") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestClearEmptyMikrotikEnvVarsSkipsNonMikrotik(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("PATH", "/usr/bin")
	clearEmptyMikrotikEnvVars()
	if os.Getenv("PATH") == "" {
		t.Error("PATH should not be cleared")
	}
}

func TestLoadSettingsDotEnv(t *testing.T) {
	dir := t.TempDir()
	envContent := `MIKROTIK_USER=admin
MIKROTIK_PASSWORD=from-dotenv
MIKROTIK_API_SSL=false
MIKROTIK_API_PORT=8443
MIKROTIK_TLS_VERIFY=false
`
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("WriteFile .env: %v", err)
	}

	orig := workspaceRoot
	workspaceRoot = func() string { return dir }
	defer func() { workspaceRoot = orig }()

	testutil.ClearMikrotikEnv(t)

	client, err := LoadSettings("192.168.88.1")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if client.UseSSL() != false {
		t.Errorf("UseSSL() = %v, want false", client.UseSSL())
	}
	if client.Port() != 8443 {
		t.Errorf("Port() = %d, want 8443", client.Port())
	}
}

func TestLoadSettingsDotEnvDoesNotOverrideEnv(t *testing.T) {
	dir := t.TempDir()
	envContent := `MIKROTIK_USER=from-dotenv
MIKROTIK_PASSWORD=from-dotenv
`
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("WriteFile .env: %v", err)
	}

	orig := workspaceRoot
	workspaceRoot = func() string { return dir }
	defer func() { workspaceRoot = orig }()

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "from-env")
	os.Setenv("MIKROTIK_PASSWORD", "from-env")

	_, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
}

func TestLoadSettingsDotEnvMissingFileIsFine(t *testing.T) {
	dir := t.TempDir()
	orig := workspaceRoot
	workspaceRoot = func() string { return dir }
	defer func() { workspaceRoot = orig }()

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")

	_, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
}

func TestGodotenvLoadsCommentAndEmptyLines(t *testing.T) {
	dir := t.TempDir()
	envContent := `# This is a comment
MIKROTIK_USER=admin

# Empty line above
MIKROTIK_PASSWORD=secret
`
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	orig := workspaceRoot
	workspaceRoot = func() string { return dir }
	defer func() { workspaceRoot = orig }()
	testutil.ClearMikrotikEnv(t)

	client, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if client.Host() != "router.test" {
		t.Errorf("Host() = %q", client.Host())
	}
}

func TestLoadSettingsPassesDiscoveredTLSCAFiles(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")
	os.MkdirAll(certsDir, 0755)
	os.WriteFile(filepath.Join(certsDir, "ca.pem"), []byte("ca"), 0644)

	orig := workspaceRoot
	workspaceRoot = func() string { return dir }
	defer func() { workspaceRoot = orig }()

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")

	cl, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if cl == nil {
		t.Fatal("LoadSettings returned nil client")
	}
	if cl.Host() != "router.test" {
		t.Errorf("Host() = %q, want router.test", cl.Host())
	}
}

func TestGenerateAPIPasswordRejectsZeroLength(t *testing.T) {
	_, err := GenerateAPIPassword(0)
	if err == nil {
		t.Error("expected error for length 0")
	}
}

func TestLoadSettingsRotatesPasswordWhenPasswordlessEnabled(t *testing.T) {
	origRotate := rotateStartupAPIPassword
	rotateStartupAPIPassword = func(host, username string) (string, error) {
		return "rotated-api-password", nil
	}
	defer func() { rotateStartupAPIPassword = origRotate }()

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_API_PASSWORDLESS_ENABLED", "true")
	os.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", "SHA256:valid-key")

	cl, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if cl == nil {
		t.Fatal("LoadSettings returned nil client")
	}
	if state := startupPasswordlessState(); state["status"] != "" {
		t.Errorf("passwordless state should be cleared after successful rotation, got: %v", state)
	}
}

func TestLoadSettingsRequiresFingerprintWhenPasswordlessEnabled(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_API_PASSWORDLESS_ENABLED", "true")

	_, err := LoadSettings("router.test")
	if err == nil {
		t.Fatal("expected error when fingerprint is missing")
	}
	if !strings.Contains(err.Error(), "MIKROTIK_SCP_HOST_FINGERPRINT_SHA256 must be set") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestLoadSettingsRaisesWhenStartupRotationFails(t *testing.T) {
	origRotate := rotateStartupAPIPassword
	rotateStartupAPIPassword = func(host, username string) (string, error) {
		return "", fmt.Errorf("boom")
	}
	defer func() { rotateStartupAPIPassword = origRotate }()

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_API_PASSWORDLESS_ENABLED", "true")
	os.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", "SHA256:valid-key")

	_, err := LoadSettings("router.test")
	if err == nil {
		t.Fatal("expected error when rotation fails")
	}
	if !strings.Contains(err.Error(), "startup password rotation failed") {
		t.Errorf("error should wrap rotation failure: %q", err.Error())
	}
}

func TestRotateStartupAPIPasswordUsesRequestedLength(t *testing.T) {
	origRotate := rotateStartupAPIPassword
	var gotHost, gotUser string
	rotateStartupAPIPassword = func(host, username string) (string, error) {
		gotHost, gotUser = host, username
		return "fixed-password", nil
	}
	defer func() { rotateStartupAPIPassword = origRotate }()

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_API_PASSWORDLESS_ENABLED", "true")
	os.Setenv("MIKROTIK_API_PASSWORDLESS_LENGTH", "40")
	os.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", "SHA256:valid-key")

	cl, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if cl == nil {
		t.Fatal("LoadSettings returned nil client")
	}
	if gotHost != "router.test" || gotUser != "admin" {
		t.Errorf("rotation called with host=%q user=%q, want router.test/admin", gotHost, gotUser)
	}
}


