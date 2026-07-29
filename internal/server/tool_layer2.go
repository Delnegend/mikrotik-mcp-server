package server

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/pheoxy/mikrotik-mcp/internal/client"
	"github.com/pheoxy/mikrotik-mcp/internal/helpers"
)

func registerLayer2Tools(s *server.MCPServer, cl *client.RouterOSClient) {
	reg := func(name, desc string, handler server.ToolHandlerFunc) {
		s.AddTool(mcp.NewTool(name, mcp.WithDescription(desc)), handler)
	}

	reg("bridge_list", "List bridges with optional name and disabled filters.", listHandler(cl, "/interface/bridge"))
	reg("bridge_add", "Create a bridge using RouterOS bridge attributes.", addHandler(cl, "/interface/bridge"))
	reg("bridge_remove", "Remove a bridge by RouterOS item id.", removeHandler(cl, "/interface/bridge"))
	reg("bridge_port_list", "List bridge ports with optional bridge, interface, and disabled filters.", listHandler(cl, "/interface/bridge/port"))
	reg("bridge_port_add", "Add a bridge port using RouterOS bridge port attributes.", addHandler(cl, "/interface/bridge/port"))
	reg("bridge_port_remove", "Remove a bridge port by RouterOS item id.", removeHandler(cl, "/interface/bridge/port"))
	reg("bridge_vlan_list", "List bridge VLAN entries with optional bridge, VLAN ID, and disabled filters.", listHandler(cl, "/interface/bridge/vlan"))
	reg("bridge_vlan_add", "Add a bridge VLAN entry using RouterOS bridge VLAN attributes.", addHandler(cl, "/interface/bridge/vlan"))
	reg("bridge_vlan_remove", "Remove a bridge VLAN entry by RouterOS item id.", removeHandler(cl, "/interface/bridge/vlan"))
	reg("vlan_list", "List VLAN interfaces with optional name, parent interface, and disabled filters.", listHandler(cl, "/interface/vlan"))
	reg("vlan_add", "Create a VLAN interface using RouterOS VLAN attributes.", addHandler(cl, "/interface/vlan"))
	reg("vlan_remove", "Remove a VLAN interface by RouterOS item id.", removeHandler(cl, "/interface/vlan"))
}

func listHandler(cl *client.RouterOSClient, menu string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		items, err := helpers.PrintRecords(cl, menu, nil, nil, nil)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(items)), nil
	}
}

func addHandler(cl *client.RouterOSClient, menu string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attrs, err := helpers.RequireAttributes(req.Params.Arguments)
		if err != nil {
			return nil, err
		}
		result, err := cl.Add(menu, attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func removeHandler(cl *client.RouterOSClient, menu string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		itemID := argString(req, "item_id", "")
		result, err := cl.Remove(menu, itemID)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}
