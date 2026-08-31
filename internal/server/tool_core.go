package server

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/downloads"
	"github.com/Delnegend/mikrotik-mcp/internal/formatting"
	"github.com/Delnegend/mikrotik-mcp/internal/helpers"
	"github.com/itchyny/gojq"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerCoreTools(s *server.MCPServer, api *API) {
	addTool(s, mcp.NewTool("resource_print",
		mcp.WithDescription("Run a generic RouterOS print command and optionally apply jq to the normalized array response. Use slash-separated paths, e.g. \"interface/list\" or \"ip/firewall/nat\"."),
		mcp.WithString("menu", mcp.Required(), mcp.Description("RouterOS menu path (e.g. ip/address, interface/bridge/port)")),
		mcp.WithArray("proplist", mcp.Items(map[string]any{"type": "string"}), mcp.Description("Optional property list")),
		mcp.WithArray("queries", mcp.Items(map[string]any{"type": "string"}), mcp.Description("Optional query filters")),
		mcp.WithObject("attributes", mcp.Description("Optional print attributes")),
		mcp.WithString("jq_filter", mcp.Description("Optional jq filter expression")),
	), handlerResourcePrint(api))

	addTool(s, mcp.NewTool("resource_add",
		mcp.WithDescription("Run a generic RouterOS add command for a menu path. Use slash-separated paths, e.g. \"interface/bridge/port\" or \"ip/firewall/nat\"."),
		mcp.WithString("menu", mcp.Required(), mcp.Description("RouterOS menu path (e.g. ip/address, interface/bridge/port)")),
		mcp.WithObject("attributes", mcp.Description("Optional RouterOS attributes")),
	), handlerResourceAdd(api))

	addTool(s, mcp.NewTool("resource_set",
		mcp.WithDescription("Run a generic RouterOS set command for a menu path and item id. Use slash-separated paths, e.g. \"ip/firewall/nat\" or \"interface/bridge/port\"."),
		mcp.WithString("menu", mcp.Required(), mcp.Description("RouterOS menu path (e.g. ip/address, interface/bridge/port)")),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
		mcp.WithObject("attributes", mcp.Description("Optional RouterOS attributes")),
	), handlerResourceSet(api))

	addTool(s, mcp.NewTool("resource_remove",
		mcp.WithDescription("Run a generic RouterOS remove command for a menu path and item id. Use slash-separated paths, e.g. \"ip/firewall/nat\" or \"interface/bridge/port\"."),
		mcp.WithString("menu", mcp.Required(), mcp.Description("RouterOS menu path (e.g. ip/address, interface/bridge/port)")),
		mcp.WithString("item_id", mcp.Required(), mcp.Description("RouterOS item id")),
	), handlerResourceRemove(api))

	addTool(s, mcp.NewTool("command_run",
		mcp.WithDescription("Run a generic RouterOS command path and return normalized output. Use slash-separated paths, e.g. \"/tool/ping\" or \"/system/backup/save\"."),
		mcp.WithString("command", mcp.Required(), mcp.Description("RouterOS command path")),
		mcp.WithObject("attributes", mcp.Description("Optional command attributes")),
		mcp.WithArray("queries", mcp.Items(map[string]any{"type": "string"}), mcp.Description("Optional query filters")),
	), handlerCommandRun(api))

	addTool(s, mcp.NewTool("resource_listen",
		mcp.WithDescription("Listen for changes on a RouterOS menu and return a bounded batch of events. Use slash-separated paths, e.g. \"interface\" or \"ip/firewall/nat\"."),
		mcp.WithString("menu", mcp.Required(), mcp.Description("RouterOS menu path (e.g. interface, ip/firewall/nat)")),
		mcp.WithArray("proplist", mcp.Items(map[string]any{"type": "string"}), mcp.Description("Optional property list")),
		mcp.WithArray("queries", mcp.Items(map[string]any{"type": "string"}), mcp.Description("Optional query filters")),
		mcp.WithObject("attributes", mcp.Description("Optional listen attributes")),
		mcp.WithString("tag", mcp.Description("Optional tag for the listen session")),
		mcp.WithNumber("max_events", mcp.Description("Maximum number of events to collect (default 10)")),
	), handlerResourceListen(api))

	addTool(s, mcp.NewTool("command_cancel",
		mcp.WithDescription("Cancel a tagged long-running RouterOS API command."),
		mcp.WithString("tag", mcp.Required(), mcp.Description("Tag of the command to cancel")),
	), handlerCommandCancel(api))

	// Ping and traceroute have parameters
	addTool(s, mcp.NewTool("tool_ping",
		mcp.WithDescription("Run a bounded ping from the router and return per-probe results."),
		mcp.WithString("address", mcp.Required(), mcp.Description("Target address")),
		mcp.WithNumber("count", mcp.Description("Number of pings (default 4)")),
		mcp.WithString("interval", mcp.Description("Ping interval")),
		mcp.WithString("interface", mcp.Description("Source interface")),
		mcp.WithNumber("packet_size", mcp.Description("Packet size")),
	), handlerToolPing(api))

	addTool(s, mcp.NewTool("tool_traceroute",
		mcp.WithDescription("Run a bounded traceroute from the router and return hop results."),
		mcp.WithString("address", mcp.Required(), mcp.Description("Target address")),
		mcp.WithNumber("count", mcp.Description("Probes per hop (default 3)")),
		mcp.WithNumber("max_hops", mcp.Description("Maximum hops (default 30)")),
		mcp.WithString("interval", mcp.Description("Probe interval")),
		mcp.WithString("interface", mcp.Description("Source interface")),
		mcp.WithNumber("packet_size", mcp.Description("Packet size")),
	), handlerToolTraceroute(api))

	addTool(s, mcp.NewTool("dns_resolve",
		mcp.WithDescription("Resolve a DNS name from the router, optionally using a specific DNS server."),
		mcp.WithString("name", mcp.Required(), mcp.Description("DNS name to resolve")),
		mcp.WithString("server", mcp.Description("Optional DNS server to use")),
	), handlerDNSResolve(api))

	addTool(s, mcp.NewTool("interface_monitor",
		mcp.WithDescription("Run a one-shot interface traffic monitor and return current counters and rates."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Interface name")),
	), handlerInterfaceMonitor(api))

	addTool(s, mcp.NewTool("system_resource_get",
		mcp.WithDescription("Get RouterOS system resource details."),
	), handlerSystemResourceGet(api))

	addTool(s, mcp.NewTool("system_identity_get",
		mcp.WithDescription("Get the RouterOS system identity."),
	), handlerSystemIdentityGet(api))

	addTool(s, mcp.NewTool("system_clock_get",
		mcp.WithDescription("Get the RouterOS system clock settings."),
	), handlerSystemClockGet(api))

	addTool(s, mcp.NewTool("healthcheck",
		mcp.WithDescription("Check whether the MCP can fetch RouterOS API data and connect to SCP."),
	), handlerHealthcheck(api))

	addTool(s, mcp.NewTool("interface_list",
		mcp.WithDescription("List network interfaces with optional status filters."),
		mcp.WithBoolean("running_only", mcp.Description("Only return running interfaces")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), handlerInterfaceList(api))

	addTool(s, mcp.NewTool("interface_get",
		mcp.WithDescription("Get one interface by name or RouterOS item id."),
		mcp.WithString("name", mcp.Description("Interface name")),
		mcp.WithString("item_id", mcp.Description("RouterOS item id")),
	), handlerInterfaceGet(api))

	addTool(s, mcp.NewTool("ip_address_list",
		mcp.WithDescription("List IP addresses with optional interface and disabled filters."),
		mcp.WithString("interface", mcp.Description("Filter by interface")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), handlerIPAddressList(api))

	addTool(s, mcp.NewTool("ip_address_get",
		mcp.WithDescription("Get one IP address by address or RouterOS item id."),
		mcp.WithString("address", mcp.Description("IP address")),
		mcp.WithString("item_id", mcp.Description("RouterOS item id")),
	), handlerIPAddressGet(api))

	addTool(s, mcp.NewTool("ip_route_list",
		mcp.WithDescription("List IP routes with optional destination and disabled filters."),
		mcp.WithString("dst_address", mcp.Description("Filter by destination address")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), handlerIPRouteList(api))

	addTool(s, mcp.NewTool("ip_route_get",
		mcp.WithDescription("Get one IP route by destination or RouterOS item id."),
		mcp.WithString("dst_address", mcp.Description("Destination address")),
		mcp.WithString("item_id", mcp.Description("RouterOS item id")),
	), handlerIPRouteGet(api))

	addTool(s, mcp.NewTool("dhcp_lease_list",
		mcp.WithDescription("List DHCP leases with optional address, MAC, and active filters."),
		mcp.WithString("address", mcp.Description("Filter by address")),
		mcp.WithString("mac_address", mcp.Description("Filter by MAC address")),
		mcp.WithBoolean("active_only", mcp.Description("Only return active (bound) leases")),
	), handlerDHCPLeaseList(api))

	addTool(s, mcp.NewTool("dhcp_server_list",
		mcp.WithDescription("List configured DHCP servers."),
	), handlerDHCPServerList(api))

	addTool(s, mcp.NewTool("dhcp_network_list",
		mcp.WithDescription("List configured DHCP networks."),
	), handlerDHCPNetworkList(api))

	addTool(s, mcp.NewTool("dns_get",
		mcp.WithDescription("Get RouterOS DNS settings."),
	), handlerDNSGet(api))

	addTool(s, mcp.NewTool("dns_set",
		mcp.WithDescription("Update RouterOS DNS settings."),
		mcp.WithArray("servers", mcp.Items(map[string]any{"type": "string"}), mcp.Description("DNS server addresses")),
		mcp.WithBoolean("allow_remote_requests", mcp.Description("Allow remote DNS requests")),
		mcp.WithString("cache_size", mcp.Description("DNS cache size")),
	), handlerDNSSet(api))
}

// ---- Helper functions for argument extraction ----

func argMap(req mcp.CallToolRequest) map[string]any {
	m, _ := req.Params.Arguments.(map[string]any)
	result := make(map[string]any, len(m))
	for k, v := range m {
		if k == "device" {
			continue // control arg, never sent to the router
		}
		result[k] = v
	}
	return result
}

func argString(req mcp.CallToolRequest, key, defaultVal string) string {
	return mcp.ParseString(req, key, defaultVal)
}

func argStringSlice(req mcp.CallToolRequest, key string) []string {
	v, ok := argMap(req)[key]
	if !ok {
		return nil
	}
	switch arr := v.(type) {
	case []any:
		result := make([]string, 0, len(arr))
		for _, e := range arr {
			result = append(result, fmt.Sprint(e))
		}
		return result
	case []string:
		return arr
	}
	return nil
}

func argFloat(req mcp.CallToolRequest, key string, defaultVal float64) float64 {
	return mcp.ParseFloat64(req, key, defaultVal)
}

func argBool(req mcp.CallToolRequest, key string, defaultVal bool) bool {
	return mcp.ParseBoolean(req, key, defaultVal)
}

func argBoolNullable(req mcp.CallToolRequest, key string) *bool {
	v, ok := argMap(req)[key]
	if !ok {
		return nil
	}
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
}

func argObject(req mcp.CallToolRequest, key string) map[string]any {
	v, ok := argMap(req)[key]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return m
}

func argAttributes(req mcp.CallToolRequest, controlKeys ...string) map[string]any {
	if attrs := argObject(req, "attributes"); attrs != nil {
		return attrs
	}
	return stripControlKeys(argMap(req), controlKeys...)
}

func printSingleRecord(cl *client.RouterOSClient, menu string, queries []string, attrs map[string]any, entityName string) (map[string]string, error) {
	items, err := cl.Print(menu, nil, queries, attrs)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no matching %s found", entityName)
	}
	if len(items) > 1 {
		return nil, fmt.Errorf("multiple %s records matched", entityName)
	}
	return items[0], nil
}

func applyJQFilter(payload any, jqFilter string) (any, error) {
	query, err := gojq.Parse(jqFilter)
	if err != nil {
		return nil, fmt.Errorf("invalid jq_filter: %v", err)
	}

	iter := query.Run(payload)
	var results []any
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return nil, fmt.Errorf("jq_filter evaluation failed: %v", err)
		}
		results = append(results, v)
	}

	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}

func addTool(s *server.MCPServer, tool mcp.Tool, handler server.ToolHandlerFunc) {
	s.AddTool(tool, recoverHandler(handler))
}

func recoverHandler(h server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("internal error: %v", r)
			}
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		return h(ctx, req)
	}
}

func stripControlKeys(attrs map[string]any, keys ...string) map[string]any {
	result := make(map[string]any, len(attrs))
	maps.Copy(result, attrs)
	for _, k := range keys {
		delete(result, k)
	}
	return result
}

// ---- Handler implementations ----

func handlerResourcePrint(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		menu := argString(req, "menu", "")
		proplist := argStringSlice(req, "proplist")
		queries := argStringSlice(req, "queries")
		attrs := argAttributes(req, "menu", "proplist", "queries", "jq_filter")
		jqFilter := argString(req, "jq_filter", "")

		items, err := cl.Print(menu, proplist, queries, attrs)
		if err != nil {
			return nil, err
		}

		if jqFilter != "" {
			var anyItems []any
			for _, m := range helpers.RecordsAsAny(items) {
				anyItems = append(anyItems, m)
			}
			filtered, err := applyJQFilter(anyItems, jqFilter)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResultText(helpers.JSONCompact(filtered)), nil
		}

		return mcp.NewToolResultText(helpers.JSONCompact(items)), nil
	}
}

func handlerResourceAdd(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		menu := argString(req, "menu", "")
		attrs := argAttributes(req, "menu")
		result, err := api.add(req, menu, attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func handlerResourceSet(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		menu := argString(req, "menu", "")
		itemID, err := helpers.NormalizeRequiredString(argString(req, "item_id", ""), "item_id")
		if err != nil {
			return nil, err
		}
		attrs := argAttributes(req, "menu", "item_id")
		result, err := api.set(req, menu, itemID, attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func handlerResourceRemove(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		menu := argString(req, "menu", "")
		itemID, err := helpers.NormalizeRequiredString(argString(req, "item_id", ""), "item_id")
		if err != nil {
			return nil, err
		}
		result, err := api.remove(req, menu, itemID)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func handlerCommandRun(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		command := argString(req, "command", "")
		queries := argStringSlice(req, "queries")
		attrs := argAttributes(req, "command", "queries")
		result, err := api.run(req, command, attrs, queries)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func handlerResourceListen(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		menu := argString(req, "menu", "")
		proplist := argStringSlice(req, "proplist")
		queries := argStringSlice(req, "queries")
		attrs := argAttributes(req, "menu", "proplist", "queries", "tag", "max_events")
		tag := argString(req, "tag", "")
		maxEvents := int(argFloat(req, "max_events", 10))
		if maxEvents < 1 {
			maxEvents = 10
		}

		var result *client.ListenResult
		err = cl.Isolated(func(iso *client.RouterOSClient) error {
			var listenErr error
			result, listenErr = iso.ListenContext(ctx, menu, proplist, queries, attrs, tag, maxEvents)
			return listenErr
		})
		if err != nil {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		records := helpers.RecordsAsAny(result.Records)
		text := fmt.Sprintf("tag=%s events=%d cancelled=%v\n\n%s",
			result.Tag, len(result.Records), result.Cancelled, helpers.JSONCompact(records))
		out := mcp.NewToolResultText(text)
		out.Meta = mcp.NewMetaFromMap(map[string]any{
			"structuredContent": map[string]any{
				"tag":       result.Tag,
				"cancelled": result.Cancelled,
				"events":    records,
			},
		})
		return out, nil
	}
}

func handlerCommandCancel(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		tag, err := helpers.NormalizeRequiredString(argString(req, "tag", ""), "tag")
		if err != nil {
			return nil, err
		}
		result, err := cl.Cancel(tag)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func handlerToolPing(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		address := argString(req, "address", "")
		count := int(argFloat(req, "count", 4))
		interval := argString(req, "interval", "")
		iface := argString(req, "interface", "")
		packetSize := int(argFloat(req, "packet_size", 0))

		if strings.TrimSpace(address) == "" {
			return nil, fmt.Errorf("address is required")
		}
		if count < 1 {
			return nil, fmt.Errorf("count must be at least 1")
		}
		if interval != "" && strings.TrimSpace(interval) == "" {
			return nil, fmt.Errorf("interval is required")
		}
		if _, ok := argMap(req)["packet_size"]; ok && packetSize < 1 {
			return nil, fmt.Errorf("packet_size must be at least 1")
		}

		attrs := map[string]any{"address": address, "count": count}
		if interval != "" {
			attrs["interval"] = interval
		}
		if iface != "" {
			attrs["interface"] = iface
		}
		if packetSize > 0 {
			attrs["size"] = packetSize
		}

		var records []map[string]string
		err = cl.Isolated(func(iso *client.RouterOSClient) error {
			result, runErr := iso.RunContext(ctx, "/tool/ping", attrs, nil, "")
			if runErr != nil {
				return runErr
			}
			if recs, ok := result.([]map[string]string); ok {
				records = recs
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		return formatting.CallToolResultFromRecords("Ping "+address, helpers.RecordsAsAny(records), "probe",
			[][2]string{
				{"seq", "Seq"}, {"host", "Host"}, {"size", "Size"},
				{"ttl", "TTL"}, {"time", "Time"}, {"status", "Status"},
			})
	}
}

func handlerToolTraceroute(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		address := argString(req, "address", "")
		count := int(argFloat(req, "count", 3))
		maxHops := int(argFloat(req, "max_hops", 30))
		interval := argString(req, "interval", "")
		iface := argString(req, "interface", "")
		packetSize := int(argFloat(req, "packet_size", 0))

		if strings.TrimSpace(address) == "" {
			return nil, fmt.Errorf("address is required")
		}
		if count < 1 {
			return nil, fmt.Errorf("count must be at least 1")
		}
		if maxHops < 1 {
			return nil, fmt.Errorf("max_hops must be at least 1")
		}
		if interval != "" && strings.TrimSpace(interval) == "" {
			return nil, fmt.Errorf("interval is required")
		}
		if _, ok := argMap(req)["packet_size"]; ok && packetSize < 1 {
			return nil, fmt.Errorf("packet_size must be at least 1")
		}

		attrs := map[string]any{"address": address, "count": count, "max-hops": maxHops}
		if interval != "" {
			attrs["interval"] = interval
		}
		if iface != "" {
			attrs["interface"] = iface
		}
		if packetSize > 0 {
			attrs["size"] = packetSize
		}

		var records []map[string]string
		err = cl.Isolated(func(iso *client.RouterOSClient) error {
			result, runErr := iso.RunContext(ctx, "/tool/traceroute", attrs, nil, "")
			if runErr != nil {
				return runErr
			}
			if recs, ok := result.([]map[string]string); ok {
				records = recs
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		return formatting.CallToolResultFromRecords("Traceroute "+address, helpers.RecordsAsAny(records), "hop",
			[][2]string{
				{"hop", "Hop"}, {"host", "Host"}, {"address", "Address"},
				{"loss", "Loss"}, {"last", "Last"}, {"avg", "Avg"},
				{"best", "Best"}, {"worst", "Worst"}, {"status", "Status"},
			})
	}
}

func handlerDNSResolve(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		name := argString(req, "name", "")
		server := argString(req, "server", "")

		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("name is required")
		}

		attrs := map[string]any{"domain-name": name}
		if server != "" {
			attrs["server"] = server
		}

		var result any
		err = cl.Isolated(func(iso *client.RouterOSClient) error {
			var runErr error
			result, runErr = iso.RunContext(ctx, "/resolve", attrs, nil, "")
			return runErr
		})
		if err != nil {
			return nil, err
		}

		resultMap, ok := result.(map[string]any)
		if !ok {
			return mcp.NewToolResultError("RouterOS resolve command did not return a single result"), nil
		}

		address := ""
		if v, ok := resultMap["ret"]; ok {
			address = fmt.Sprint(v)
		} else if v, ok := resultMap["address"]; ok {
			address = fmt.Sprint(v)
		}
		if address == "" {
			return mcp.NewToolResultError("RouterOS resolve command did not return an address"), nil
		}

		resolved := map[string]any{"name": name, "address": address}
		if server != "" {
			resolved["server"] = server
		}

		return formatting.CallToolResultFromRecord("DNS Resolve",
			fmt.Sprintf("DNS resolve: %s -> %s", name, address),
			resolved, []string{"name", "address", "server"})
	}
}

func handlerInterfaceMonitor(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		name := argString(req, "name", "")
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("name is required")
		}
		attrs := map[string]any{"interface": name, "once": true}

		var result any
		err = cl.Isolated(func(iso *client.RouterOSClient) error {
			var runErr error
			result, runErr = iso.RunContext(ctx, "/interface/monitor-traffic", attrs, nil, "")
			return runErr
		})
		if err != nil {
			return nil, err
		}

		data := map[string]any{"name": name}
		if recs, ok := result.([]map[string]string); ok && len(recs) > 0 {
			for k, v := range recs[0] {
				data[k] = v
			}
		} else if m, ok := result.(map[string]any); ok {
			maps.Copy(data, m)
		}

		rxRate := fmt.Sprint(data["rx-bits-per-second"])
		txRate := fmt.Sprint(data["tx-bits-per-second"])
		return formatting.CallToolResultFromRecord("Interface Monitor",
			fmt.Sprintf("Interface monitor %s: rx %s, tx %s", name, rxRate, txRate),
			data,
			[]string{"name", "status", "rx-bits-per-second", "tx-bits-per-second",
				"rx-packets-per-second", "tx-packets-per-second", "rx-byte", "tx-byte"})
	}
}

func handlerSystemResourceGet(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		record, err := printSingleRecord(cl, "/system/resource", nil, nil, "system resource")
		if err != nil {
			return nil, err
		}
		data := helpers.ValuesAsAny(record)
		version := fmt.Sprint(data["version"])
		uptime := fmt.Sprint(data["uptime"])
		return formatting.CallToolResultFromRecord("System Resource",
			fmt.Sprintf("System resource: RouterOS %s, uptime %s", version, uptime),
			data,
			[]string{"platform", "board-name", "version", "uptime", "cpu-load",
				"free-memory", "total-memory", "free-hdd-space", "total-hdd-space"})
	}
}

func handlerSystemIdentityGet(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		record, err := printSingleRecord(cl, "/system/identity", nil, nil, "system identity")
		if err != nil {
			return nil, err
		}
		data := helpers.ValuesAsAny(record)
		name := fmt.Sprint(data["name"])
		return formatting.CallToolResultFromRecord("System Identity",
			fmt.Sprintf("System identity: %s", name),
			data, []string{"name"})
	}
}

func handlerSystemClockGet(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		record, err := printSingleRecord(cl, "/system/clock", nil, nil, "system clock")
		if err != nil {
			return nil, err
		}
		data := helpers.ValuesAsAny(record)
		date := fmt.Sprint(data["date"])
		clockTime := fmt.Sprint(data["time"])
		return formatting.CallToolResultFromRecord("System Clock",
			fmt.Sprintf("System clock: %s %s", date, clockTime),
			data,
			[]string{"date", "time", "time-zone-name", "gmt-offset", "time-zone-autodetect"})
	}
}

func handlerHealthcheck(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		timestamp := time.Now().UTC().Format(time.RFC3339)
		data := map[string]any{
			"timestamp":   timestamp,
			"target_host": cl.Host(),
		}

		// --- API health ---
		apiResult := probeAPI(cl)

		// --- SCP/SFTP health ---
		scpResult := probeSCP(cl)

		// --- Passwordless health ---
		passwordlessResult := probePasswordless(cl, scpResult)

		// --- Config info ---
		config := map[string]any{
			"api_credentials_configured": cl.Host() != "" && os.Getenv("MIKROTIK_USER") != "",
			"api_passwordless_enabled":   helpers.ParseBool(os.Getenv("MIKROTIK_API_PASSWORDLESS_ENABLED"), false),
			"api_host":                   cl.Host(),
			"api_port":                   cl.Port(),
			"api_tls":                    cl.UseSSL(),
			"resolved_scp_host":          os.Getenv("MIKROTIK_SCP_HOST"),
			"scp_credentials_configured": os.Getenv("MIKROTIK_SCP_USER") != "" || os.Getenv("MIKROTIK_USER") != "",
		}
		if scpKey, keyErr := downloads.ResolveSCPPrivateKeyPath(); keyErr != nil {
			config["scp_key_path_error"] = keyErr.Error()
		} else if scpKey != "" {
			config["scp_key_path"] = scpKey
			config["scp_auth_mode"] = "key"
		} else if os.Getenv("MIKROTIK_SCP_PASSWORD") != "" || os.Getenv("MIKROTIK_PASSWORD") != "" {
			config["scp_auth_mode"] = "password"
		}
		fingerprint := os.Getenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256")
		config["scp_host_fingerprint_verification"] = fingerprint != ""
		if fingerprint == "" {
			config["scp_host_fingerprint_warning"] = "SSH host fingerprint verification is disabled"
		}

		data["api"] = apiResult
		data["scp"] = scpResult
		data["passwordless"] = passwordlessResult
		data["config"] = config

		// --- Overall status ---
		apiOK := false
		if status, ok := apiResult["ok"]; ok {
			apiOK, _ = status.(bool)
		}
		scpOK := false
		if status, ok := scpResult["ok"]; ok {
			scpOK, _ = status.(bool)
		}
		pwdOK := false
		if status, ok := passwordlessResult["ok"]; ok {
			pwdOK, _ = status.(bool)
		}
		pwdEnabled := helpers.ParseBool(os.Getenv("MIKROTIK_API_PASSWORDLESS_ENABLED"), false)

		overallStatus := classifyOverallHealth(apiOK, scpOK, pwdEnabled, pwdOK)
		data["success"] = overallStatus == "healthy"
		data["status"] = overallStatus

		title := fmt.Sprintf("Healthcheck: %s", overallStatus)
		return formatting.FormatHealthcheckResult(title, data,
			[]string{"success", "status", "timestamp", "target_host"})
	}
}

func probeAPI(cl *client.RouterOSClient) map[string]any {
	startedAt := time.Now()
	result := map[string]any{
		"host": cl.Host(),
		"port": cl.Port(),
		"tls":  cl.UseSSL(),
	}

	identity, err := printSingleRecord(cl, "/system/identity", nil, nil, "system identity")
	if err != nil {
		result["ok"] = false
		result["status"] = "failed"
		result["code"] = classifyAPIError(err)
		result["message"] = err.Error()
		result["duration_ms"] = time.Since(startedAt).Milliseconds()
		return result
	}

	result["ok"] = true
	result["status"] = "ok"
	result["code"] = "api.ok"
	result["message"] = "RouterOS API returned system identity"
	result["identity"] = helpers.ValuesAsAny(identity)
	result["duration_ms"] = time.Since(startedAt).Milliseconds()

	tlsInfo := cl.TLSSessionInfo()
	if tlsInfo != nil {
		result["certificate"] = tlsInfo
	}

	return result
}

func probeSCP(cl *client.RouterOSClient) map[string]any {
	startedAt := time.Now()
	result := map[string]any{}

	settings, err := downloads.LoadFileTransferSettings(cl.Host())
	if err != nil {
		result["ok"] = false
		result["status"] = "failed"
		result["code"] = classifySCPError(err)
		result["message"] = err.Error()
		result["duration_ms"] = time.Since(startedAt).Milliseconds()

		scpHost := cl.Host()
		scpPort := 22
		if h := os.Getenv("MIKROTIK_SCP_HOST"); h != "" {
			scpHost = h
		}
		if p := os.Getenv("MIKROTIK_SCP_PORT"); p != "" {
			if v, err := fmt.Sscanf(p, "%d", &scpPort); err == nil {
				_ = v
			}
		}
		result["host"] = scpHost
		result["port"] = scpPort
		result["server_identity"] = probeSSHServerIdentity(scpHost, scpPort)
		return result
	}

	check, err := downloads.NewSCPFileDownloader(settings).CheckConnection()
	if err != nil {
		result["ok"] = false
		result["status"] = "failed"
		result["code"] = classifySCPError(err)
		result["message"] = err.Error()
	} else {
		result["ok"] = true
		result["status"] = "ok"
		result["code"] = "scp.ok"
		result["message"] = fmt.Sprintf("SCP login and directory probe succeeded for %s:%d", settings.Host, settings.Port)
		result["probe"] = check
	}

	result["host"] = settings.Host
	result["port"] = settings.Port
	result["duration_ms"] = time.Since(startedAt).Milliseconds()
	result["server_identity"] = probeSSHServerIdentity(settings.Host, settings.Port)
	return result
}

func probeSSHServerIdentity(host string, port int) map[string]any {
	fingerprint, err := downloads.ProbeSSHFingerprint(host, port, 10*time.Second)
	if err != nil {
		return map[string]any{
			"status":  "failed",
			"message": err.Error(),
		}
	}
	return fingerprint
}

func probePasswordless(cl *client.RouterOSClient, scpResult map[string]any) map[string]any {
	pwdEnabled := helpers.ParseBool(os.Getenv("MIKROTIK_API_PASSWORDLESS_ENABLED"), false)
	if !pwdEnabled {
		return map[string]any{
			"ok":      true,
			"status":  "skipped",
			"code":    "passwordless.disabled",
			"message": "Passwordless startup rotation is disabled",
		}
	}

	startedAt := time.Now()

	scpOK := false
	if ok, exists := scpResult["ok"]; exists {
		scpOK, _ = ok.(bool)
	}

	scpAuthMode := "unknown"
	if s := os.Getenv("MIKROTIK_SCP_PRIVATE_KEY"); s != "" {
		scpAuthMode = "key"
	} else if os.Getenv("MIKROTIK_SCP_PASSWORD") != "" || os.Getenv("MIKROTIK_PASSWORD") != "" {
		scpAuthMode = "password"
	}

	if scpAuthMode != "key" {
		return map[string]any{
			"ok":          false,
			"status":      "failed",
			"code":        "passwordless.key_required",
			"message":     "MIKROTIK_SCP_PRIVATE_KEY must be set when API passwordless startup rotation is enabled",
			"duration_ms": time.Since(startedAt).Milliseconds(),
		}
	}

	if !scpOK {
		return map[string]any{
			"ok":          false,
			"status":      "failed",
			"code":        "passwordless.ssh_unavailable",
			"message":     fmt.Sprintf("SSH bootstrap is unavailable: %v", scpResult["message"]),
			"duration_ms": time.Since(startedAt).Milliseconds(),
		}
	}

	targetUser := os.Getenv("MIKROTIK_USER")
	probe, err := downloads.CheckPasswordRotationReady(cl.Host(), targetUser)
	if err != nil {
		return map[string]any{
			"ok":          false,
			"status":      "failed",
			"code":        "passwordless.exec_failed",
			"message":     err.Error(),
			"duration_ms": time.Since(startedAt).Milliseconds(),
		}
	}

	return map[string]any{
		"ok":          true,
		"status":      "ok",
		"code":        "passwordless.ok",
		"message":     fmt.Sprintf("Passwordless startup rotation SSH command succeeded for %s:%d", probe["host"], probe["port"]),
		"probe":       probe,
		"duration_ms": time.Since(startedAt).Milliseconds(),
	}
}

func classifyAPIError(err error) string {
	if rosErr, ok := errors.AsType[*client.RouterOSError](err); ok {
		if strings.Contains(rosErr.Message, "user name or password") || strings.Contains(rosErr.Message, "login") {
			return "api.auth_failed"
		}
	}
	switch {
	case errors.Is(err, client.ErrRouterOSAuthError):
		return "api.auth_failed"
	case errors.Is(err, client.ErrRouterOSTransportError):
		return "api.connect_failed"
	case errors.Is(err, client.ErrRouterOSFatalError):
		return "api.fatal"
	}
	errStr := err.Error()
	if strings.Contains(errStr, "login") || strings.Contains(errStr, "auth") {
		return "api.auth_failed"
	}
	if strings.Contains(errStr, "connect") || strings.Contains(errStr, "timeout") {
		return "api.connect_failed"
	}
	if strings.Contains(errStr, "routeros: fatal") {
		return "api.fatal"
	}
	return "api.error"
}

func classifySCPError(err error) string {
	switch {
	case errors.Is(err, downloads.ErrSCPConfigMissing):
		return "scp.config_missing"
	case errors.Is(err, downloads.ErrSCPAuthFailed):
		return "scp.auth_failed"
	case errors.Is(err, downloads.ErrSCPConnectFailed):
		return "scp.connect_failed"
	case errors.Is(err, downloads.ErrSCPOperation):
		return "scp.operation_failed"
	}
	errStr := err.Error()
	if strings.Contains(errStr, "MIKROTIK_SCP_PRIVATE_KEY") || strings.Contains(errStr, "must be set") {
		return "scp.config_missing"
	}
	if strings.Contains(errStr, "authentication failed") || strings.Contains(errStr, "auth") {
		return "scp.auth_failed"
	}
	if strings.Contains(errStr, "directory probe failed") || strings.Contains(errStr, "ReadDir") {
		return "scp.operation_failed"
	}
	if strings.Contains(errStr, "connect") {
		return "scp.connect_failed"
	}
	return "scp.error"
}

func classifyOverallHealth(apiOK, scpOK, pwdEnabled, pwdOK bool) string {
	pwdRequiredOK := !pwdEnabled || pwdOK
	if apiOK && scpOK && pwdRequiredOK {
		return "healthy"
	}
	if apiOK || scpOK || (pwdEnabled && pwdOK) {
		return "degraded"
	}
	return "failed"
}

func handlerInterfaceList(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		runningOnly := argBool(req, "running_only", false)
		disabled := argBoolNullable(req, "disabled")

		filters := map[string]any{}
		if disabled != nil {
			filters["disabled"] = *disabled
		}
		queries := helpers.BuildEqualityQueries(filters)
		if runningOnly {
			queries = append(queries, "running=true")
		}

		items, err := cl.Print("/interface", nil, queries, nil)
		if err != nil {
			return nil, err
		}

		anyItems := helpers.RecordsAsAny(items)

		return formatting.CallToolResultFromRecords("Interfaces", anyItems, "interface",
			[][2]string{
				{"name", "Name"}, {"type", "Type"}, {"running", "Running"},
				{"disabled", "Disabled"}, {"actual-mtu", "MTU"}, {"mac-address", "MAC Address"},
			})
	}
}

func handlerInterfaceGet(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		name := argString(req, "name", "")
		itemID := argString(req, "item_id", "")

		field, value, err := helpers.RequireExactlyOneLocator("interface", map[string]string{"name": name, "item_id": itemID})
		if err != nil {
			return nil, err
		}
		queryField := "name"
		if field == "item_id" {
			queryField = ".id"
		}

		record, err := printSingleRecord(cl, "/interface", []string{queryField + "=" + value}, nil, "interface")
		if err != nil {
			return nil, err
		}
		data := helpers.ValuesAsAny(record)
		ifName := fmt.Sprint(data["name"])
		return formatting.CallToolResultFromRecord("Interface",
			fmt.Sprintf("Interface: %s", ifName), data,
			[]string{"name", "type", "running", "disabled", "actual-mtu", "mac-address", "comment"})
	}
}

func handlerIPAddressList(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		iface := argString(req, "interface", "")
		disabled := argBoolNullable(req, "disabled")

		filters := map[string]any{}
		if iface != "" {
			filters["interface"] = iface
		}
		if disabled != nil {
			filters["disabled"] = *disabled
		}
		queries := helpers.BuildEqualityQueries(filters)

		items, err := cl.Print("/ip/address", nil, queries, nil)
		if err != nil {
			return nil, err
		}

		anyItems := helpers.RecordsAsAny(items)

		return formatting.CallToolResultFromRecords("IP Addresses", anyItems, "IP address",
			[][2]string{
				{"address", "Address"}, {"interface", "Interface"}, {"network", "Network"},
				{"disabled", "Disabled"}, {"dynamic", "Dynamic"},
			})
	}
}

func handlerIPAddressGet(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		address := argString(req, "address", "")
		itemID := argString(req, "item_id", "")

		field, value, err := helpers.RequireExactlyOneLocator("IP address", map[string]string{"address": address, "item_id": itemID})
		if err != nil {
			return nil, err
		}
		queryField := "address"
		if field == "item_id" {
			queryField = ".id"
		}

		record, err := printSingleRecord(cl, "/ip/address", []string{queryField + "=" + value}, nil, "IP address")
		if err != nil {
			return nil, err
		}
		data := helpers.ValuesAsAny(record)
		addr := fmt.Sprint(data["address"])
		return formatting.CallToolResultFromRecord("IP Address",
			fmt.Sprintf("IP address: %s", addr), data,
			[]string{"address", "interface", "network", "disabled", "dynamic", "comment"})
	}
}

func handlerIPRouteList(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		dstAddr := argString(req, "dst_address", "")
		disabled := argBoolNullable(req, "disabled")

		filters := map[string]any{}
		if dstAddr != "" {
			filters["dst-address"] = dstAddr
		}
		if disabled != nil {
			filters["disabled"] = *disabled
		}
		queries := helpers.BuildEqualityQueries(filters)

		items, err := cl.Print("/ip/route", nil, queries, nil)
		if err != nil {
			return nil, err
		}

		anyItems := helpers.RecordsAsAny(items)

		return formatting.CallToolResultFromRecords("IP Routes", anyItems, "IP route",
			[][2]string{
				{"dst-address", "Destination"}, {"gateway", "Gateway"}, {"distance", "Distance"},
				{"active", "Active"}, {"static", "Static"}, {"disabled", "Disabled"},
			})
	}
}

func handlerIPRouteGet(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		dstAddr := argString(req, "dst_address", "")
		itemID := argString(req, "item_id", "")

		field, value, err := helpers.RequireExactlyOneLocator("IP route", map[string]string{"dst_address": dstAddr, "item_id": itemID})
		if err != nil {
			return nil, err
		}
		queryField := "dst-address"
		if field == "item_id" {
			queryField = ".id"
		}

		record, err := printSingleRecord(cl, "/ip/route", []string{queryField + "=" + value}, nil, "IP route")
		if err != nil {
			return nil, err
		}
		data := helpers.ValuesAsAny(record)
		dst := fmt.Sprint(data["dst-address"])
		gw := fmt.Sprint(data["gateway"])
		return formatting.CallToolResultFromRecord("IP Route",
			fmt.Sprintf("IP route: %s via %s", dst, gw), data,
			[]string{"dst-address", "gateway", "distance", "disabled", "active", "static", "comment"})
	}
}

func handlerDHCPLeaseList(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		address := argString(req, "address", "")
		macAddr := argString(req, "mac_address", "")
		activeOnly := argBool(req, "active_only", false)

		filters := map[string]any{}
		if address != "" {
			filters["address"] = address
		}
		if macAddr != "" {
			filters["mac-address"] = macAddr
		}
		queries := helpers.BuildEqualityQueries(filters)
		if activeOnly {
			queries = append(queries, "status=bound")
		}

		items, err := cl.Print("/ip/dhcp-server/lease", nil, queries, nil)
		if err != nil {
			return nil, err
		}

		anyItems := helpers.RecordsAsAny(items)

		return formatting.CallToolResultFromRecords("DHCP Leases", anyItems, "DHCP lease",
			[][2]string{
				{"address", "Address"}, {"mac-address", "MAC Address"}, {"host-name", "Host Name"},
				{"status", "Status"}, {"server", "Server"}, {"expires-after", "Expires After"},
			})
	}
}

func handlerDHCPServerList(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		items, err := cl.Print("/ip/dhcp-server", nil, nil, nil)
		if err != nil {
			return nil, err
		}
		anyItems := helpers.RecordsAsAny(items)
		return formatting.CallToolResultFromRecords("DHCP Servers", anyItems, "DHCP server",
			[][2]string{
				{"name", "Name"}, {"interface", "Interface"}, {"address-pool", "Address Pool"},
				{"lease-time", "Lease Time"}, {"disabled", "Disabled"},
			})
	}
}

func handlerDHCPNetworkList(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		items, err := cl.Print("/ip/dhcp-server/network", nil, nil, nil)
		if err != nil {
			return nil, err
		}
		anyItems := helpers.RecordsAsAny(items)
		return formatting.CallToolResultFromRecords("DHCP Networks", anyItems, "DHCP network",
			[][2]string{
				{"address", "Address"}, {"gateway", "Gateway"}, {"dns-server", "DNS Server"},
				{"domain", "Domain"}, {"ntp-server", "NTP Server"},
			})
	}
}

func handlerDNSGet(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cl, done, err := api.clientFor(req)
		if err != nil {
			return nil, err
		}
		defer done()
		record, err := printSingleRecord(cl, "/ip/dns", nil, nil, "DNS settings")
		if err != nil {
			return nil, err
		}
		data := helpers.ValuesAsAny(record)
		servers := fmt.Sprint(data["servers"])
		remoteReq := fmt.Sprint(data["allow-remote-requests"])
		return formatting.CallToolResultFromRecord("DNS Settings",
			fmt.Sprintf("DNS settings: servers %s, remote requests %s", servers, remoteReq),
			data,
			[]string{"servers", "allow-remote-requests", "cache-size", "dynamic-servers", "use-doh-server"})
	}
}

func handlerDNSSet(api *API) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		servers := argStringSlice(req, "servers")
		allowRemote := argBoolNullable(req, "allow_remote_requests")
		cacheSize := argString(req, "cache_size", "")

		attrs := map[string]any{}
		if len(servers) > 0 {
			var cleaned []string
			for _, s := range servers {
				s = strings.TrimSpace(s)
				if s != "" {
					cleaned = append(cleaned, s)
				}
			}
			if len(cleaned) == 0 {
				return mcp.NewToolResultError("At least one DNS server must be provided"), nil
			}
			attrs["servers"] = strings.Join(cleaned, ",")
		}
		if allowRemote != nil {
			attrs["allow-remote-requests"] = *allowRemote
		}
		cacheSize = strings.TrimSpace(cacheSize)
		if cacheSize != "" {
			attrs["cache-size"] = cacheSize
		}
		if len(attrs) == 0 {
			return mcp.NewToolResultError("At least one DNS setting must be provided"), nil
		}

		result, err := api.run(req, "/ip/dns/set", attrs, nil)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}
