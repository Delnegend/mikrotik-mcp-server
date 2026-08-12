package server

import (
	"context"
	"fmt"

	"github.com/Delnegend/mikrotik-mcp/internal/helpers"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSecurityTools(s *server.MCPServer, api *API) {
	addTool(s, mcp.NewTool("firewall_filter_list",
		mcp.WithDescription("List firewall filter rules with optional chain, action, and disabled filters."),
		mcp.WithString("chain", mcp.Description("Filter by chain")),
		mcp.WithString("action", mcp.Description("Filter by action")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), filteredListHandler(api, "/ip/firewall/filter", map[string]string{"chain": "chain", "action": "action", "disabled": "disabled"}))

	addTool(s, mcp.NewTool("firewall_filter_add",
		mcp.WithDescription("Add a firewall filter rule using RouterOS firewall attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("Firewall filter attributes")),
	), addHandler(api, "/ip/firewall/filter"))

	addTool(s, mcp.NewTool("firewall_filter_set",
		mcp.WithDescription("Update a firewall filter rule by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("Firewall filter attributes")),
	), setHandler(api, "/ip/firewall/filter"))

	addTool(s, mcp.NewTool("firewall_filter_remove",
		mcp.WithDescription("Remove a firewall filter rule by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(api, "/ip/firewall/filter"))

	addTool(s, mcp.NewTool("firewall_nat_list",
		mcp.WithDescription("List firewall NAT rules with optional chain, action, and disabled filters."),
		mcp.WithString("chain", mcp.Description("Filter by chain")),
		mcp.WithString("action", mcp.Description("Filter by action")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), filteredListHandler(api, "/ip/firewall/nat", map[string]string{"chain": "chain", "action": "action", "disabled": "disabled"}))

	addTool(s, mcp.NewTool("firewall_nat_add",
		mcp.WithDescription("Add a firewall NAT rule using RouterOS firewall attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("Firewall NAT attributes")),
	), addHandler(api, "/ip/firewall/nat"))

	addTool(s, mcp.NewTool("firewall_nat_set",
		mcp.WithDescription("Update a firewall NAT rule by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("Firewall NAT attributes")),
	), setHandler(api, "/ip/firewall/nat"))

	addTool(s, mcp.NewTool("firewall_nat_remove",
		mcp.WithDescription("Remove a firewall NAT rule by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(api, "/ip/firewall/nat"))

	addTool(s, mcp.NewTool("firewall_rule_move",
		mcp.WithDescription("Move a firewall filter or NAT rule to a new destination position or item id."),
		mcp.WithString("table", mcp.Required(), mcp.Description("Firewall table: filter or nat")),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id to move")),
		mcp.WithString("destination", mcp.Required(), mcp.Description("Destination position or item id")),
	), firewallRuleMoveHandler(api))

	addTool(s, mcp.NewTool("firewall_address_list_list",
		mcp.WithDescription("List firewall address-list entries with optional list, address, and disabled filters."),
		mcp.WithString("list_name", mcp.Description("Filter by address list name")),
		mcp.WithString("address", mcp.Description("Filter by address")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), filteredListHandler(api, "/ip/firewall/address-list", map[string]string{"list_name": "list", "address": "address", "disabled": "disabled"}))

	addTool(s, mcp.NewTool("firewall_address_list_add",
		mcp.WithDescription("Add a firewall address-list entry using RouterOS firewall attributes."),
		mcp.WithObject("attributes", mcp.Required(), mcp.Description("Address list attributes")),
	), addHandler(api, "/ip/firewall/address-list"))

	addTool(s, mcp.NewTool("firewall_address_list_remove",
		mcp.WithDescription("Remove a firewall address-list entry by RouterOS item id."),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), removeHandler(api, "/ip/firewall/address-list"))
}

func setHandler(api *API, menu string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		itemID, err := helpers.NormalizeRequiredString(argString(req, "item_id", ""), "item_id")
		if err != nil {
			return nil, err
		}
		attrs, err := helpers.RequireAttributes(argObject(req, "attributes"))
		if err != nil {
			return nil, err
		}
		result, err := api.set(req, menu, itemID, attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func firewallRuleMoveHandler(api *API) server.ToolHandlerFunc {
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
		result, err := api.run(req, path, map[string]any{".id": normalizedID, "destination": normalizedDest}, nil)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}
