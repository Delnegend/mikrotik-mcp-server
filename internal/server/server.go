package server

import (
	"context"
	"strings"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/formatting"
	"github.com/Delnegend/mikrotik-mcp/internal/inventory"
	"github.com/Delnegend/mikrotik-mcp/internal/safemode"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// API provides tool handlers with router access: per-request device routing
// for the fleet, a shared client for the single-device case, and safe-mode
// routing for mutations (see tool_safemode.go).
type API struct {
	reg    *inventory.Registry
	shared *client.RouterOSClient // non-nil when exactly one device is configured
	safe   *safemode.Manager
}

func newAPI(reg *inventory.Registry) *API {
	a := &API{reg: reg, safe: safemode.NewManager()}
	if reg.Len() == 1 {
		// Single device: keep one persistent, lazily-connected client (the
		// historical behavior). Multi-device fleets open a fresh connection
		// per command instead.
		d := reg.Default()
		a.shared = d.Client()
	}
	return a
}

func NewMCPServer(reg *inventory.Registry) *server.MCPServer {
	api := newAPI(reg)

	s := server.NewMCPServer(
		"mikrotik",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	registerCoreTools(s, api)
	registerLayer2Tools(s, api)
	registerSecurityTools(s, api)
	registerAccessTools(s, api)
	registerFileTools(s, api)
	registerSafeModeTools(s, api)

	addTool(s, mcp.NewTool("list_devices",
		mcp.WithDescription("List the MikroTik devices this server manages: title, host, port, username, tags, region. Credentials are never returned."),
	), api.listDevices)

	return s
}

// deviceFor resolves the device targeted by the request.
func (a *API) deviceFor(req mcp.CallToolRequest) (inventory.Device, error) {
	title := ""
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		title, _ = m["device"].(string)
	}
	return a.reg.Get(strings.TrimSpace(title))
}

// clientFor resolves the device targeted by the request and returns a client
// plus a done callback. With a single device the shared client is reused and
// done is a no-op; with a fleet, a fresh connection is opened per command and
// done closes it.
func (a *API) clientFor(req mcp.CallToolRequest) (*client.RouterOSClient, func(), error) {
	d, err := a.deviceFor(req)
	if err != nil {
		return nil, nil, err
	}
	if a.shared != nil {
		return a.shared, func() {}, nil
	}
	cl := d.Client()
	if err := cl.Open(); err != nil {
		return nil, nil, err
	}
	return cl, func() { cl.Close() }, nil
}

// add routes an add through the safe-mode CLI session when safe mode is
// active for the target device, otherwise through the API client.
func (a *API) add(req mcp.CallToolRequest, menu string, attrs map[string]any) (map[string]any, error) {
	d, err := a.deviceFor(req)
	if err != nil {
		return nil, err
	}
	if s := a.safe.Session(d.Title); s != nil {
		return s.Add(menu, attrs)
	}
	cl, done, err := a.clientFor(req)
	if err != nil {
		return nil, err
	}
	defer done()
	return cl.Add(menu, attrs)
}

func (a *API) set(req mcp.CallToolRequest, menu, itemID string, attrs map[string]any) (map[string]any, error) {
	d, err := a.deviceFor(req)
	if err != nil {
		return nil, err
	}
	if s := a.safe.Session(d.Title); s != nil {
		return s.Set(menu, itemID, attrs)
	}
	cl, done, err := a.clientFor(req)
	if err != nil {
		return nil, err
	}
	defer done()
	return cl.Set(menu, itemID, attrs)
}

func (a *API) remove(req mcp.CallToolRequest, menu, itemID string) (map[string]any, error) {
	d, err := a.deviceFor(req)
	if err != nil {
		return nil, err
	}
	if s := a.safe.Session(d.Title); s != nil {
		return s.Remove(menu, itemID)
	}
	cl, done, err := a.clientFor(req)
	if err != nil {
		return nil, err
	}
	defer done()
	return cl.Remove(menu, itemID)
}

func (a *API) run(req mcp.CallToolRequest, path string, attrs map[string]any, queries []string) (any, error) {
	d, err := a.deviceFor(req)
	if err != nil {
		return nil, err
	}
	if s := a.safe.Session(d.Title); s != nil {
		return s.Run(path, attrs)
	}
	cl, done, err := a.clientFor(req)
	if err != nil {
		return nil, err
	}
	defer done()
	return cl.Run(path, attrs, queries, "")
}

func (a *API) listDevices(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	records := make([]map[string]any, 0, a.reg.Len())
	for _, d := range a.reg.Devices() {
		records = append(records, map[string]any{
			"title":    d.Title,
			"host":     d.Host,
			"port":     d.Port,
			"username": d.Username,
			"tags":     strings.Join(d.Tags, ", "),
			"region":   d.Region,
		})
	}
	return formatting.CallToolResultFromRecords("Devices", records, "device", [][2]string{
		{"title", "Title"}, {"host", "Host"}, {"port", "Port"},
		{"username", "Username"}, {"tags", "Tags"}, {"region", "Region"},
	})
}
