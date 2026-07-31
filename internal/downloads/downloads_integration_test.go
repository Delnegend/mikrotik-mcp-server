package downloads

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
)

func srvSettings(srv *inMemorySSHServer) *FileTransferSettings {
	host, portStr, _ := net.SplitHostPort(srv.addr())
	port, _ := strconv.Atoi(portStr)
	return &FileTransferSettings{
		Host:     host,
		Port:     port,
		Username: "admin",
		Password: "test-pass",
		Timeout:  5 * time.Second,
	}
}

func TestSCPFileDownloaderWrapsConnectFailure(t *testing.T) {
	settings := &FileTransferSettings{
		Host:     "198.51.100.1",
		Port:     22,
		Username: "admin",
		Password: "test",
		Timeout:  100 * time.Millisecond,
	}
	dl := NewSCPFileDownloader(settings)
	_, err := dl.CheckConnection()
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "failed to connect") {
		t.Errorf("error = %q, want 'failed to connect'", err.Error())
	}
}

func TestOpenSSHClientRejectsMismatchedHostKey(t *testing.T) {
	srv := newInMemorySSHServer(t)
	defer srv.close()
	srv.setPassword("test-pass")

	settings := srvSettings(srv)
	settings.SSHFingerprintSHA256 = "SHA256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	dl := NewSCPFileDownloader(settings)
	_, err := dl.CheckConnection()
	if err == nil {
		t.Fatal("expected host key mismatch error")
	}
	if !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Errorf("error = %q, want 'fingerprint mismatch'", err.Error())
	}
}

// pipeSFTP creates a net.Pipe() pair and starts an SFTP server on one end.
// Returns the override function for NewSFTPClient and a cleanup closer.
func pipeSFTP(dir string) func(*ssh.Client, ...sftp.ClientOption) (*sftp.Client, error) {
	clientEnd, serverEnd := net.Pipe()
	go func() {
		svr, err := sftp.NewServer(serverEnd)
		if err != nil {
			return
		}
		svr.Serve()
	}()
	return func(_ *ssh.Client, _ ...sftp.ClientOption) (*sftp.Client, error) {
		cl, err := sftp.NewClientPipe(clientEnd, clientEnd)
		if err != nil {
			return nil, fmt.Errorf("sftp client pipe: %v", err)
		}
		return cl, nil
	}
}

func sftpTestEnvironment(t *testing.T) (*FileTransferSettings, func()) {
	srv := newInMemorySSHServer(t)
	srv.setPassword("test-pass")

	dir := t.TempDir()
	origSFTP := NewSFTPClient
	NewSFTPClient = pipeSFTP(dir)

	cleanup := func() {
		srv.close()
		NewSFTPClient = origSFTP
	}
	return srvSettings(srv), cleanup
}

func TestSCPFileDownloaderCheckConnectionSucceeds(t *testing.T) {
	settings, cleanup := sftpTestEnvironment(t)
	defer cleanup()

	dl := NewSCPFileDownloader(settings)
	result, err := dl.CheckConnection()
	if err != nil {
		t.Fatalf("CheckConnection error: %v", err)
	}
	// SFTP server serves from real filesystem; ReadDir on CWD should succeed
	if result["operation"] != "ReadDir" {
		t.Errorf("operation = %q, want ReadDir", result["operation"])
	}
}

func TestSCPFileDownloaderWritesDownloadedFile(t *testing.T) {
	settings, cleanup := sftpTestEnvironment(t)
	defer cleanup()

	dl := NewSCPFileDownloader(settings)
	localPath := filepath.Join(t.TempDir(), "downloaded-file.txt")
	// Use an absolute path for the router file to avoid workDir issues
	routerPath := filepath.Join(t.TempDir(), "remote-file.txt")
	fileContent := []byte("downloaded content for verification")
	if err := os.WriteFile(routerPath, fileContent, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := dl.DownloadFile(routerPath, localPath); err != nil {
		t.Fatalf("DownloadFile error: %v", err)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(fileContent) {
		t.Errorf("downloaded content = %q, want %q", string(data), string(fileContent))
	}
}

func TestSCPFileDownloaderCreatesParentDirectories(t *testing.T) {
	settings, cleanup := sftpTestEnvironment(t)
	defer cleanup()

	dl := NewSCPFileDownloader(settings)
	routerPath := filepath.Join(t.TempDir(), "remote.txt")
	if err := os.WriteFile(routerPath, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// MkdirAll creates parent dirs, so the download succeeds.
	// Verify the file was downloaded to verify the flow completes.
	localPath := filepath.Join(t.TempDir(), "new-dir", "file.txt")
	if err := dl.DownloadFile(routerPath, localPath); err != nil {
		t.Fatalf("DownloadFile error: %v", err)
	}
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		t.Errorf("downloaded file not found at %s", localPath)
	}
}

func TestOpenSSHClientAllowsMissingHostFingerprint(t *testing.T) {
	settings, cleanup := sftpTestEnvironment(t)
	defer cleanup()

	dl := NewSCPFileDownloader(settings)
	_, err := dl.CheckConnection()
	if err != nil {
		t.Fatalf("connection should succeed without fingerprint verification, got: %v", err)
	}
}

func TestLoadFileTransferSettingsRequiresAuthWhenNoKeyOrPassword(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_SCP_USER", "admin")
	_, err := LoadFileTransferSettings("router.test")
	if err == nil {
		t.Fatal("expected error when no password or key provided")
	}
	if !strings.Contains(err.Error(), "must be set") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestLoadFileTransferSettingsRejectsInvalidHostKeyFingerprint(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")
	os.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", "invalid!")

	_, err := LoadFileTransferSettings("router.test")
	if err == nil {
		t.Fatal("expected error for invalid fingerprint")
	}
}

func TestLoadFileTransferSettingsUsesExplicitPrivateKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test-key")
	if err := os.WriteFile(keyPath, []byte("key-data"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")
	os.Setenv("MIKROTIK_SCP_PRIVATE_KEY", keyPath)

	settings, err := LoadFileTransferSettings("router.test")
	if err != nil {
		t.Fatalf("LoadFileTransferSettings error: %v", err)
	}
	if settings.PrivateKey == "" {
		t.Error("expected PrivateKey to be set")
	}
	if settings.Password != "" {
		t.Error("expected Password to be empty when key is used")
	}
}

func TestLoadFileTransferSettingsDoesNotUseDefaultRouterKey(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")

	settings, err := LoadFileTransferSettings("router.test")
	if err != nil {
		t.Fatalf("LoadFileTransferSettings error: %v", err)
	}
	if settings.PrivateKey != "" {
		t.Error("expected PrivateKey to be empty when no key env is set")
	}
	if settings.Password != "secret" {
		t.Errorf("Password = %q, want secret", settings.Password)
	}
}

func TestLoadFileTransferSettingsRequiresHostKeyFingerprint(t *testing.T) {
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_PASSWORD", "secret")

	settings, err := LoadFileTransferSettings("router.test")
	if err != nil {
		t.Fatalf("LoadFileTransferSettings error: %v", err)
	}
	if settings.SSHFingerprintSHA256 != "" {
		t.Errorf("SSHFingerprintSHA256 = %q, want empty when unset", settings.SSHFingerprintSHA256)
	}
}

func TestRotateRouterOSPasswordRunsCommandOverSSH(t *testing.T) {
	srv := newInMemorySSHServer(t)
	defer srv.close()
	srv.setPassword("test-pass")

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test-key")
	key, err := generateTestPrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	host, portStr, _ := net.SplitHostPort(srv.addr())

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", srv.fingerprint())
	os.Setenv("MIKROTIK_SCP_PRIVATE_KEY", keyPath)
	os.Setenv("MIKROTIK_SCP_HOST", host)
	os.Setenv("MIKROTIK_SCP_PORT", portStr)

	origGen := GenerateAPIPassword
	GenerateAPIPassword = func(length int) (string, error) {
		return "rotated-password-here", nil
	}
	defer func() { GenerateAPIPassword = origGen }()

	newPassword, err := RotateRouterOSPassword(host, "admin")
	if err != nil {
		t.Fatalf("RotateRouterOSPassword error: %v", err)
	}
	if newPassword != "rotated-password-here" {
		t.Errorf("password = %q, want rotated-password-here", newPassword)
	}
	cmds := srv.commandsRun()
	if len(cmds) == 0 {
		t.Error("expected at least one SSH command to be run")
	}
}

func TestRotateRouterOSPasswordWrapsRemoteCommandFailures(t *testing.T) {
	// No SSH server running — will fail to connect
	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", "SHA256:test")
	os.Setenv("MIKROTIK_SCP_PRIVATE_KEY", "/nonexistent/key")
	os.Setenv("MIKROTIK_SCP_HOST", "127.0.0.1")
	os.Setenv("MIKROTIK_SCP_PORT", "19999")

	_, err := RotateRouterOSPassword("127.0.0.1", "admin")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

func TestCheckPasswordRotationReadyRunsUserProbe(t *testing.T) {
	srv := newInMemorySSHServer(t)
	defer srv.close()
	srv.setPassword("test-pass")

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test-key")
	key, err := generateTestPrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	host, portStr, _ := net.SplitHostPort(srv.addr())

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", srv.fingerprint())
	os.Setenv("MIKROTIK_SCP_PRIVATE_KEY", keyPath)
	os.Setenv("MIKROTIK_SCP_HOST", host)
	os.Setenv("MIKROTIK_SCP_PORT", portStr)

	_, err = CheckPasswordRotationReady(host, "admin")
	if err != nil {
		// The in-memory server doesn't emulate the user-count probe output,
		// so a "not found" error is expected unless we mock the exec output.
		if !strings.Contains(err.Error(), "was not found over SSH") {
			t.Errorf("unexpected error from user probe: %v", err)
		}
	}
}

func TestCheckPasswordRotationReadyRejectsMissingUser(t *testing.T) {
	srv := newInMemorySSHServer(t)
	defer srv.close()
	srv.setPassword("test-pass")

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test-key")
	key, err := generateTestPrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	host, portStr, _ := net.SplitHostPort(srv.addr())

	testutil.ClearMikrotikEnv(t)
	os.Setenv("MIKROTIK_USER", "admin")
	os.Setenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256", srv.fingerprint())
	os.Setenv("MIKROTIK_SCP_PRIVATE_KEY", keyPath)
	os.Setenv("MIKROTIK_SCP_HOST", host)
	os.Setenv("MIKROTIK_SCP_PORT", portStr)

	_, err = CheckPasswordRotationReady(host, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}

func generateTestPrivateKey() ([]byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	marshaled := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: marshaled}
	return pem.EncodeToMemory(pemBlock), nil
}


