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
		before, _, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		key := before
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
