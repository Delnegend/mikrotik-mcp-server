package testutil

import (
	"os"
	"strings"
	"testing"
)

func Setenv(t *testing.T, key, value string) {
	t.Helper()
	orig := os.Getenv(key)
	os.Setenv(key, value)
	t.Cleanup(func() { os.Setenv(key, orig) })
}

func ClearMikrotikEnv(t *testing.T) {
	t.Helper()
	saved := make(map[string]string)
	for _, pair := range os.Environ() {
		eq := strings.IndexByte(pair, '=')
		if eq < 0 {
			continue
		}
		key := pair[:eq]
		if strings.HasPrefix(key, "MIKROTIK_") {
			saved[key] = os.Getenv(key)
			os.Unsetenv(key)
		}
	}
	t.Cleanup(func() {
		for k, v := range saved {
			os.Setenv(k, v)
		}
	})
}
