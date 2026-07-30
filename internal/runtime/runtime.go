package runtime

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/downloads"
	"github.com/Delnegend/mikrotik-mcp/internal/helpers"
)

var workspaceRoot = WorkspaceRoot

func WorkspaceRoot() string {
	dir, _ := os.Getwd()
	return dir
}

func LoadSettings(host string) (*client.RouterOSClient, error) {
	clearEmptyMikrotikEnvVars()
	_ = godotenv.Load(filepath.Join(workspaceRoot(), ".env"))

	username := os.Getenv("MIKROTIK_USER")
	if username == "" {
		return nil, fmt.Errorf("MIKROTIK_USER must be set before starting the MCP server")
	}

	var password string
	if passwordlessEnabled() {
		var err error
		password, err = resolveStartupAPIPassword(host, username)
		if err != nil {
			return nil, err
		}
	} else {
		clearStartupPasswordlessState()
		password = os.Getenv("MIKROTIK_PASSWORD")
		if password == "" {
			return nil, fmt.Errorf("MIKROTIK_USER and MIKROTIK_PASSWORD must be set before starting the MCP server")
		}
	}

	useSSL := helpers.ParseBool(os.Getenv("MIKROTIK_API_SSL"), true)
	tlsVerify := helpers.ParseBool(os.Getenv("MIKROTIK_TLS_VERIFY"), true)
	port := helpers.IntFromEnv("MIKROTIK_API_PORT", 0)
	if port == 0 {
		if useSSL {
			port = 8729
		} else {
			port = 8728
		}
	}

	tlsCAFiles := LoadTLSCAFiles()

	return client.NewRouterOSClient(host, username, password,
		client.WithTLS(useSSL),
		client.WithTLSVerify(tlsVerify),
		client.WithPort(port),
		client.WithTLSCAFiles(tlsCAFiles),
	), nil
}

func LoadTLSCAFiles() []string {
	certsDir := filepath.Join(workspaceRoot(), "certs")
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
		ext := strings.ToLower(filepath.Ext(name))
		if !validExtensions[ext] {
			continue
		}
		if strings.HasSuffix(name, ".disabled") {
			continue
		}
		files = append(files, filepath.Join(certsDir, name))
	}
	sort.Strings(files)
	return files
}

func clearEmptyMikrotikEnvVars() {
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name, value := parts[0], parts[1]
		if strings.HasPrefix(name, "MIKROTIK_") && value == "" {
			os.Unsetenv(name)
		}
	}
}

func passwordlessEnabled() bool {
	return helpers.ParseBool(os.Getenv("MIKROTIK_API_PASSWORDLESS_ENABLED"), false)
}

func startupPasswordlessState() map[string]string {
	return map[string]string{
		"status":  os.Getenv("MIKROTIK_STARTUP_PASSWORDLESS_STATUS"),
		"code":    os.Getenv("MIKROTIK_STARTUP_PASSWORDLESS_CODE"),
		"message": os.Getenv("MIKROTIK_STARTUP_PASSWORDLESS_MESSAGE"),
	}
}

func setStartupPasswordlessState(status, code, message string) {
	if status == "" {
		clearStartupPasswordlessState()
		return
	}
	os.Setenv("MIKROTIK_STARTUP_PASSWORDLESS_STATUS", status)
	os.Setenv("MIKROTIK_STARTUP_PASSWORDLESS_CODE", code)
	os.Setenv("MIKROTIK_STARTUP_PASSWORDLESS_MESSAGE", message)
}

func clearStartupPasswordlessState() {
	for _, key := range []string{
		"MIKROTIK_STARTUP_PASSWORDLESS_STATUS",
		"MIKROTIK_STARTUP_PASSWORDLESS_CODE",
		"MIKROTIK_STARTUP_PASSWORDLESS_MESSAGE",
	} {
		os.Unsetenv(key)
	}
}

func resolveStartupAPIPassword(host, username string) (string, error) {
	fingerprint := os.Getenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256")
	if fingerprint == "" {
		return "", fmt.Errorf("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256 must be set before starting passwordless API rotation")
	}

	password, err := rotateStartupAPIPassword(host, username)
	if err != nil {
		return "", fmt.Errorf("startup password rotation failed: %v", err)
	}

	clearStartupPasswordlessState()
	return password, nil
}

func rotateStartupAPIPassword(host, username string) (string, error) {
	return downloads.RotateRouterOSPassword(host, username)
}

func GenerateAPIPassword(length int) (string, error) {
	if length < 1 {
		return "", fmt.Errorf("password length must be at least 1")
	}
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
