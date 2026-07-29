package server

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/pheoxy/mikrotik-mcp/internal/client"
	"github.com/pheoxy/mikrotik-mcp/internal/downloads"
	"github.com/pheoxy/mikrotik-mcp/internal/filters"
	"github.com/pheoxy/mikrotik-mcp/internal/formatting"
	"github.com/pheoxy/mikrotik-mcp/internal/helpers"
)

// scpChecker is an interface for checking SCP connectivity, used by healthcheck.
type scpChecker interface {
	CheckConnection() (map[string]any, error)
}

// Package-level variables for healthcheck dependencies.
// Swappable in tests via save/swap/defer-restore pattern.
var (
	hcLoadFileTransferSettings   = downloads.LoadFileTransferSettings
	hcNewSCPFileDownloader       = func(s *downloads.FileTransferSettings) scpChecker {
		return downloads.NewSCPFileDownloader(s)
	}
	hcProbeSSHFingerprint         = downloads.ProbeSSHFingerprint
	hcCheckPasswordRotationReady = downloads.CheckPasswordRotationReady
	hcResolveSCPPrivateKeyPath    = downloads.ResolveSCPPrivateKeyPath
)

func registerCoreTools(s *server.MCPServer, cl *client.RouterOSClient) {
	s.AddTool(mcp.NewTool("resource_print",
		mcp.WithDescription("Run a generic RouterOS print command and optionally apply jq to the normalized array response. Use slash-separated paths, e.g. \"interface/list\" or \"ip/firewall/nat\"."),
		mcp.WithString("menu", mcp.Required(), mcp.Description("RouterOS menu path (e.g. ip/address, interface/bridge/port)")),
		mcp.WithArray("proplist", mcp.Items(map[string]any{"type": "string"}), mcp.Description("Optional property list")),
		mcp.WithArray("queries", mcp.Items(map[string]any{"type": "string"}), mcp.Description("Optional query filters")),
		mcp.WithObject("attributes", mcp.Description("Optional print attributes")),
		mcp.WithString("jq_filter", mcp.Description("Optional jq filter expression")),
	), handlerResourcePrint(cl))

	// Simple register helper for tools with just a description
	reg := func(name, desc string, handler server.ToolHandlerFunc) {
		s.AddTool(mcp.NewTool(name, mcp.WithDescription(desc)), handler)
	}

	reg("resource_add", "Run a generic RouterOS add command for a menu path. Use slash-separated paths, e.g. \"interface/bridge/port\" or \"ip/firewall/nat\".", handlerResourceAdd(cl))
	reg("resource_set", "Run a generic RouterOS set command for a menu path and item id. Use slash-separated paths, e.g. \"ip/firewall/nat\" or \"interface/bridge/port\".", handlerResourceSet(cl))
	reg("resource_remove", "Run a generic RouterOS remove command for a menu path and item id. Use slash-separated paths, e.g. \"ip/firewall/nat\" or \"interface/bridge/port\".", handlerResourceRemove(cl))
	reg("command_run", "Run a generic RouterOS command path and return normalized output. Use slash-separated paths, e.g. \"/tool/ping\" or \"/system/backup/save\".", handlerCommandRun(cl))
	reg("resource_listen", "Listen for changes on a RouterOS menu and return a bounded batch of events. Use slash-separated paths, e.g. \"interface\" or \"ip/firewall/nat\".", handlerResourceListen(cl))
	reg("command_cancel", "Cancel a tagged long-running RouterOS API command.", handlerCommandCancel(cl))

	// Ping and traceroute have parameters
	s.AddTool(mcp.NewTool("tool_ping",
		mcp.WithDescription("Run a bounded ping from the router and return per-probe results."),
		mcp.WithString("address", mcp.Required(), mcp.Description("Target address")),
		mcp.WithNumber("count", mcp.Description("Number of pings (default 4)")),
		mcp.WithString("interval", mcp.Description("Ping interval")),
		mcp.WithString("interface", mcp.Description("Source interface")),
		mcp.WithNumber("packet_size", mcp.Description("Packet size")),
	), handlerToolPing(cl))

	s.AddTool(mcp.NewTool("tool_traceroute",
		mcp.WithDescription("Run a bounded traceroute from the router and return hop results."),
		mcp.WithString("address", mcp.Required(), mcp.Description("Target address")),
		mcp.WithNumber("count", mcp.Description("Probes per hop (default 3)")),
		mcp.WithNumber("max_hops", mcp.Description("Maximum hops (default 30)")),
		mcp.WithString("interval", mcp.Description("Probe interval")),
		mcp.WithString("interface", mcp.Description("Source interface")),
		mcp.WithNumber("packet_size", mcp.Description("Packet size")),
	), handlerToolTraceroute(cl))

	reg("dns_resolve", "Resolve a DNS name from the router, optionally using a specific DNS server.", handlerDNSResolve(cl))
	reg("interface_monitor", "Run a one-shot interface traffic monitor and return current counters and rates.", handlerInterfaceMonitor(cl))
	reg("system_resource_get", "Get RouterOS system resource details.", handlerSystemResourceGet(cl))
	reg("system_identity_get", "Get the RouterOS system identity.", handlerSystemIdentityGet(cl))
	reg("system_clock_get", "Get the RouterOS system clock settings.", handlerSystemClockGet(cl))
	reg("healthcheck", "Check whether the MCP can fetch RouterOS API data and connect to SCP.", handlerHealthcheck(cl))

	s.AddTool(mcp.NewTool("interface_list",
		mcp.WithDescription("List network interfaces with optional status filters."),
		mcp.WithBoolean("running_only", mcp.Description("Only return running interfaces")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), handlerInterfaceList(cl))

	s.AddTool(mcp.NewTool("interface_get",
		mcp.WithDescription("Get one interface by name or RouterOS item id."),
		mcp.WithString("name", mcp.Description("Interface name")),
		mcp.WithString("item_id", mcp.Description("RouterOS item id")),
	), handlerInterfaceGet(cl))

	s.AddTool(mcp.NewTool("ip_address_list",
		mcp.WithDescription("List IP addresses with optional interface and disabled filters."),
		mcp.WithString("interface", mcp.Description("Filter by interface")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), handlerIPAddressList(cl))

	s.AddTool(mcp.NewTool("ip_address_get",
		mcp.WithDescription("Get one IP address by address or RouterOS item id."),
		mcp.WithString("address", mcp.Description("IP address")),
		mcp.WithString("item_id", mcp.Description("RouterOS item id")),
	), handlerIPAddressGet(cl))

	s.AddTool(mcp.NewTool("ip_route_list",
		mcp.WithDescription("List IP routes with optional destination and disabled filters."),
		mcp.WithString("dst_address", mcp.Description("Filter by destination address")),
		mcp.WithBoolean("disabled", mcp.Description("Filter by disabled state")),
	), handlerIPRouteList(cl))

	s.AddTool(mcp.NewTool("ip_route_get",
		mcp.WithDescription("Get one IP route by destination or RouterOS item id."),
		mcp.WithString("dst_address", mcp.Description("Destination address")),
		mcp.WithString("item_id", mcp.Description("RouterOS item id")),
	), handlerIPRouteGet(cl))

	s.AddTool(mcp.NewTool("dhcp_lease_list",
		mcp.WithDescription("List DHCP leases with optional address, MAC, and active filters."),
		mcp.WithString("address", mcp.Description("Filter by address")),
		mcp.WithString("mac_address", mcp.Description("Filter by MAC address")),
		mcp.WithBoolean("active_only", mcp.Description("Only return active (bound) leases")),
	), handlerDHCPLeaseList(cl))

	reg("dhcp_server_list", "List configured DHCP servers.", handlerDHCPServerList(cl))
	reg("dhcp_network_list", "List configured DHCP networks.", handlerDHCPNetworkList(cl))
	reg("dns_get", "Get RouterOS DNS settings.", handlerDNSGet(cl))

	s.AddTool(mcp.NewTool("dns_set",
		mcp.WithDescription("Update RouterOS DNS settings."),
		mcp.WithArray("servers", mcp.Items(map[string]any{"type": "string"}), mcp.Description("DNS server addresses")),
		mcp.WithBoolean("allow_remote_requests", mcp.Description("Allow remote DNS requests")),
		mcp.WithString("cache_size", mcp.Description("DNS cache size")),
	), handlerDNSSet(cl))
}

// ---- Helper functions for argument extraction ----

func argString(req mcp.CallToolRequest, key, defaultVal string) string {
	return mcp.ParseString(req, key, defaultVal)
}

func argStringSlice(req mcp.CallToolRequest, key string) []string {
	v, ok := req.Params.Arguments[key]
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
	v, ok := req.Params.Arguments[key]
	if !ok {
		return nil
	}
	b, ok := v.(bool)
	if !ok {
		return nil
	}
	return &b
}

// ---- Handler implementations ----

func handlerResourcePrint(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		menu := argString(req, "menu", "")
		proplist := argStringSlice(req, "proplist")
		queries := argStringSlice(req, "queries")
		attrs := req.Params.Arguments
		jqFilter := argString(req, "jq_filter", "")

		items, err := helpers.PrintRecords(cl, menu, proplist, queries, attrs)
		if err != nil {
			return nil, err
		}

		if jqFilter != "" {
			var anyItems []any
			for _, item := range items {
				m := make(map[string]any)
				for k, v := range item {
					m[k] = v
				}
				anyItems = append(anyItems, m)
			}
			filtered, err := filters.ApplyJQFilter(anyItems, jqFilter)
			if err != nil {
				return nil, err
			}
			return mcp.NewToolResultText(fmt.Sprintf("%v", filtered)), nil
		}

		return mcp.NewToolResultText(helpers.JSONCompact(items)), nil
	}
}

func handlerResourceAdd(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		menu := argString(req, "menu", "")
		attrs := req.Params.Arguments
		result, err := cl.Add(menu, attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func handlerResourceSet(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		menu := argString(req, "menu", "")
		itemID := argString(req, "item_id", "")
		attrs := req.Params.Arguments
		result, err := cl.Set(menu, itemID, attrs)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func handlerResourceRemove(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		menu := argString(req, "menu", "")
		itemID := argString(req, "item_id", "")
		result, err := cl.Remove(menu, itemID)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func handlerCommandRun(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		command := argString(req, "command", "")
		queries := argStringSlice(req, "queries")
		result, err := cl.Run(command, req.Params.Arguments, queries, "")
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func handlerResourceListen(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		menu := argString(req, "menu", "")
		proplist := argStringSlice(req, "proplist")
		queries := argStringSlice(req, "queries")
		attrs := req.Params.Arguments
		tag := argString(req, "tag", "")
		maxEvents := int(argFloat(req, "max_events", 10))
		if maxEvents < 1 {
			maxEvents = 10
		}

		var result *client.ListenResult
		err := cl.Isolated(func(iso *client.RouterOSClient) error {
			var listenErr error
			result, listenErr = iso.Listen(menu, proplist, queries, attrs, tag, maxEvents)
			return listenErr
		})
		if err != nil {
			return nil, err
		}

		text := fmt.Sprintf("tag=%s events=%d cancelled=%v", result.Tag, len(result.Records), result.Cancelled)
		return mcp.NewToolResultText(text), nil
	}
}

func handlerCommandCancel(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tag := argString(req, "tag", "")
		result, err := cl.Cancel(tag)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func handlerToolPing(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		if packetSize > 0 && packetSize < 1 {
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
		err := cl.Isolated(func(iso *client.RouterOSClient) error {
			result, runErr := iso.Run("/tool/ping", attrs, nil, "")
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

		var anyItems []map[string]any
		for _, r := range records {
			m := make(map[string]any)
			for k, v := range r {
				m[k] = v
			}
			anyItems = append(anyItems, m)
		}

		return formatting.CallToolResultFromRecords("Ping "+address, anyItems, "probe",
			[][2]string{
				{"seq", "Seq"}, {"host", "Host"}, {"size", "Size"},
				{"ttl", "TTL"}, {"time", "Time"}, {"status", "Status"},
			})
	}
}

func handlerToolTraceroute(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		if _, ok := req.Params.Arguments["packet_size"]; ok && packetSize < 1 {
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
		err := cl.Isolated(func(iso *client.RouterOSClient) error {
			result, runErr := iso.Run("/tool/traceroute", attrs, nil, "")
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

		var anyItems []map[string]any
		for _, r := range records {
			m := make(map[string]any)
			for k, v := range r {
				m[k] = v
			}
			anyItems = append(anyItems, m)
		}

		return formatting.CallToolResultFromRecords("Traceroute "+address, anyItems, "hop",
			[][2]string{
				{"hop", "Hop"}, {"host", "Host"}, {"address", "Address"},
				{"loss", "Loss"}, {"last", "Last"}, {"avg", "Avg"},
				{"best", "Best"}, {"worst", "Worst"}, {"status", "Status"},
			})
	}
}

func handlerDNSResolve(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := argString(req, "name", "")
		server := argString(req, "server", "")

		attrs := map[string]any{"domain-name": name}
		if server != "" {
			attrs["server"] = server
		}

		var result any
		err := cl.Isolated(func(iso *client.RouterOSClient) error {
			var runErr error
			result, runErr = iso.Run("/resolve", attrs, nil, "")
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

func handlerInterfaceMonitor(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := argString(req, "name", "")
		attrs := map[string]any{"interface": name, "once": true}

		var result any
		err := cl.Isolated(func(iso *client.RouterOSClient) error {
			var runErr error
			result, runErr = iso.Run("/interface/monitor-traffic", attrs, nil, "")
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
			for k, v := range m {
				data[k] = v
			}
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

func handlerSystemResourceGet(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		record, err := helpers.PrintSingleRecord(cl, "/system/resource", nil, nil, "system resource")
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

func handlerSystemIdentityGet(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		record, err := helpers.PrintSingleRecord(cl, "/system/identity", nil, nil, "system identity")
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

func handlerSystemClockGet(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		record, err := helpers.PrintSingleRecord(cl, "/system/clock", nil, nil, "system clock")
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

func handlerHealthcheck(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			"api_passwordless_enabled":  helpers.ParseBool(os.Getenv("MIKROTIK_API_PASSWORDLESS_ENABLED"), false),
			"api_host":                  cl.Host(),
			"api_port":                  cl.Port(),
			"api_tls":                   cl.UseSSL(),
			"resolved_scp_host":         os.Getenv("MIKROTIK_SCP_HOST"),
			"scp_credentials_configured": os.Getenv("MIKROTIK_SCP_USER") != "" || os.Getenv("MIKROTIK_USER") != "",
		}
		if scpKey := hcResolveSCPPrivateKeyPath(); scpKey != "" {
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
		return formatting.CallToolResultFromRecord("Healthcheck", title, data,
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

	identity, err := helpers.PrintSingleRecord(cl, "/system/identity", nil, nil, "system identity")
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

	settings, err := hcLoadFileTransferSettings(cl.Host())
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

	check, err := hcNewSCPFileDownloader(settings).CheckConnection()
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
	fingerprint, err := hcProbeSSHFingerprint(host, port, 10*time.Second)
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
			"ok":     true,
			"status": "skipped",
			"code":   "passwordless.disabled",
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
			"ok":     false,
			"status": "failed",
			"code":   "passwordless.key_required",
			"message": "MIKROTIK_SCP_PRIVATE_KEY must be set when API passwordless startup rotation is enabled",
			"duration_ms": time.Since(startedAt).Milliseconds(),
		}
	}

	if !scpOK {
		return map[string]any{
			"ok":     false,
			"status": "failed",
			"code":   "passwordless.ssh_unavailable",
			"message": fmt.Sprintf("SSH bootstrap is unavailable: %v", scpResult["message"]),
			"duration_ms": time.Since(startedAt).Milliseconds(),
		}
	}

	targetUser := os.Getenv("MIKROTIK_USER")
	probe, err := hcCheckPasswordRotationReady(cl.Host(), targetUser)
	if err != nil {
		return map[string]any{
			"ok":     false,
			"status": "failed",
			"code":   "passwordless.exec_failed",
			"message": err.Error(),
			"duration_ms": time.Since(startedAt).Milliseconds(),
		}
	}

	return map[string]any{
		"ok":     true,
		"status": "ok",
		"code":   "passwordless.ok",
		"message": fmt.Sprintf("Passwordless startup rotation SSH command succeeded for %s:%d", probe["host"], probe["port"]),
		"probe": probe,
		"duration_ms": time.Since(startedAt).Milliseconds(),
	}
}

func classifyAPIError(err error) string {
	errStr := err.Error()
	if contains(errStr, "login") || contains(errStr, "auth") {
		return "api.auth_failed"
	}
	if contains(errStr, "connect") || contains(errStr, "timeout") {
		return "api.connect_failed"
	}
	if contains(errStr, "routeros: fatal") {
		return "api.fatal"
	}
	return "api.error"
}

func classifySCPError(err error) string {
	errStr := err.Error()
	if contains(errStr, "MIKROTIK_SCP_PRIVATE_KEY") || contains(errStr, "must be set") {
		return "scp.config_missing"
	}
	if contains(errStr, "authentication failed") || contains(errStr, "auth") {
		return "scp.auth_failed"
	}
	if contains(errStr, "directory probe failed") || contains(errStr, "ReadDir") {
		return "scp.operation_failed"
	}
	if contains(errStr, "connect") {
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

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func handlerInterfaceList(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		items, err := helpers.PrintRecords(cl, "/interface", nil, queries, nil)
		if err != nil {
			return nil, err
		}

		var anyItems []map[string]any
		for _, item := range items {
			m := make(map[string]any)
			for k, v := range item {
				m[k] = v
			}
			anyItems = append(anyItems, m)
		}

		return formatting.CallToolResultFromRecords("Interfaces", anyItems, "interface",
			[][2]string{
				{"name", "Name"}, {"type", "Type"}, {"running", "Running"},
				{"disabled", "Disabled"}, {"actual-mtu", "MTU"}, {"mac-address", "MAC Address"},
			})
	}
}

func handlerInterfaceGet(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		record, err := helpers.PrintSingleRecord(cl, "/interface", []string{queryField + "=" + value}, nil, "interface")
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

func handlerIPAddressList(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		items, err := helpers.PrintRecords(cl, "/ip/address", nil, queries, nil)
		if err != nil {
			return nil, err
		}

		var anyItems []map[string]any
		for _, item := range items {
			m := make(map[string]any)
			for k, v := range item {
				m[k] = v
			}
			anyItems = append(anyItems, m)
		}

		return formatting.CallToolResultFromRecords("IP Addresses", anyItems, "IP address",
			[][2]string{
				{"address", "Address"}, {"interface", "Interface"}, {"network", "Network"},
				{"disabled", "Disabled"}, {"dynamic", "Dynamic"},
			})
	}
}

func handlerIPAddressGet(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		record, err := helpers.PrintSingleRecord(cl, "/ip/address", []string{queryField + "=" + value}, nil, "IP address")
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

func handlerIPRouteList(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		items, err := helpers.PrintRecords(cl, "/ip/route", nil, queries, nil)
		if err != nil {
			return nil, err
		}

		var anyItems []map[string]any
		for _, item := range items {
			m := make(map[string]any)
			for k, v := range item {
				m[k] = v
			}
			anyItems = append(anyItems, m)
		}

		return formatting.CallToolResultFromRecords("IP Routes", anyItems, "IP route",
			[][2]string{
				{"dst-address", "Destination"}, {"gateway", "Gateway"}, {"distance", "Distance"},
				{"active", "Active"}, {"static", "Static"}, {"disabled", "Disabled"},
			})
	}
}

func handlerIPRouteGet(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		record, err := helpers.PrintSingleRecord(cl, "/ip/route", []string{queryField + "=" + value}, nil, "IP route")
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

func handlerDHCPLeaseList(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		items, err := helpers.PrintRecords(cl, "/ip/dhcp-server/lease", nil, queries, nil)
		if err != nil {
			return nil, err
		}

		var anyItems []map[string]any
		for _, item := range items {
			m := make(map[string]any)
			for k, v := range item {
				m[k] = v
			}
			anyItems = append(anyItems, m)
		}

		return formatting.CallToolResultFromRecords("DHCP Leases", anyItems, "DHCP lease",
			[][2]string{
				{"address", "Address"}, {"mac-address", "MAC Address"}, {"host-name", "Host Name"},
				{"status", "Status"}, {"server", "Server"}, {"expires-after", "Expires After"},
			})
	}
}

func handlerDHCPServerList(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		items, err := helpers.PrintRecords(cl, "/ip/dhcp-server", nil, nil, nil)
		if err != nil {
			return nil, err
		}
		var anyItems []map[string]any
		for _, item := range items {
			m := make(map[string]any)
			for k, v := range item {
				m[k] = v
			}
			anyItems = append(anyItems, m)
		}
		return formatting.CallToolResultFromRecords("DHCP Servers", anyItems, "DHCP server",
			[][2]string{
				{"name", "Name"}, {"interface", "Interface"}, {"address-pool", "Address Pool"},
				{"lease-time", "Lease Time"}, {"disabled", "Disabled"},
			})
	}
}

func handlerDHCPNetworkList(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		items, err := helpers.PrintRecords(cl, "/ip/dhcp-server/network", nil, nil, nil)
		if err != nil {
			return nil, err
		}
		var anyItems []map[string]any
		for _, item := range items {
			m := make(map[string]any)
			for k, v := range item {
				m[k] = v
			}
			anyItems = append(anyItems, m)
		}
		return formatting.CallToolResultFromRecords("DHCP Networks", anyItems, "DHCP network",
			[][2]string{
				{"address", "Address"}, {"gateway", "Gateway"}, {"dns-server", "DNS Server"},
				{"domain", "Domain"}, {"ntp-server", "NTP Server"},
			})
	}
}

func handlerDNSGet(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		record, err := helpers.PrintSingleRecord(cl, "/ip/dns", nil, nil, "DNS settings")
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

func handlerDNSSet(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		servers := argStringSlice(req, "servers")
		allowRemote := argBoolNullable(req, "allow_remote_requests")
		cacheSize := argString(req, "cache_size", "")

		attrs := map[string]any{}
		if len(servers) > 0 {
			attrs["servers"] = servers
		}
		if allowRemote != nil {
			attrs["allow-remote-requests"] = *allowRemote
		}
		if cacheSize != "" {
			attrs["cache-size"] = cacheSize
		}
		if len(attrs) == 0 {
			return mcp.NewToolResultError("At least one DNS setting must be provided"), nil
		}

		result, err := cl.Run("/ip/dns/set", attrs, nil, "")
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}
