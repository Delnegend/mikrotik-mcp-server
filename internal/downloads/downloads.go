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

	"golang.org/x/crypto/ssh"
	"github.com/pkg/sftp"

	"github.com/pheoxy/mikrotik-mcp/internal/helpers"
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
		return nil, fmt.Errorf("failed to connect to SCP service on %s:%d: %v",
			d.settings.Host, d.settings.Port, err)
	}
	defer sshClient.Close()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, fmt.Errorf("connected to SCP service but directory probe failed: %v", err)
	}
	defer sftpClient.Close()

	wd, err := sftpClient.Getwd()
	if err != nil {
		wd = "/"
	}
	entries, err := sftpClient.ReadDir(wd)
	if err != nil {
		return nil, fmt.Errorf("connected to SCP service but directory probe failed: %v", err)
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
		return fmt.Errorf("failed to connect to SCP service on %s:%d: %v",
			d.settings.Host, d.settings.Port, err)
	}
	defer sshClient.Close()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("failed to open SFTP session: %v", err)
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

func (d *SCPFileDownloader) connect() (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User:            d.settings.Username,
		Auth:            d.authMethods(),
		HostKeyCallback: d.hostKeyCallback(),
		Timeout:         d.settings.Timeout,
	}

	addr := net.JoinHostPort(d.settings.Host, strconv.Itoa(d.settings.Port))
	return ssh.Dial("tcp", addr, config)
}

func (d *SCPFileDownloader) authMethods() []ssh.AuthMethod {
	if d.settings.PrivateKey != "" {
		signer := loadPrivateKey(d.settings.PrivateKey, d.settings.KeyPassphrase)
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}
	}
	if d.settings.Password != "" {
		return []ssh.AuthMethod{ssh.Password(d.settings.Password)}
	}
	return nil
}

func (d *SCPFileDownloader) hostKeyCallback() ssh.HostKeyCallback {
	if d.settings.SSHFingerprintSHA256 != "" {
		return sha256FingerprintPolicy(d.settings.Host, d.settings.SSHFingerprintSHA256)
	}
	return ssh.InsecureIgnoreHostKey()
}

func loadPrivateKey(path, passphrase string) ssh.Signer {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("cannot read private key: %v", err))
	}
	var signer ssh.Signer
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(keyBytes)
	}
	if err != nil {
		panic(fmt.Sprintf("cannot parse private key: %v", err))
	}
	return signer
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
		Timeout: time.Duration(helpers.FloatFromEnv("MIKROTIK_SCP_TIMEOUT", 30.0) * float64(time.Second)),
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

	sshClient, err := connectSSH(settings)
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

	sshClient, err := connectSSH(settings)
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
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[b[i]%byte(len(alphabet))]
	}
	return string(b), nil
}

var GenerateAPIPassword = generateAPIPassword

func connectSSH(settings *FileTransferSettings) (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User:            settings.Username,
		HostKeyCallback: sha256FingerprintPolicy(settings.Host, settings.SSHFingerprintSHA256),
		Timeout:         settings.Timeout,
	}

	if settings.PrivateKey != "" {
		signer := loadPrivateKey(settings.PrivateKey, settings.KeyPassphrase)
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else if settings.Password != "" {
		config.Auth = []ssh.AuthMethod{ssh.Password(settings.Password)}
	}

	if settings.SSHFingerprintSHA256 == "" {
		config.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	addr := net.JoinHostPort(settings.Host, strconv.Itoa(settings.Port))
	return ssh.Dial("tcp", addr, config)
}

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
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return map[string]any{
			"status":  "failed",
			"message": fmt.Sprintf("Failed to fetch SSH server fingerprint from %s: %v", addr, err),
			"host":    host,
			"port":    port,
		}, nil
	}
	client.Close()

	keyType := serverKey.Type()
	fingerprint := ssh.FingerprintSHA256(serverKey)
	return map[string]any{
		"status":               "ok",
		"message":              fmt.Sprintf("SSH server fingerprint probe succeeded for %s", addr),
		"host":                 host,
		"port":                 port,
		"key_type":             keyType,
		"fingerprint_sha256":   fingerprint,
	}, nil
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

func sshHostKeySHA256(key ssh.PublicKey) string {
	hash := sha256.Sum256(key.Marshal())
	encoded := base64.StdEncoding.EncodeToString(hash[:])
	encoded = strings.TrimRight(encoded, "=")
	return "SHA256:" + encoded
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
