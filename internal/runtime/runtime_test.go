package runtime

import (
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

	t.Chdir(dir)

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
	t.Chdir(dir)

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

	t.Chdir(dir)

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

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	content := "# comment\nMIKROTIK_USER=admin\nMIKROTIK_PASSWORD=\n  MIKROTIK_API_PORT = \"8443\"  \n"
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_PASSWORD", "from-env")

	loadEnvFile(envFile)

	if v := os.Getenv("MIKROTIK_USER"); v != "admin" {
		t.Errorf("MIKROTIK_USER = %q, want admin", v)
	}
	if v := os.Getenv("MIKROTIK_PASSWORD"); v != "from-env" {
		t.Errorf("MIKROTIK_PASSWORD = %q, want from-env (env must not be overridden)", v)
	}
	if v := os.Getenv("MIKROTIK_API_PORT"); v != "8443" {
		t.Errorf("MIKROTIK_API_PORT = %q, want 8443", v)
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

	t.Chdir(dir)

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

	t.Chdir(dir)

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
	t.Chdir(dir)

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

	t.Chdir(dir)
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

	t.Chdir(dir)

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

func TestLoadRegistrySingleDeviceFallback(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")
	os.Setenv("MIKROTIK_API_SSL", "false")

	reg, err := LoadRegistry("router.test")
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if reg.Len() != 1 {
		t.Fatalf("Len = %d, want 1", reg.Len())
	}
	d, err := reg.Get("")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Host != "router.test" || d.Username != "admin" || d.Password != "secret" || d.Port != 8728 {
		t.Errorf("device = %+v", d)
	}
	if d.APISSL {
		t.Errorf("api_ssl should be false")
	}
}

func TestLoadRegistryInventoryWins(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "flat-user")
	os.Setenv("MIKROTIK_PASSWORD", "flat-pass")
	os.Setenv("MIKROTIK_INVENTORY", `[{"title":"A","host":"10.0.0.1"},{"title":"B","host":"10.0.0.2"}]`)

	reg, err := LoadRegistry("ignored-host")
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if reg.Len() != 2 {
		t.Fatalf("Len = %d, want 2", reg.Len())
	}
	a, err := reg.Get("A")
	if err != nil {
		t.Fatal(err)
	}
	if a.Username != "admin" {
		t.Errorf("inventory username default = %q", a.Username)
	}
}

func TestLoadRegistryRejectsBrokenInventory(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_INVENTORY", `[{bad json]`)

	if _, err := LoadRegistry("ignored-host"); err == nil {
		t.Fatal("expected error for broken inventory")
	}
}
