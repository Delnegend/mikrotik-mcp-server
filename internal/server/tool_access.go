package server

import (
	"context"

	"github.com/Delnegend/mikrotik-mcp/internal/helpers"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerAccessTools(s *server.MCPServer, api *API) {
	addTool(s, mcp.NewTool("ppp_active_list",
		mcp.WithDescription("List active PPP sessions with optional service and name filters."),
		mcp.WithString("service", mcp.Description("Filter by service type")),
		mcp.WithString("name", mcp.Description("Filter by username")),
	), filteredListHandler(api, "/ppp/active", map[string]string{"service": "service", "name": "name"}))

	addTool(s, mcp.NewTool("ppp_secret_list",
		mcp.WithDescription("List PPP secrets with optional name, service, and disabled filters."),
		mcp.WithString("name", mcp.Description("Filter by name")),
		mcp.WithString("service", mcp.Description("Filter by service")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), filteredListHandler(api, "/ppp/secret", map[string]string{"name": "name", "service": "service", "disabled": "disabled"}))

	addTool(s, mcp.NewTool("ppp_secret_add",
		mcp.WithDescription("Create a PPP secret using RouterOS PPP secret attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("PPP secret attributes (name and password required)")),
	), pppSecretAddHandler(api))

	addTool(s, mcp.NewTool("ppp_secret_remove",
		mcp.WithDescription("Remove a PPP secret by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(api, "/ppp/secret"))

	addTool(s, mcp.NewTool("wireguard_interface_list",
		mcp.WithDescription("List WireGuard interfaces with optional name and disabled filters."),
		mcp.WithString("name", mcp.Description("Filter by interface name")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), filteredListHandler(api, "/interface/wireguard", map[string]string{"name": "name", "disabled": "disabled"}))

	addTool(s, mcp.NewTool("wireguard_interface_add",
		mcp.WithDescription("Create a WireGuard interface using RouterOS WireGuard attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("WireGuard interface attributes (name required)")),
	), wgInterfaceAddHandler(api))

	addTool(s, mcp.NewTool("wireguard_peer_list",
		mcp.WithDescription("List WireGuard peers with optional interface and disabled filters."),
		mcp.WithString("interface", mcp.Description("Filter by interface")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), filteredListHandler(api, "/interface/wireguard/peers", map[string]string{"interface": "interface", "disabled": "disabled"}))

	addTool(s, mcp.NewTool("wireguard_peer_add",
		mcp.WithDescription("Create a WireGuard peer using RouterOS peer attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("WireGuard peer attributes (interface and public-key required)")),
	), wgPeerAddHandler(api))

	addTool(s, mcp.NewTool("wireguard_peer_remove",
		mcp.WithDescription("Remove a WireGuard peer by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(api, "/interface/wireguard/peers"))
}

func pppSecretAddHandler(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attrs, err := helpers.RequireAttributeFields(argObject(req, "attributes"), []string{"name", "password"})
		if err != nil {
			return nil, err
		}
		result, err := api.add(req, "/ppp/secret", attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func wgInterfaceAddHandler(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attrs, err := helpers.RequireAttributeFields(argObject(req, "attributes"), []string{"name"})
		if err != nil {
			return nil, err
		}
		result, err := api.add(req, "/interface/wireguard", attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func wgPeerAddHandler(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attrs, err := helpers.RequireAttributeFields(argObject(req, "attributes"), []string{"interface", "public-key"})
		if err != nil {
			return nil, err
		}
		result, err := api.add(req, "/interface/wireguard/peers", attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}
