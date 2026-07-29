package server

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/pheoxy/mikrotik-mcp/internal/client"
	"github.com/pheoxy/mikrotik-mcp/internal/helpers"
)

func registerSecurityTools(s *server.MCPServer, cl *client.RouterOSClient) {
	reg := func(name, desc string, handler server.ToolHandlerFunc) {
		s.AddTool(mcp.NewTool(name, mcp.WithDescription(desc)), handler)
	}

	reg("firewall_filter_list", "List firewall filter rules with optional chain, action, and disabled filters.", listHandler(cl, "/ip/firewall/filter"))
	reg("firewall_filter_add", "Add a firewall filter rule using RouterOS firewall attributes.", addHandler(cl, "/ip/firewall/filter"))
	reg("firewall_filter_set", "Update a firewall filter rule by RouterOS item id.", setHandler(cl, "/ip/firewall/filter"))
	reg("firewall_filter_remove", "Remove a firewall filter rule by RouterOS item id.", removeHandler(cl, "/ip/firewall/filter"))
	reg("firewall_nat_list", "List firewall NAT rules with optional chain, action, and disabled filters.", listHandler(cl, "/ip/firewall/nat"))
	reg("firewall_nat_add", "Add a firewall NAT rule using RouterOS firewall attributes.", addHandler(cl, "/ip/firewall/nat"))
	reg("firewall_nat_set", "Update a firewall NAT rule by RouterOS item id.", setHandler(cl, "/ip/firewall/nat"))
	reg("firewall_nat_remove", "Remove a firewall NAT rule by RouterOS item id.", removeHandler(cl, "/ip/firewall/nat"))
	reg("firewall_rule_move", "Move a firewall filter or NAT rule to a new destination position or item id.", firewallRuleMoveHandler(cl))
	reg("firewall_address_list_list", "List firewall address-list entries with optional list, address, and disabled filters.", listHandler(cl, "/ip/firewall/address-list"))
	reg("firewall_address_list_add", "Add a firewall address-list entry using RouterOS firewall attributes.", addHandler(cl, "/ip/firewall/address-list"))
	reg("firewall_address_list_remove", "Remove a firewall address-list entry by RouterOS item id.", removeHandler(cl, "/ip/firewall/address-list"))
}

func setHandler(cl *client.RouterOSClient, menu string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		itemID := argString(req, "item_id", "")
		attrs, err := helpers.RequireAttributes(req.Params.Arguments)
		if err != nil {
			return nil, err
		}
		result, err := cl.Set(menu, itemID, attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func firewallRuleMoveHandler(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		table := argString(req, "table", "")
		itemID := argString(req, "item_id", "")
		dest := argString(req, "destination", "")

		normalizedTable, err := helpers.NormalizeFirewallTable(table)
		if err != nil {
			return nil, err
		}
		normalizedID, err := helpers.NormalizeRequiredString(itemID, "item_id")
		if err != nil {
			return nil, err
		}
		normalizedDest, err := helpers.NormalizeMoveDestination(dest)
		if err != nil {
			return nil, err
		}

		path := fmt.Sprintf("/ip/firewall/%s/move", normalizedTable)
		result, err := cl.Run(path, map[string]any{".id": normalizedID, "destination": normalizedDest}, nil, "")
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}
