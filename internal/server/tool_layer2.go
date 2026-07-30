package server

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/pheoxy/mikrotik-mcp/internal/client"
	"github.com/pheoxy/mikrotik-mcp/internal/helpers"
)

func registerLayer2Tools(s *server.MCPServer, cl *client.RouterOSClient) {
	addTool(s, mcp.NewTool("bridge_list",
		mcp.WithDescription("List bridges with optional name and disabled filters."),
		mcp.WithString("name", mcp.Description("Filter by bridge name")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), filteredListHandler(cl, "/interface/bridge", map[string]string{"name": "name", "disabled": "disabled"}))

	addTool(s, mcp.NewTool("bridge_add",
		mcp.WithDescription("Create a bridge using RouterOS bridge attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("Bridge attributes")),
	), addHandler(cl, "/interface/bridge"))

	addTool(s, mcp.NewTool("bridge_remove",
		mcp.WithDescription("Remove a bridge by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(cl, "/interface/bridge"))

	addTool(s, mcp.NewTool("bridge_port_list",
		mcp.WithDescription("List bridge ports with optional bridge, interface, and disabled filters."),
		mcp.WithString("bridge", mcp.Description("Filter by bridge")),
		mcp.WithString("interface", mcp.Description("Filter by interface")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), filteredListHandler(cl, "/interface/bridge/port", map[string]string{"bridge": "bridge", "interface": "interface", "disabled": "disabled"}))

	addTool(s, mcp.NewTool("bridge_port_add",
		mcp.WithDescription("Add a bridge port using RouterOS bridge port attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("Bridge port attributes")),
	), addHandler(cl, "/interface/bridge/port"))

	addTool(s, mcp.NewTool("bridge_port_remove",
		mcp.WithDescription("Remove a bridge port by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(cl, "/interface/bridge/port"))

	addTool(s, mcp.NewTool("bridge_vlan_list",
		mcp.WithDescription("List bridge VLAN entries with optional bridge, VLAN ID, and disabled filters."),
		mcp.WithString("bridge", mcp.Description("Filter by bridge")),
		mcp.WithString("vlan_ids", mcp.Description("Filter by VLAN IDs")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), filteredListHandler(cl, "/interface/bridge/vlan", map[string]string{"bridge": "bridge", "vlan_ids": "vlan-ids", "disabled": "disabled"}))

	addTool(s, mcp.NewTool("bridge_vlan_add",
		mcp.WithDescription("Add a bridge VLAN entry using RouterOS bridge VLAN attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("Bridge VLAN attributes")),
	), addHandler(cl, "/interface/bridge/vlan"))

	addTool(s, mcp.NewTool("bridge_vlan_remove",
		mcp.WithDescription("Remove a bridge VLAN entry by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(cl, "/interface/bridge/vlan"))

	addTool(s, mcp.NewTool("vlan_list",
		mcp.WithDescription("List VLAN interfaces with optional name, parent interface, and disabled filters."),
		mcp.WithString("name", mcp.Description("Filter by VLAN name")),
		mcp.WithString("interface", mcp.Description("Filter by parent interface")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), filteredListHandler(cl, "/interface/vlan", map[string]string{"name": "name", "interface": "interface", "disabled": "disabled"}))

	addTool(s, mcp.NewTool("vlan_add",
		mcp.WithDescription("Create a VLAN interface using RouterOS VLAN attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("VLAN attributes")),
	), addHandler(cl, "/interface/vlan"))

	addTool(s, mcp.NewTool("vlan_remove",
		mcp.WithDescription("Remove a VLAN interface by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(cl, "/interface/vlan"))
}

func listHandler(cl *client.RouterOSClient, menu string) server.ToolHandlerFunc {
	return filteredListHandler(cl, menu, nil)
}

func filteredListHandler(cl *client.RouterOSClient, menu string, filterParams map[string]string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var queries []string
		for param, rosField := range filterParams {
			v, ok := req.Params.Arguments[param]
			if !ok {
				continue
			}
			switch val := v.(type) {
			case bool:
				if val {
					queries = append(queries, rosField+"=true")
				} else {
					queries = append(queries, rosField+"=false")
				}
			case string:
				if val != "" {
					queries = append(queries, rosField+"="+val)
				}
			}
		}
		items, err := helpers.PrintRecords(cl, menu, nil, queries, nil)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(items)), nil
	}
}

func addHandler(cl *client.RouterOSClient, menu string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		attrs, err := helpers.RequireAttributes(argObject(req, "attributes"))
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
		itemID, err := helpers.NormalizeRequiredString(argString(req, "item_id", ""), "item_id")
		if err != nil {
			return nil, err
		}
		result, err := cl.Remove(menu, itemID)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}
