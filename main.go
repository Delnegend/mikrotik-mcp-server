package main

import (
	"flag"
	"log"

	"github.com/mark3labs/mcp-go/server"
	"github.com/pheoxy/mikrotik-mcp/internal/runtime"
	mcpserver "github.com/pheoxy/mikrotik-mcp/internal/server"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		log.Fatal("Usage: mikrotik-mcp <host>")
	}

	host := args[0]

	cl, err := runtime.LoadSettings(host)
	if err != nil {
		log.Fatalf("Failed to load settings: %v", err)
	}
	defer cl.Close()

	s := mcpserver.NewMCPServer(cl)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
