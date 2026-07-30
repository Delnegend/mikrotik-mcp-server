package server

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/pheoxy/mikrotik-mcp/internal/client"
	"github.com/pheoxy/mikrotik-mcp/internal/downloads"
	"github.com/pheoxy/mikrotik-mcp/internal/helpers"
)

// Package-level variables for file tool dependencies, swappable in tests.
var (
	ftLoadFileTransferSettings = downloads.LoadFileTransferSettings
	ftNewSCPFileDownloader     = func(s *downloads.FileTransferSettings) *downloads.SCPFileDownloader {
		return downloads.NewSCPFileDownloader(s)
	}
)

func registerFileTools(s *server.MCPServer, cl *client.RouterOSClient) {
	addTool(s, mcp.NewTool("system_backup_save",
		mcp.WithDescription("Create a RouterOS backup file on the router."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Backup name (without extension)")),
	), backupSaveHandler(cl))

	addTool(s, mcp.NewTool("system_export",
		mcp.WithDescription("Export RouterOS configuration to an .rsc file on the router."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Export file name (without extension)")),
		mcp.WithBoolean("include_sensitive", mcp.Description("Include sensitive data")),
		mcp.WithBoolean("compact", mcp.Description("Compact export")),
	), exportHandler(cl))

	addTool(s, mcp.NewTool("file_download",
		mcp.WithDescription("Download a router file into the local workspace."),
		mcp.WithString("router_path", mcp.Required(), mcp.Description("Path of the file on the router")),
		mcp.WithString("local_path", mcp.Description("Local destination path")),
	), downloadHandler(cl))

	addTool(s, mcp.NewTool("system_backup_collect",
		mcp.WithDescription("Create router backup artifacts and download them into the local workspace."),
		mcp.WithString("name_prefix", mcp.Description("Prefix for backup filenames")),
		mcp.WithBoolean("include_sensitive", mcp.Description("Include sensitive data in export")),
		mcp.WithBoolean("compact", mcp.Description("Compact export")),
		mcp.WithString("local_dir", mcp.Description("Local download directory")),
	), backupCollectHandler(cl))

	addTool(s, mcp.NewTool("file_list",
		mcp.WithDescription("List router files with optional directory, name, and type filters."),
		mcp.WithString("directory", mcp.Description("Filter by directory")),
		mcp.WithString("name", mcp.Description("Filter by file name")),
		mcp.WithString("file_type", mcp.Description("Filter by file type (e.g. script, backup, export)")),
	), filteredListHandler(cl, "/file", map[string]string{"name": "name", "file_type": "type"}))
}

func backupSaveHandler(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := argString(req, "name", "")
		normalized, err := helpers.NormalizeGeneratedName(name, ".backup", "name")
		if err != nil {
			return nil, err
		}
		_, err = cl.Run("/system/backup/save", map[string]any{"name": normalized}, nil, "")
		if err != nil {
			return nil, err
		}
		result := map[string]any{
			"success": true,
			"name":    normalized,
			"path":    normalized + ".backup",
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func exportHandler(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := argString(req, "name", "")
		includeSensitive := argBool(req, "include_sensitive", false)
		compact := argBool(req, "compact", false)

		normalized, err := helpers.NormalizeGeneratedName(name, ".rsc", "name")
		if err != nil {
			return nil, err
		}

		attrs := map[string]any{"file": normalized}
		if includeSensitive {
			attrs["show-sensitive"] = ""
		}
		if compact {
			attrs["compact"] = ""
		}

		_, err = cl.Run("/export", attrs, nil, "")
		if err != nil {
			return nil, err
		}

		result := map[string]any{
			"success":           true,
			"name":              normalized,
			"path":              normalized + ".rsc",
			"include_sensitive": includeSensitive,
			"compact":           compact,
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func downloadHandler(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		routerPath := argString(req, "router_path", "")
		localPath := argString(req, "local_path", "")

		normalized, err := helpers.NormalizeRouterFilePath(routerPath)
		if err != nil {
			return nil, err
		}

		settings, err := ftLoadFileTransferSettings(cl.Host())
		if err != nil {
			return nil, err
		}

		targetPath := localPath
		if targetPath == "" {
			targetPath = helpers.UniqueLocalPath(
				filepath.Join(helpers.WorkspaceRoot(), "backups"),
				filepath.Base(normalized),
			)
		} else if !filepath.IsAbs(targetPath) {
			targetPath = filepath.Join(helpers.WorkspaceRoot(), targetPath)
		}

		downloader := ftNewSCPFileDownloader(settings)
		if err := downloader.DownloadFile(normalized, targetPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Download failed: %v", err)), nil
		}

		result := map[string]any{
			"success":     true,
			"router_path": normalized,
			"local_path":  targetPath,
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}

func backupCollectHandler(cl *client.RouterOSClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namePrefix := argString(req, "name_prefix", "")
		includeSensitive := argBool(req, "include_sensitive", false)
		compact := argBool(req, "compact", false)
		localDirArg := argString(req, "local_dir", "")

		prefix := namePrefix
		if prefix == "" {
			prefix = "backup"
		}
		timestamp := time.Now().UTC().Format("20060102T150405Z")
		routerSlug := helpers.SafeNameComponent(cl.Host(), "router")
		backupName := fmt.Sprintf("backups/%s-%s", prefix, timestamp)

		normalized, _ := helpers.NormalizeGeneratedName(backupName, ".backup", "name")
		_, err := cl.Run("/system/backup/save", map[string]any{"name": normalized}, nil, "")
		if err != nil {
			return nil, err
		}
		routerBackupPath := normalized + ".backup"

		exportName := fmt.Sprintf("backups/%s-%s", prefix, timestamp)
		exportNorm, _ := helpers.NormalizeGeneratedName(exportName, ".rsc", "name")
		exportAttrs := map[string]any{"file": exportNorm}
		if includeSensitive {
			exportAttrs["show-sensitive"] = ""
		}
		if compact {
			exportAttrs["compact"] = ""
		}
		_, err = cl.Run("/export", exportAttrs, nil, "")
		if err != nil {
			return nil, fmt.Errorf("backup created at '%s' but export creation failed: %v", routerBackupPath, err)
		}
		routerExportPath := exportNorm + ".rsc"

		// Verify files exist on router
		backupFiles, err := helpers.PrintRecords(cl, "/file", nil, nil, nil)
		if err == nil {
			available := make(map[string]bool)
			for _, f := range backupFiles {
				available[f["name"]] = true
			}
			var missing []string
			if !available[routerBackupPath] {
				missing = append(missing, routerBackupPath)
			}
			if !available[routerExportPath] {
				missing = append(missing, routerExportPath)
			}
			if len(missing) > 0 {
				return nil, fmt.Errorf("expected router backup files were not found: %s", missing)
			}
		}

		// Download via SFTP
		settings, sftpErr := ftLoadFileTransferSettings(cl.Host())
		if sftpErr != nil {
			return nil, fmt.Errorf("backup files created but download config failed: %v", sftpErr)
		}

		downloader := ftNewSCPFileDownloader(settings)

		localDir, _ := helpers.NormalizeLocalDirectory(localDirArg)
		localBackup := helpers.UniqueLocalPath(localDir, fmt.Sprintf("%s-%s-%s.backup", routerSlug, prefix, timestamp))
		localExport := helpers.UniqueLocalPath(localDir, fmt.Sprintf("%s-%s-%s.rsc", routerSlug, prefix, timestamp))

		if err := downloader.DownloadFile(routerBackupPath, localBackup); err != nil {
			return nil, fmt.Errorf("backup files created but local download failed (backup): %v", err)
		}
		if err := downloader.DownloadFile(routerExportPath, localExport); err != nil {
			return nil, fmt.Errorf("backup files created but local download failed (export): %v", err)
		}

		result := map[string]any{
			"success":             true,
			"created_at":          timestamp,
			"router_backup_path":  routerBackupPath,
			"router_export_path":  routerExportPath,
			"local_backup_path":   localBackup,
			"local_export_path":   localExport,
			"include_sensitive":   includeSensitive,
			"compact":             compact,
		}
		return mcp.NewToolResultText(helpers.JSONCompact(result)), nil
	}
}
