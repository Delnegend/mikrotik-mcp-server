package server

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/pheoxy/mikrotik-mcp/internal/client"
	"github.com/pheoxy/mikrotik-mcp/internal/helpers"
)

func registerAccessTools(s *server.MCPServer, cl *client.RouterOSClient) {
	reg := func(name, desc string, handler server.ToolHandlerFunc) {
		s.AddTool(mcp.NewTool(name, mcp.WithDescription(desc)), handler)
	}

	reg("ppp_active_list", "List active PPP sessions with optional service and name filters.", listHandler(cl, "/ppp/active"))
	reg("ppp_secret_list", "List PPP secrets with optional name, service, and disabled filters.", listHandler(cl, "/ppp/secret"))
	reg("ppp_secret_add", "Create a PPP secret using RouterOS PPP secret attributes.", pppSecretAddHandler(cl))
	reg("ppp_secret_remove", "Remove a PPP secret by RouterOS item id.", removeHandler(cl, "/ppp/secret"))
	reg("wireguard_interface_list", "List WireGuard interfaces with optional name and disabled filters.", listHandler(cl, "/interface/wireguard"))
	reg("wireguard_interface_add", "Create a WireGuard interface using RouterOS WireGuard attributes.", wgInterfaceAddHandler(cl))
	reg("wireguard_peer_list", "List WireGuard peers with optional interface and disabled filters.", listHandler(cl, "/interface/wireguard/peers"))
	reg("wireguard_peer_add", "Create a WireGuard peer using RouterOS peer attributes.", wgPeerAddHandler(cl))
	reg("wireguard_peer_remove", "Remove a WireGuard peer by RouterOS item id.", removeHandler(cl, "/interface/wireguard/peers"))
}

func pppSecretAddHandler(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attrs, err := helpers.RequireAttributeFields(req.Params.Arguments, []string{"name", "password"})
		if err != nil {
			return nil, err
		}
		result, err := cl.Add("/ppp/secret", attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func wgInterfaceAddHandler(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attrs, err := helpers.RequireAttributeFields(req.Params.Arguments, []string{"name"})
		if err != nil {
			return nil, err
		}
		result, err := cl.Add("/interface/wireguard", attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func wgPeerAddHandler(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attrs, err := helpers.RequireAttributeFields(req.Params.Arguments, []string{"interface", "public-key"})
		if err != nil {
			return nil, err
		}
		result, err := cl.Add("/interface/wireguard/peers", attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}
