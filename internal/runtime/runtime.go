package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/downloads"
	"github.com/Delnegend/mikrotik-mcp/internal/helpers"
	"github.com/Delnegend/mikrotik-mcp/internal/inventory"
)

// LoadRegistry builds the device registry at startup: a fleet when an
// inventory is configured, otherwise the single device from the flat env.
func LoadRegistry(host string) (*inventory.Registry, error) {
	loadEnvFile(filepath.Join(helpers.WorkspaceRoot(), ".env"))

	if inventory.Configured() {
		reg, err := inventory.FromEnv()
		if err != nil {
			return nil, err
		}
		return reg, nil
	}

	d, err := loadDevice(host)
	if err != nil {
		return nil, err
	}
	return inventory.Single(*d), nil
}

// LoadSettings builds the single-device client from the environment.
func LoadSettings(host string) (*client.RouterOSClient, error) {
	loadEnvFile(filepath.Join(helpers.WorkspaceRoot(), ".env"))

	d, err := loadDevice(host)
	if err != nil {
		return nil, err
	}
	return d.Client(), nil
}

func loadDevice(host string) (*inventory.Device, error) {
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

	useSSL, err := boolFromEnv("MIKROTIK_API_SSL", true)
	if err != nil {
		return nil, err
	}
	tlsVerify, err := boolFromEnv("MIKROTIK_TLS_VERIFY", true)
	if err != nil {
		return nil, err
	}
	port := helpers.IntFromEnv("MIKROTIK_API_PORT", 0)
	if port == 0 {
		if useSSL {
			port = 8729
		} else {
			port = 8728
		}
	}
	timeout := time.Duration(helpers.FloatFromEnv("MIKROTIK_API_TIMEOUT", 10.0) * float64(time.Second))

	sshUser := envOr("MIKROTIK_SCP_USER", username)
	sshPort := helpers.IntFromEnv("MIKROTIK_SCP_PORT", 22)

	return &inventory.Device{
		Title:                host,
		Host:                 host,
		Port:                 port,
		Username:             username,
		Password:             password,
		APISSL:               useSSL,
		TLSVerify:            tlsVerify,
		Timeout:              timeout,
		TLSCAFiles:           LoadTLSCAFiles(),
		SSHPort:              sshPort,
		SSHUsername:          sshUser,
		SSHFingerprintSHA256: os.Getenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256"),
	}, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// boolFromEnv parses a boolean env var, failing on unrecognized values so a
// typo cannot silently flip a security-relevant setting.
func boolFromEnv(key string, defaultVal bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultVal, nil
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s has an unrecognized value %q (expected true/false)", key, raw)
	}
}

func LoadTLSCAFiles() []string {
	certsDir := filepath.Join(helpers.WorkspaceRoot(), "certs")
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

func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if value == "" {
			continue
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

func passwordlessEnabled() bool {
	return helpers.ParseBool(os.Getenv("MIKROTIK_API_PASSWORDLESS_ENABLED"), false)
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

	password, err := downloads.RotateRouterOSPassword(host, username)
	if err != nil {
		return "", fmt.Errorf("startup password rotation failed: %v", err)
	}

	clearStartupPasswordlessState()
	return password, nil
}
