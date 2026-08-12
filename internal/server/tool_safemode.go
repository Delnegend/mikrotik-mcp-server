package server

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSafeModeTools(s *server.MCPServer, api *API) {
	addTool(s, mcp.NewTool("safe_mode_status",
		mcp.WithDescription("Report whether RouterOS safe mode is active for the device. While active, changes are held in memory and only persisted on commit."),
	), api.safeModeStatus)

	addTool(s, mcp.NewTool("enable_safe_mode",
		mcp.WithDescription("Enable RouterOS safe mode for the device. All subsequent mutating tool calls are held in memory until commit_safe_mode or rollback_safe_mode."),
	), api.enableSafeMode)

	addTool(s, mcp.NewTool("commit_safe_mode",
		mcp.WithDescription("Persist all pending safe-mode changes and leave safe mode."),
	), api.commitSafeMode)

	addTool(s, mcp.NewTool("rollback_safe_mode",
		mcp.WithDescription("Discard all pending safe-mode changes (RouterOS reverts them automatically on disconnect) and leave safe mode."),
	), api.rollbackSafeMode)
}

func (a *API) safeModeStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	d, err := a.deviceFor(req)
	if err != nil {
		return nil, err
	}
	if a.safe.Active(d.Title) {
		return mcp.NewToolResultText("Safe mode is ACTIVE. Changes are pending and are NOT yet persisted. Call commit_safe_mode to persist or rollback_safe_mode to revert."), nil
	}
	return mcp.NewToolResultText("Safe mode is NOT active. Changes take effect and persist immediately."), nil
}

func (a *API) enableSafeMode(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	d, err := a.deviceFor(req)
	if err != nil {
		return nil, err
	}
	if err := a.safe.Enable(d.Title, d); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("Safe mode ENABLED. All changes are temporary until commit_safe_mode or rollback_safe_mode."), nil
}

func (a *API) commitSafeMode(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	d, err := a.deviceFor(req)
	if err != nil {
		return nil, err
	}
	if err := a.safe.Commit(d.Title); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("Changes committed. Safe mode DISABLED."), nil
}

func (a *API) rollbackSafeMode(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	d, err := a.deviceFor(req)
	if err != nil {
		return nil, err
	}
	if err := a.safe.Rollback(d.Title); err != nil {
		return nil, err
	}
	return mcp.NewToolResultText("Safe mode session closed. MikroTik has reverted all uncommitted changes automatically."), nil
}
