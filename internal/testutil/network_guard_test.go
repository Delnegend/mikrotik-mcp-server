package testutil

import (
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
)

// Network guardrail: blocks accidental real-TCP dials from the RouterOS
// client during tests. Localhost dials are allowed (in-memory SSH server),
// everything else requires MIKROTIK_TEST_ALLOW_NETWORK=1.
func init() {
	orig := client.NetDial
	client.NetDial = func(network, addr string, timeout time.Duration) (net.Conn, error) {
		if os.Getenv("MIKROTIK_TEST_ALLOW_NETWORK") == "1" {
			return orig(network, addr, timeout)
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		host = strings.Trim(host, "[]")
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return orig(network, addr, timeout)
		}
		return nil, errors.New("network access blocked by test guard; set MIKROTIK_TEST_ALLOW_NETWORK=1 to allow real dials")
	}
}
