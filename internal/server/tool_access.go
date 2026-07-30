package server

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/pheoxy/mikrotik-mcp/internal/client"
	"github.com/pheoxy/mikrotik-mcp/internal/helpers"
)

func registerAccessTools(s *server.MCPServer, cl *client.RouterOSClient) {
	addTool(s, mcp.NewTool("ppp_active_list",
		mcp.WithDescription("List active PPP sessions with optional service and name filters."),
		mcp.WithString("service", mcp.Description("Filter by service type")),
		mcp.WithString("name", mcp.Description("Filter by username")),
	), listHandler(cl, "/ppp/active"))

	addTool(s, mcp.NewTool("ppp_secret_list",
		mcp.WithDescription("List PPP secrets with optional name, service, and disabled filters."),
		mcp.WithString("name", mcp.Description("Filter by name")),
		mcp.WithString("service", mcp.Description("Filter by service")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), listHandler(cl, "/ppp/secret"))

	addTool(s, mcp.NewTool("ppp_secret_add",
		mcp.WithDescription("Create a PPP secret using RouterOS PPP secret attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("PPP secret attributes (name and password required)")),
	), pppSecretAddHandler(cl))

	addTool(s, mcp.NewTool("ppp_secret_remove",
		mcp.WithDescription("Remove a PPP secret by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(cl, "/ppp/secret"))

	addTool(s, mcp.NewTool("wireguard_interface_list",
		mcp.WithDescription("List WireGuard interfaces with optional name and disabled filters."),
		mcp.WithString("name", mcp.Description("Filter by interface name")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), listHandler(cl, "/interface/wireguard"))

	addTool(s, mcp.NewTool("wireguard_interface_add",
		mcp.WithDescription("Create a WireGuard interface using RouterOS WireGuard attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("WireGuard interface attributes (name required)")),
	), wgInterfaceAddHandler(cl))

	addTool(s, mcp.NewTool("wireguard_peer_list",
		mcp.WithDescription("List WireGuard peers with optional interface and disabled filters."),
		mcp.WithString("interface", mcp.Description("Filter by interface")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), listHandler(cl, "/interface/wireguard/peers"))

	addTool(s, mcp.NewTool("wireguard_peer_add",
		mcp.WithDescription("Create a WireGuard peer using RouterOS peer attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("WireGuard peer attributes (interface and public-key required)")),
	), wgPeerAddHandler(cl))

	addTool(s, mcp.NewTool("wireguard_peer_remove",
		mcp.WithDescription("Remove a WireGuard peer by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(cl, "/interface/wireguard/peers"))
}

func pppSecretAddHandler(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attrs, err := helpers.RequireAttributeFields(argObject(req, "attributes"), []string{"name", "password"})
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
		attrs, err := helpers.RequireAttributeFields(argObject(req, "attributes"), []string{"name"})
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
		attrs, err := helpers.RequireAttributeFields(argObject(req, "attributes"), []string{"interface", "public-key"})
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
