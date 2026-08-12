// Command chrprovision hardens a freshly booted CHR test router over the
// native RouterOS API: it sets the admin password, enables the api/api-ssl/ssh
// services, and names the router. It connects as admin with an EMPTY password
// (the CHR default), so it only works on a router that has not been
// provisioned yet — run `bash scripts/chr/up.sh --fresh` to reset first.
//
// Usage:  go run ./cmd/chrprovision [-host 127.0.0.1] [-port 8728] [-password admin]
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
)

func main() {
	host := flag.String("host", "127.0.0.1", "CHR host")
	port := flag.Int("port", 8728, "CHR API port (plain)")
	password := flag.String("password", "admin", "admin password to set")
	flag.Parse()

	// A freshly booted CHR accepts TCP on 8728 before the API processes
	// commands, so retry the empty-password login on connect/timeout errors.
	// An auth failure means the router is already provisioned: stop at once
	// (each retry would log another failed login on the router).
	var cl *client.RouterOSClient
	var err error
	for i := range 10 {
		cl = client.NewRouterOSClient(*host, "admin", "",
			client.WithTLS(false), client.WithPort(*port), client.WithTimeout(30*time.Second))
		if err = cl.Open(); err == nil {
			break
		}
		if errors.Is(err, client.ErrRouterOSAuthError) {
			break
		}
		fmt.Fprintf(os.Stderr, "login attempt %d failed: %v\n", i+1, err)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "login with empty password failed (router already provisioned?): %v\n", err)
		fmt.Fprintln(os.Stderr, "Reset with: bash scripts/chr/up.sh --fresh")
		os.Exit(2)
	}
	defer cl.Close()

	idOf := func(menu, name string) (string, error) {
		items, err := cl.Print(menu, nil, nil, nil)
		if err != nil {
			return "", err
		}
		for _, it := range items {
			if it["name"] == name {
				return it[".id"], nil
			}
		}
		return "", fmt.Errorf("%q not found in %s", name, menu)
	}

	adminID, err := idOf("/user", "admin")
	if err != nil {
		fatal(err)
	}
	if _, err := cl.Set("/user", adminID, map[string]any{"password": *password}); err != nil {
		fatal(fmt.Errorf("set admin password: %w", err))
	}

	for _, svc := range []string{"api", "api-ssl", "ssh", "winbox"} {
		id, err := idOf("/ip/service", svc)
		if err != nil {
			fatal(err)
		}
		if _, err := cl.Set("/ip/service", id, map[string]any{"disabled": false}); err != nil {
			fatal(fmt.Errorf("enable service %s: %w", svc, err))
		}
	}

	if _, err := cl.Run("/system/identity/set", map[string]any{"name": "mikrotik-mcp-test"}, nil, ""); err != nil {
		fatal(fmt.Errorf("set identity: %w", err))
	}

	fmt.Printf("provisioned: admin password=%q, api/api-ssl/ssh enabled, identity=mikrotik-mcp-test\n", *password)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "chrprovision: %v\n", err)
	os.Exit(1)
}
