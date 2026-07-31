package server

import (
	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/mark3labs/mcp-go/server"
)

func NewMCPServer(cl *client.RouterOSClient) *server.MCPServer {
	s := server.NewMCPServer(
		"mikrotik",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	registerCoreTools(s, cl)
	registerLayer2Tools(s, cl)
	registerSecurityTools(s, cl)
	registerAccessTools(s, cl)
	registerFileTools(s, cl)

	return s
}
