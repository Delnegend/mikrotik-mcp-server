package downloads

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/Delnegend/mikrotik-mcp/internal/helpers"
)

var SSHDial = ssh.Dial
var NewSFTPClient = sftp.NewClient

var (
	ErrSCPConfigMissing = errors.New("scp: configuration is incomplete")
	ErrSCPAuthFailed    = errors.New("scp: authentication failed")
	ErrSCPConnectFailed = errors.New("scp: connect failed")
	ErrSCPOperation     = errors.New("scp: operation failed")
)

type FileTransferSettings struct {
	Host                 string
	Username             string
	Password             string
	PrivateKey           string
	KeyPassphrase        string
	SSHFingerprintSHA256 string
	Port                 int
	Timeout              time.Duration
	Insecure             bool
}

type SCPFileDownloader struct {
	settings *FileTransferSettings
}

func NewSCPFileDownloader(settings *FileTransferSettings) *SCPFileDownloader {
	return &SCPFileDownloader{settings: settings}
}

func (d *SCPFileDownloader) CheckConnection() (map[string]any, error) {
	sshClient, err := d.connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SCP service on %s:%d: %w",
			d.settings.Host, d.settings.Port, err)
	}
	defer sshClient.Close()

	sftpClient, err := NewSFTPClient(sshClient)
	if err != nil {
		return nil, fmt.Errorf("%w: connected to SCP service but directory probe failed: %v", ErrSCPOperation, err)
	}
	defer sftpClient.Close()

	wd, err := sftpClient.Getwd()
	if err != nil {
		wd = "/"
	}
	entries, err := sftpClient.ReadDir(wd)
	if err != nil {
		return nil, fmt.Errorf("%w: connected to SCP service but directory probe failed: %v", ErrSCPOperation, err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sample := names
	if len(sample) > 5 {
		sample = sample[:5]
	}

	return map[string]any{
		"working_directory": wd,
		"listing_count":     len(names),
		"listing_sample":    sample,
		"operation":         "ReadDir",
	}, nil
}

func (d *SCPFileDownloader) DownloadFile(routerPath, localPath string) error {
	remoteName := strings.TrimLeft(routerPath, "/")
	targetDir := filepath.Dir(localPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %v", err)
	}

	sshClient, err := d.connect()
	if err != nil {
		return fmt.Errorf("failed to connect to SCP service on %s:%d: %w",
			d.settings.Host, d.settings.Port, err)
	}
	defer sshClient.Close()

	sftpClient, err := NewSFTPClient(sshClient)
	if err != nil {
		return fmt.Errorf("%w: failed to open SFTP session: %v", ErrSCPConnectFailed, err)
	}
	defer sftpClient.Close()

	remoteFile, err := sftpClient.Open(remoteName)
	if err != nil {
		return fmt.Errorf("failed to download router file '%s': %v", remoteName, err)
	}
	defer remoteFile.Close()

	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to write local file '%s': %v", localPath, err)
	}
	defer localFile.Close()

	_, err = io.Copy(localFile, remoteFile)
	return err
}

// UploadFile copies a local file onto the router over SFTP. It creates (or
// overwrites) the remote file at routerPath.
func (d *SCPFileDownloader) UploadFile(localPath, routerPath string) error {
	remoteName := strings.TrimLeft(routerPath, "/")

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file '%s': %v", localPath, err)
	}
	defer localFile.Close()

	sshClient, err := d.connect()
	if err != nil {
		return fmt.Errorf("failed to connect to SCP service on %s:%d: %w",
			d.settings.Host, d.settings.Port, err)
	}
	defer sshClient.Close()

	sftpClient, err := NewSFTPClient(sshClient)
	if err != nil {
		return fmt.Errorf("%w: failed to open SFTP session: %v", ErrSCPConnectFailed, err)
	}
	defer sftpClient.Close()

	remoteFile, err := sftpClient.Create(remoteName)
	if err != nil {
		return fmt.Errorf("failed to upload router file '%s': %v", remoteName, err)
	}

	if _, err := io.Copy(remoteFile, localFile); err != nil {
		remoteFile.Close()
		return fmt.Errorf("failed to upload router file '%s': %v", remoteName, err)
	}
	if err := remoteFile.Close(); err != nil {
		return fmt.Errorf("failed to finalize upload of '%s': %v", remoteName, err)
	}
	return nil
}

func (d *SCPFileDownloader) connect() (*ssh.Client, error) {
	return dialSSH(d.settings)
}

func dialSSH(settings *FileTransferSettings) (*ssh.Client, error) {
	auth, err := authMethods(settings)
	if err != nil {
		return nil, err
	}
	cb, err := hostKeyCallback(settings)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSCPConfigMissing, err)
	}
	config := &ssh.ClientConfig{
		User:            settings.Username,
		Auth:            auth,
		HostKeyCallback: cb,
		Timeout:         settings.Timeout,
	}

	addr := net.JoinHostPort(settings.Host, strconv.Itoa(settings.Port))
	client, err := SSHDial("tcp", addr, config)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unable to authenticate") {
			return nil, fmt.Errorf("%w: %v", ErrSCPAuthFailed, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrSCPConnectFailed, err)
	}
	return client, nil
}

func authMethods(settings *FileTransferSettings) ([]ssh.AuthMethod, error) {
	if settings.PrivateKey != "" {
		signer, err := loadPrivateKey(settings.PrivateKey, settings.KeyPassphrase)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}
	if settings.Password != "" {
		return []ssh.AuthMethod{ssh.Password(settings.Password)}, nil
	}
	return nil, nil
}

func hostKeyCallback(settings *FileTransferSettings) (ssh.HostKeyCallback, error) {
	if settings.SSHFingerprintSHA256 != "" {
		return sha256FingerprintPolicy(settings.Host, settings.SSHFingerprintSHA256), nil
	}
	if settings.Insecure || helpers.ParseBool(os.Getenv("MIKROTIK_SCP_INSECURE"), false) {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	return nil, errors.New("SSH host key verification is disabled: MIKROTIK_SCP_HOST_FINGERPRINT_SHA256 must be set (or MIKROTIK_SCP_INSECURE=1 to opt out)")
}

func loadPrivateKey(path, passphrase string) (ssh.Signer, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read private key %s: %v", path, err)
	}
	if passphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
	}
	return ssh.ParsePrivateKey(keyBytes)
}

func sha256FingerprintPolicy(hostname string, expected string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		actual := ssh.FingerprintSHA256(key)
		if actual != expected {
			return fmt.Errorf("SSH host key fingerprint mismatch for %s: expected %s, got %s",
				hostname, expected, actual)
		}
		return nil
	}
}

func LoadFileTransferSettings(apiHost string) (*FileTransferSettings, error) {
	username := os.Getenv("MIKROTIK_SCP_USER")
	if username == "" {
		username = os.Getenv("MIKROTIK_USER")
	}

	privateKey, err := ResolveSCPPrivateKeyPath()
	if err != nil {
		return nil, err
	}

	var password string
	if privateKey == "" {
		password = os.Getenv("MIKROTIK_SCP_PASSWORD")
		if password == "" {
			password = os.Getenv("MIKROTIK_PASSWORD")
		}
	}

	fingerprint := ""
	if envFP := os.Getenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256"); envFP != "" {
		var err error
		fingerprint, err = NormalizeSSHFingerprint(envFP)
		if err != nil {
			return nil, err
		}
	}

	if username == "" || (privateKey == "" && password == "") {
		return nil, errors.New("MIKROTIK_SCP_USER plus either MIKROTIK_SCP_PRIVATE_KEY, MIKROTIK_SCP_PASSWORD, or MIKROTIK_PASSWORD must be set before downloading files")
	}

	host := os.Getenv("MIKROTIK_SCP_HOST")
	if host == "" {
		host = apiHost
	}

	return &FileTransferSettings{
		Host:                 host,
		Username:             username,
		Password:             password,
		PrivateKey:           privateKey,
		KeyPassphrase:        os.Getenv("MIKROTIK_SCP_KEY_PASSPHRASE"),
		SSHFingerprintSHA256: fingerprint,
		Port:                 helpers.IntFromEnv("MIKROTIK_SCP_PORT", 22),
		Timeout:              time.Duration(helpers.FloatFromEnv("MIKROTIK_SCP_TIMEOUT", 30.0) * float64(time.Second)),
		Insecure:             helpers.ParseBool(os.Getenv("MIKROTIK_SCP_INSECURE"), false),
	}, nil
}

func LoadPasswordRotationSettings(host string) (*FileTransferSettings, error) {
	if os.Getenv("MIKROTIK_SCP_PRIVATE_KEY") == "" {
		return nil, errors.New("MIKROTIK_SCP_PRIVATE_KEY must be set when API passwordless startup rotation is enabled")
	}
	settings, err := LoadFileTransferSettings(host)
	if err != nil {
		return nil, err
	}
	if settings.PrivateKey == "" {
		return nil, errors.New("MIKROTIK_SCP_PRIVATE_KEY must be set when API passwordless startup rotation is enabled")
	}
	return settings, nil
}

func RotateRouterOSPassword(host, username string) (string, error) {
	length := helpers.IntFromEnv("MIKROTIK_API_PASSWORDLESS_LENGTH", 32)
	if length < 1 {
		return "", errors.New("MIKROTIK_API_PASSWORDLESS_LENGTH must be at least 1")
	}
	newPassword, err := GenerateAPIPassword(length)
	if err != nil {
		return "", err
	}

	settings, err := LoadPasswordRotationSettings(host)
	if err != nil {
		return "", err
	}

	sshClient, err := dialSSH(settings)
	if err != nil {
		return "", fmt.Errorf("failed to rotate password: %v", err)
	}
	defer sshClient.Close()

	command := fmt.Sprintf("/user set [find where name=%s] password=%s",
		shellQuote(username), shellQuote(newPassword))
	_, err = runSSHCommand(sshClient, command, settings.Timeout)
	if err != nil {
		return "", fmt.Errorf("failed to rotate RouterOS password for user '%s' on %s:%d: %v",
			username, settings.Host, settings.Port, err)
	}

	return newPassword, nil
}

func CheckPasswordRotationReady(host, username string) (map[string]any, error) {
	settings, err := LoadPasswordRotationSettings(host)
	if err != nil {
		return nil, err
	}

	sshClient, err := dialSSH(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to verify readiness: %v", err)
	}
	defer sshClient.Close()

	command := fmt.Sprintf("/user print count-only where name=%s", shellQuote(username))
	output, err := runSSHCommand(sshClient, command, settings.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to verify RouterOS passwordless readiness for user '%s' on %s:%d: %v",
			username, settings.Host, settings.Port, err)
	}

	count := 0
	fmt.Sscanf(strings.TrimSpace(output), "%d", &count)
	if count != 1 {
		return nil, fmt.Errorf("RouterOS user '%s' was not found over SSH", username)
	}

	return map[string]any{
		"host":          settings.Host,
		"port":          settings.Port,
		"username":      username,
		"target_exists": true,
		"command":       command,
	}, nil
}

func generateAPIPassword(length int) (string, error) {
	if length < 1 {
		return "", errors.New("password length must be at least 1")
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	max := byte(256 - (256 % len(alphabet)))
	for i := range b {
		for {
			if _, err := io.ReadFull(rand.Reader, b[i:i+1]); err != nil {
				return "", err
			}
			if b[i] < max {
				b[i] = alphabet[b[i]%byte(len(alphabet))]
				break
			}
		}
	}
	return string(b), nil
}

var GenerateAPIPassword = generateAPIPassword

func runSSHCommand(client *ssh.Client, command string, timeout time.Duration) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr strings.Builder
	session.Stdout = &stdout
	session.Stderr = &stderr

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case err := <-done:
		stderrStr := stderr.String()
		stdoutStr := stdout.String()
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				return "", fmt.Errorf("%s%s (exit %d)", stderrStr, stdoutStr, exitErr.ExitStatus())
			}
			return "", err
		}
		if stderrStr != "" {
			return "", errors.New(stderrStr)
		}
		return stdoutStr, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func ProbeSSHFingerprint(host string, port int, timeout time.Duration) (map[string]any, error) {
	var serverKey ssh.PublicKey
	config := &ssh.ClientConfig{
		Timeout: timeout,
		User:    "probe",
		Auth:    []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			serverKey = key
			return nil
		},
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	client, err := SSHDial("tcp", addr, config)
	if err == nil {
		client.Close()
	}

	// The host key is exchanged before authentication, so serverKey is set
	// even when the probe's deliberately bogus login is rejected. That is the
	// whole point of the probe: report the key regardless.
	if serverKey == nil {
		return map[string]any{
			"status":  "failed",
			"message": fmt.Sprintf("Failed to obtain SSH host key from %s", addr),
			"host":    host,
			"port":    port,
		}, nil
	}

	result := map[string]any{
		"status":             "ok",
		"message":            fmt.Sprintf("SSH server fingerprint obtained from %s", addr),
		"host":               host,
		"port":               port,
		"key_type":           serverKey.Type(),
		"fingerprint_sha256": ssh.FingerprintSHA256(serverKey),
	}
	if err != nil {
		// Auth failed (expected for a probe); the key was still captured.
		result["auth"] = "rejected"
	}
	return result, nil
}

func ResolveSCPPrivateKeyPath() (string, error) {
	configured := os.Getenv("MIKROTIK_SCP_PRIVATE_KEY")
	if configured == "" {
		return "", nil
	}
	path := configured
	if !filepath.IsAbs(path) {
		path = filepath.Join(helpers.WorkspaceRoot(), path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("SCP private key file '%s' is inaccessible: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("SCP private key path '%s' is not a regular file", path)
	}
	return path, nil
}

func NormalizeSSHFingerprint(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", errors.New("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256 must not be empty")
	}

	if strings.HasPrefix(strings.ToLower(normalized), "sha256:") {
		normalized = normalized[7:]
	}
	if normalized == "" {
		return "", errors.New("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256 must include a SHA256 fingerprint value")
	}

	// Add padding
	padding := (4 - len(normalized)%4) % 4
	normalized += strings.Repeat("=", padding)

	decoded, err := base64.StdEncoding.DecodeString(normalized)
	if err != nil {
		return "", fmt.Errorf("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256 must be an OpenSSH-style SHA256 fingerprint: %v", err)
	}
	if len(decoded) != sha256.Size {
		return "", errors.New("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256 must decode to a SHA256 digest")
	}

	encoded := base64.StdEncoding.EncodeToString(decoded)
	encoded = strings.TrimRight(encoded, "=")
	return "SHA256:" + encoded, nil
}

func shellQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '$':
			b.WriteString(`\$`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
