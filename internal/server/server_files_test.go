package server

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/downloads"
	"github.com/Delnegend/mikrotik-mcp/internal/testutil"
)

func TestIntegrationFileList(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=test.rsc", "=type=script"), enc("!done"))
	cl.SetConn(fc)
	result, err := listHandler(cl, "/file")(ctx(), testutil.MkReq("file_list"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertResult(t, result)
}

func TestIntegrationFileListFiltersByType(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!re", "=name=script.rsc", "=type=script"), enc("!done"))
	cl.SetConn(fc)
	_, err := fileListHandler(cl)(ctx(), testutil.MkReq("file_list", "file_type", "script"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	assertSent(t, fc, "?type=script")
}

func TestIntegrationFileListRejectsEmptyDirectory(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := fileListHandler(cl)(ctx(), testutil.MkReq("file_list", "directory", ""))
	if err != nil {
		t.Fatalf("unexpected error with empty directory arg: %v", err)
	}
}

func TestIntegrationFileListFiltersByDirectory(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!re", "=name=backups/nightly.backup", "=type=backup"),
		enc("!re", "=name=scripts/setup.rsc", "=type=script"),
		enc("!done"),
	)
	cl.SetConn(fc)

	result, err := fileListHandler(cl)(ctx(), testutil.MkReq("file_list", "directory", "backups"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := resultText(result)
	if !containsAll(text, "nightly.backup") {
		t.Errorf("expected backup file in directory filter result: %s", text)
	}
	if strings.Contains(text, "setup.rsc") {
		t.Errorf("file outside directory should be filtered out: %s", text)
	}
}

func TestIntegrationFileDownloadExplicitPath(t *testing.T) {
	origLoad := ftLoadFileTransferSettings
	origNew := ftNewSCPFileDownloader
	ftLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22, Username: "admin", Password: "pass"}, nil
	}
	ftNewSCPFileDownloader = func(s *downloads.FileTransferSettings) fileDownloader {
		return &mockDownloader{}
	}
	defer func() {
		ftLoadFileTransferSettings = origLoad
		ftNewSCPFileDownloader = origNew
	}()

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)

	localDir := t.TempDir()
	localPath := filepath.Join(localDir, "downloaded.backup")
	result, err := downloadHandler(cl)(ctx(), testutil.MkReq("file_download", "router_path", "test.backup", "local_path", localPath))
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
	assertResult(t, result)
	if !containsAll(resultText(result), "success", "downloaded.backup") {
		t.Errorf("unexpected result: %s", resultText(result))
	}
}

func TestIntegrationFileDownloadResolvesRelative(t *testing.T) {
	origLoad := ftLoadFileTransferSettings
	origNew := ftNewSCPFileDownloader
	ftLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22, Username: "admin", Password: "pass"}, nil
	}
	ftNewSCPFileDownloader = func(s *downloads.FileTransferSettings) fileDownloader {
		return &mockDownloader{}
	}
	defer func() {
		ftLoadFileTransferSettings = origLoad
		ftNewSCPFileDownloader = origNew
	}()

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(enc("!done"))
	cl.SetConn(fc)
	_, err := downloadHandler(cl)(ctx(), testutil.MkReq("file_download", "router_path", "test.backup", "local_path", "relative-dir/downloaded.backup"))
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
}

func TestIntegrationBackupCollectBothArtifacts(t *testing.T) {
	origLoad := ftLoadFileTransferSettings
	origNew := ftNewSCPFileDownloader
	ftLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22, Username: "admin", Password: "pass"}, nil
	}
	ftNewSCPFileDownloader = func(s *downloads.FileTransferSettings) fileDownloader {
		return &mockDownloader{}
	}
	defer func() {
		ftLoadFileTransferSettings = origLoad
		ftNewSCPFileDownloader = origNew
	}()

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	now := time.Now().UTC().Format("20060102T150405Z")
	backupFile := fmt.Sprintf("backups/test-%s.backup", now)
	exportFile := fmt.Sprintf("backups/test-%s.rsc", now)
	fc := newFakeConn(
		enc("!re", "=name=backups", "=type=directory"), enc("!done"),
		enc("!done"), enc("!done"),
		enc("!re", "=name="+backupFile, "=type=backup"),
		enc("!re", "=name="+exportFile, "=type=script"),
		enc("!done"),
	)
	cl.SetConn(fc)
	_, err := backupCollectHandler(cl)(ctx(), testutil.MkReq("system_backup_collect", "name_prefix", "test"))
	if err != nil {
		t.Fatalf("backup collect error: %v", err)
	}
}

func TestIntegrationBackupCollectUsesCustomLocalDir(t *testing.T) {
	origLoad := ftLoadFileTransferSettings
	origNew := ftNewSCPFileDownloader
	ftLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22, Username: "admin", Password: "pass"}, nil
	}
	ftNewSCPFileDownloader = func(s *downloads.FileTransferSettings) fileDownloader {
		return &mockDownloader{}
	}
	defer func() {
		ftLoadFileTransferSettings = origLoad
		ftNewSCPFileDownloader = origNew
	}()

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	now := time.Now().UTC().Format("20060102T150405Z")
	backupFile := fmt.Sprintf("backups/custom-%s.backup", now)
	exportFile := fmt.Sprintf("backups/custom-%s.rsc", now)
	fc := newFakeConn(
		enc("!re", "=name=backups", "=type=directory"), enc("!done"),
		enc("!done"), enc("!done"),
		enc("!re", "=name="+backupFile, "=type=backup"),
		enc("!re", "=name="+exportFile, "=type=script"),
		enc("!done"),
	)
	cl.SetConn(fc)
	localDir := t.TempDir()
	_, err := backupCollectHandler(cl)(ctx(), testutil.MkReq("system_backup_collect", "name_prefix", "custom", "local_dir", localDir))
	if err != nil {
		t.Fatalf("backup collect error: %v", err)
	}
}

func TestIntegrationBackupCollectSkipsDirectoryCreation(t *testing.T) {
	origLoad := ftLoadFileTransferSettings
	origNew := ftNewSCPFileDownloader
	ftLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22, Username: "admin", Password: "pass"}, nil
	}
	ftNewSCPFileDownloader = func(s *downloads.FileTransferSettings) fileDownloader {
		return &mockDownloader{}
	}
	defer func() {
		ftLoadFileTransferSettings = origLoad
		ftNewSCPFileDownloader = origNew
	}()

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	now := time.Now().UTC().Format("20060102T150405Z")
	backupFile := fmt.Sprintf("backups/test-%s.backup", now)
	exportFile := fmt.Sprintf("backups/test-%s.rsc", now)
	fc := newFakeConn(
		enc("!re", "=name=backups", "=type=directory"), enc("!done"),
		enc("!done"), enc("!done"),
		enc("!re", "=name="+backupFile, "=type=backup"),
		enc("!re", "=name="+exportFile, "=type=script"),
		enc("!done"),
	)
	cl.SetConn(fc)
	_, err := backupCollectHandler(cl)(ctx(), testutil.MkReq("system_backup_collect", "name_prefix", "test"))
	if err != nil {
		t.Fatalf("backup collect error: %v", err)
	}
	sent := string(fc.Sent())
	if strings.Contains(sent, "/file/add") {
		t.Errorf("expected no /file/add when backups dir exists, got: %s", sent)
	}
}

func TestIntegrationBackupCollectCreatesMissingDirectory(t *testing.T) {
	origLoad := ftLoadFileTransferSettings
	origNew := ftNewSCPFileDownloader
	ftLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22, Username: "admin", Password: "pass"}, nil
	}
	ftNewSCPFileDownloader = func(s *downloads.FileTransferSettings) fileDownloader {
		return &mockDownloader{}
	}
	defer func() {
		ftLoadFileTransferSettings = origLoad
		ftNewSCPFileDownloader = origNew
	}()

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	now := time.Now().UTC().Format("20060102T150405Z")
	backupFile := fmt.Sprintf("backups/test-%s.backup", now)
	exportFile := fmt.Sprintf("backups/test-%s.rsc", now)
	// Directory check returns empty → cl.Add creates it
	fc := newFakeConn(
		enc("!done"),
		enc("!done"),
		enc("!done"),
		enc("!re", "=name="+backupFile, "=type=backup"),
		enc("!re", "=name="+exportFile, "=type=script"),
		enc("!done"),
	)
	cl.SetConn(fc)

	_, err := backupCollectHandler(cl)(ctx(), testutil.MkReq("system_backup_collect", "name_prefix", "test"))
	if err != nil {
		t.Fatalf("backup collect error: %v", err)
	}
	sent := string(fc.Sent())
	if !strings.Contains(sent, "/file/add") {
		t.Errorf("expected /file/add when backups dir missing, got: %s", sent)
	}
	if !strings.Contains(sent, "=type=directory") {
		t.Errorf("expected directory type in add, got: %s", sent)
	}
}

func TestIntegrationBackupCollectDownloadFailure(t *testing.T) {
	origLoad := ftLoadFileTransferSettings
	origNew := ftNewSCPFileDownloader
	ftLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22, Username: "admin", Password: "pass"}, nil
	}
	ftNewSCPFileDownloader = func(s *downloads.FileTransferSettings) fileDownloader {
		return &mockDownloader{failOnCall: 1}
	}
	defer func() {
		ftLoadFileTransferSettings = origLoad
		ftNewSCPFileDownloader = origNew
	}()

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	now := time.Now().UTC().Format("20060102T150405Z")
	backupFile := fmt.Sprintf("backups/test-%s.backup", now)
	exportFile := fmt.Sprintf("backups/test-%s.rsc", now)
	fc := newFakeConn(
		enc("!re", "=name=backups", "=type=directory"), enc("!done"),
		enc("!done"), enc("!done"),
		enc("!re", "=name="+backupFile, "=type=backup"),
		enc("!re", "=name="+exportFile, "=type=script"),
		enc("!done"),
	)
	cl.SetConn(fc)

	_, err := backupCollectHandler(cl)(ctx(), testutil.MkReq("system_backup_collect", "name_prefix", "test"))
	if err == nil {
		t.Fatal("expected download failure error")
	}
	if !containsAll(err.Error(), "download failed", ".backup") {
		t.Errorf("error should mention download and backup path: %v", err)
	}
}

func TestIntegrationBackupCollectResolvesRelativeLocalDir(t *testing.T) {
	origLoad := ftLoadFileTransferSettings
	origNew := ftNewSCPFileDownloader
	ftLoadFileTransferSettings = func(host string) (*downloads.FileTransferSettings, error) {
		return &downloads.FileTransferSettings{Host: host, Port: 22, Username: "admin", Password: "pass"}, nil
	}
	ftNewSCPFileDownloader = func(s *downloads.FileTransferSettings) fileDownloader {
		return &mockDownloader{}
	}
	defer func() {
		ftLoadFileTransferSettings = origLoad
		ftNewSCPFileDownloader = origNew
	}()

	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	now := time.Now().UTC().Format("20060102T150405Z")
	backupFile := fmt.Sprintf("backups/test-%s.backup", now)
	exportFile := fmt.Sprintf("backups/test-%s.rsc", now)
	fc := newFakeConn(
		enc("!re", "=name=backups", "=type=directory"), enc("!done"),
		enc("!done"), enc("!done"),
		enc("!re", "=name="+backupFile, "=type=backup"),
		enc("!re", "=name="+exportFile, "=type=script"),
		enc("!done"),
	)
	cl.SetConn(fc)

	_, err := backupCollectHandler(cl)(ctx(), testutil.MkReq("system_backup_collect", "name_prefix", "test", "local_dir", "my-backups"))
	if err != nil {
		t.Fatalf("backup collect error with relative local_dir: %v", err)
	}
}

func TestIntegrationBackupCollectExportFailure(t *testing.T) {
	cl := client.NewRouterOSClient("router.test", "admin", "secret")
	fc := newFakeConn(
		enc("!re", "=name=backups", "=type=directory"), enc("!done"),
		enc("!done"), enc("!trap", "=message=disk full"), enc("!done"),
	)
	cl.SetConn(fc)
	_, err := backupCollectHandler(cl)(ctx(),
		testutil.MkReq("system_backup_collect", "name_prefix", "nightly", "include_sensitive", true, "compact", true))
	if err == nil {
		t.Fatal("expected export failure error")
	}
	if !strings.Contains(err.Error(), "export") {
		t.Errorf("error should mention export: %v", err)
	}
}
