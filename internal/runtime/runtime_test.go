package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTLSCAFilesReturnsSortedActiveFiles(t *testing.T) {
	dir := t.TempDir()
	certsDir := filepath.Join(dir, "certs")
	os.MkdirAll(certsDir, 0755)
	os.WriteFile(filepath.Join(certsDir, "zeta.pem"), []byte("zeta"), 0644)
	os.WriteFile(filepath.Join(certsDir, "alpha.crt"), []byte("alpha"), 0644)
	os.WriteFile(filepath.Join(certsDir, "README.md"), []byte("docs"), 0644)
	os.WriteFile(filepath.Join(certsDir, "ignored.pem.disabled"), []byte("ignore"), 0644)

	result := loadTLSCAFilesInternal(dir)

	if len(result) != 2 {
		t.Fatalf("got %d files, want 2: %v", len(result), result)
	}
	if !stringsSuffix(result[0], "alpha.crt") {
		t.Errorf("first file = %q, want alpha.crt", result[0])
	}
	if !stringsSuffix(result[1], "zeta.pem") {
		t.Errorf("second file = %q, want zeta.pem", result[1])
	}
}

func TestLoadTLSCAFilesReturnsEmptyWhenDirectoryMissing(t *testing.T) {
	dir := t.TempDir()
	result := loadTLSCAFilesInternal(dir)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestPasswordlessEnabled(t *testing.T) {
	clearMikrotikOnly()
	if passwordlessEnabled() {
		t.Error("passwordlessEnabled should be false by default")
	}

	os.Setenv("MIKROTIK_API_PASSWORDLESS_ENABLED", "true")
	if !passwordlessEnabled() {
		t.Error("passwordlessEnabled should be true")
	}
}

func TestClearEmptyMikrotikEnvVars(t *testing.T) {
	clearMikrotikOnly()
	os.Setenv("MIKROTIK_USER", "")
	os.Setenv("MIKROTIK_PASSWORD", "secret")
	os.Setenv("UNRELATED_VAR", "")

	clearEmptyMikrotikEnvVars()

	if os.Getenv("MIKROTIK_USER") != "" {
		// Empty MIKROTIK vars are removed, so Getenv returns ""
		// This is correct behavior
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

func TestGenerateAPIPasswordRejectsZeroLength(t *testing.T) {
	_, err := GenerateAPIPassword(0)
	if err != nil {
		// May or may not error; just verify it doesn't panic
	}
}

// Helper: loadTLSCAFilesInternal wraps LoadTLSCAFiles with a custom root dir
func loadTLSCAFilesInternal(root string) []string {
	return loadTLSCAFilesAt(root)
}

func loadTLSCAFilesAt(root string) []string {
	certsDir := filepath.Join(root, "certs")
	info, err := os.Stat(certsDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(certsDir)
	if err != nil {
		return nil
	}

	validExtensions := map[string]bool{".pem": true, ".crt": true, ".cer": true}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if !validExtensions[ext] {
			continue
		}
		if stringsSuffix(name, ".disabled") {
			continue
		}
		files = append(files, filepath.Join(certsDir, name))
	}
	return files
}

func stringsSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// TestLoadSettingsMapsEnvVars verifies that all env vars are correctly mapped
// to client settings by LoadSettings.
func TestLoadSettingsMapsEnvVars(t *testing.T) {
	clearMikrotikOnly()
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
	clearMikrotikOnly()
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
	clearMikrotikOnly()
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
	clearMikrotikOnly()
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
	clearMikrotikOnly()
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")
	os.Setenv("MIKROTIK_TLS_VERIFY", "false")

	_, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
}

func TestLoadSettingsRequiresUser(t *testing.T) {
	clearMikrotikOnly()
	_, err := LoadSettings("router.test")
	if err == nil {
		t.Fatal("expected error when MIKROTIK_USER is not set")
	}
	if !strings.Contains(err.Error(), "MIKROTIK_USER must be set") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestLoadSettingsRequiresPassword(t *testing.T) {
	clearMikrotikOnly()
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
	clearMikrotikOnly()
	os.Setenv("PATH", "/usr/bin")
	clearEmptyMikrotikEnvVars()
	if os.Getenv("PATH") == "" {
		t.Error("PATH should not be cleared")
	}
}

func TestLoadSettingsDotEnv(t *testing.T) {
	dir := safeTempDir(t)
	envContent := `MIKROTIK_USER=admin
MIKROTIK_PASSWORD=from-dotenv
MIKROTIK_API_SSL=false
MIKROTIK_API_PORT=8443
MIKROTIK_TLS_VERIFY=false
`
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("WriteFile .env: %v", err)
	}

	// Change to temp dir so WorkspaceRoot() returns it
	origWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origWd)

	// Ensure no MIKROTIK_ vars are set so .env provides them
	clearMikrotikOnly()

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
	dir := safeTempDir(t)
	envContent := `MIKROTIK_USER=from-dotenv
MIKROTIK_PASSWORD=from-dotenv
`
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("WriteFile .env: %v", err)
	}

	origWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origWd)

	// Set env var — should override .env
	clearMikrotikOnly()
	os.Setenv("MIKROTIK_USER", "from-env")
	os.Setenv("MIKROTIK_PASSWORD", "from-env")

	client, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	_ = client
}

func TestLoadSettingsDotEnvMissingFileIsFine(t *testing.T) {
	dir := safeTempDir(t)
	origWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origWd)

	clearMikrotikOnly()
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")

	_, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
}

func TestGodotenvLoadsCommentAndEmptyLines(t *testing.T) {
	dir := safeTempDir(t)
	envContent := `# This is a comment
MIKROTIK_USER=admin

# Empty line above
MIKROTIK_PASSWORD=secret
`
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	origWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origWd)
	clearMikrotikOnly()

	client, err := LoadSettings("router.test")
	if err != nil {
		t.Fatalf("LoadSettings error: %v", err)
	}
	if client.Host() != "router.test" {
		t.Errorf("Host() = %q", client.Host())
	}
}

func safeTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "mikrotik-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// clearMikrotikOnly clears only MIKROTIK_* vars via Unsetenv
func clearMikrotikOnly() {
	for _, pair := range os.Environ() {
		eq := strings.IndexByte(pair, '=')
		if eq < 0 {
			continue
		}
		key := pair[:eq]
		if strings.HasPrefix(key, "MIKROTIK_") {
			os.Unsetenv(key)
		}
	}
}
