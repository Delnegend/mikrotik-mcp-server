package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/Delnegend/mikrotik-mcp/internal/runtime"
	mcpserver "github.com/Delnegend/mikrotik-mcp/internal/server"
	"github.com/mark3labs/mcp-go/server"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("mikrotik-mcp %s\n", version)
		return
	}

	args := flag.Args()
	if len(args) < 1 {
		log.Fatal("Usage: mikrotik-mcp <host>")
	}

	host := args[0]

	reg, err := runtime.LoadRegistry(host)
	if err != nil {
		log.Fatalf("Failed to load settings: %v", err)
	}

	s := mcpserver.NewMCPServer(reg)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
