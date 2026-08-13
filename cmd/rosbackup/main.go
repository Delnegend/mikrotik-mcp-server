// Command rosbackup backs up and restores a RouterOS device configuration
// over the native API plus fingerprint-pinned SFTP. Platform-agnostic: it is a
// single Go binary, cross-compiled for linux/darwin/windows (see the release
// matrix) — no shell/PowerShell twins needed.
//
// Backup captures the full binary config (/system/backup/save, secrets
// included) and optionally a portable text export (/export, secrets hidden).
// Restore loads a .backup binary or imports a .rsc export, always taking a
// timestamped pre-restore backup first (unless -no-preserve).
//
// Usage:
//
//	rosbackup backup  [-host 127.0.0.1] [-api-port 8728] [-ssh-port 2222]
//	                  [-user admin] [-password PW] [-key KEY] [-fingerprint FP]
//	                  [-insecure] [-dir .] [-name NAME] [-export] [-sensitive]
//	                  [-keep-remote]
//	rosbackup restore [-host 127.0.0.1] [-api-port 8728] [-ssh-port 2222]
//	                  [-user admin] [-password PW] [-key KEY] [-fingerprint FP]
//	                  [-insecure] -file FILE [-no-preserve]
//
// Connection settings fall back to the MIKROTIK_* environment variables used
// by the rest of the project (MIKROTIK_PASSWORD, MIKROTIK_SCP_HOST,
// MIKROTIK_SCP_PORT, MIKROTIK_SCP_USER, MIKROTIK_SCP_PRIVATE_KEY,
// MIKROTIK_SCP_HOST_FINGERPRINT_SHA256, MIKROTIK_SCP_INSECURE).
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
	"github.com/Delnegend/mikrotik-mcp/internal/downloads"
	"github.com/Delnegend/mikrotik-mcp/internal/helpers"
)

const defaultSSHPort = 22

// version is stamped by the release build via -ldflags "-X main.version=...".
var version = "dev"

type connFlags struct {
	host        string
	apiPort     int
	apiSSL      bool
	sshPort     int
	user        string
	password    string
	privateKey  string
	fingerprint string
	insecure    bool
	timeout     time.Duration
}

func (f *connFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.host, "host", "127.0.0.1", "RouterOS host")
	fs.IntVar(&f.apiPort, "api-port", 8728, "RouterOS API port")
	fs.BoolVar(&f.apiSSL, "api-ssl", false, "use API-SSL instead of plain API")
	fs.IntVar(&f.sshPort, "ssh-port", defaultSSHPort, "RouterOS SSH port (for file transfer)")
	fs.StringVar(&f.user, "user", "admin", "RouterOS username")
	fs.StringVar(&f.password, "password", "", "RouterOS password (falls back to MIKROTIK_PASSWORD)")
	fs.StringVar(&f.privateKey, "key", "", "SSH private key path (falls back to MIKROTIK_SCP_PRIVATE_KEY)")
	fs.StringVar(&f.fingerprint, "fingerprint", "", "expected SSH host key SHA256 (falls back to MIKROTIK_SCP_HOST_FINGERPRINT_SHA256)")
	fs.BoolVar(&f.insecure, "insecure", false, "skip SSH host key verification (equivalent to MIKROTIK_SCP_INSECURE=1)")
	fs.DurationVar(&f.timeout, "timeout", 30*time.Second, "connection timeout")
}

// settings builds fingerprint-pinned SFTP settings from flags with env
// fallback. -insecure bypasses pinning (loudly).
func (f *connFlags) settings() (*downloads.FileTransferSettings, error) {
	password := f.password
	if password == "" {
		password = os.Getenv("MIKROTIK_PASSWORD")
	}
	key := f.privateKey
	if key == "" {
		key = os.Getenv("MIKROTIK_SCP_PRIVATE_KEY")
	}
	fingerprint := f.fingerprint
	if fingerprint == "" {
		fingerprint = os.Getenv("MIKROTIK_SCP_HOST_FINGERPRINT_SHA256")
	}
	if fingerprint != "" {
		norm, err := downloads.NormalizeSSHFingerprint(fingerprint)
		if err != nil {
			return nil, err
		}
		fingerprint = norm
	}
	if fingerprint == "" && !f.insecure {
		return nil, errors.New("SSH host key fingerprint required: pass -fingerprint (or MIKROTIK_SCP_HOST_FINGERPRINT_SHA256), or -insecure to skip verification")
	}
	if f.insecure && fingerprint == "" {
		fmt.Fprintln(os.Stderr, "warning: SSH host key verification DISABLED (MITM risk) - prefer -fingerprint")
	}
	return &downloads.FileTransferSettings{
		Host:                 f.host,
		Username:             f.user,
		Password:             password,
		PrivateKey:           key,
		SSHFingerprintSHA256: fingerprint,
		Port:                 f.sshPort,
		Timeout:              f.timeout,
		Insecure:             f.insecure,
	}, nil
}

func (f *connFlags) apiClient() (*client.RouterOSClient, error) {
	cl := client.NewRouterOSClient(f.host, f.user, f.password,
		client.WithTLS(f.apiSSL), client.WithPort(f.apiPort), client.WithTimeout(f.timeout))
	if err := cl.Open(); err != nil {
		return nil, err
	}
	return cl, nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "backup":
		err = runBackup(os.Args[2:])
	case "restore":
		err = runRestore(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "rosbackup: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "rosbackup: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`rosbackup - back up and restore a RouterOS configuration.

Usage:
  rosbackup backup  [-host H] [-api-port P] [-api-ssl] [-ssh-port P] [-user U]
                    [-password PW] [-key KEY] [-fingerprint FP] [-insecure]
                    [-dir DIR] [-name NAME] [-export] [-sensitive] [-keep-remote]
  rosbackup restore [-host H] [-api-port P] [-api-ssl] [-ssh-port P] [-user U]
                    [-password PW] [-key KEY] [-fingerprint FP] [-insecure]
                    -file FILE [-no-preserve]

backup saves the full binary config (/system/backup/save) to DIR and
downloads it; -export also fetches a portable text .rsc export.
restore uploads FILE (a .backup or .rsc) and loads it, taking a timestamped
pre-restore backup first unless -no-preserve. The API session drops after a
binary restore - that is expected.

Connection settings fall back to MIKROTIK_PASSWORD, MIKROTIK_SCP_HOST,
MIKROTIK_SCP_PORT, MIKROTIK_SCP_USER, MIKROTIK_SCP_PRIVATE_KEY and
MIKROTIK_SCP_HOST_FINGERPRINT_SHA256.
`)
}

func timestamp() string {
	return time.Now().UTC().Format("20060102T150405Z")
}

// ensureRemoteDir creates a directory on the router if missing.
func ensureRemoteDir(cl *client.RouterOSClient, dir string) error {
	items, err := cl.Print("/file", nil, []string{"name=" + dir}, nil)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		if _, err := cl.Add("/file", map[string]any{"name": dir, "type": "directory"}); err != nil {
			return fmt.Errorf("create directory %q on router: %v", dir, err)
		}
	}
	return nil
}

// waitForFile polls /file until an entry with the given name has size > 0.
func waitForFile(cl *client.RouterOSClient, name string, attempts int) error {
	for i := range attempts {
		items, err := cl.Print("/file", nil, []string{"name=" + name}, nil)
		if err != nil {
			return err
		}
		if len(items) > 0 && items[0]["size"] != "" && items[0]["size"] != "0" {
			return nil
		}
		time.Sleep(2 * time.Second)
		if i == attempts-1 {
			return fmt.Errorf("timed out waiting for router file %q", name)
		}
	}
	return nil
}

// removeRemoteFile deletes a router file, ignoring "not found" errors.
func removeRemoteFile(cl *client.RouterOSClient, name string) {
	items, err := cl.Print("/file", nil, []string{"name=" + name}, nil)
	if err == nil && len(items) > 0 {
		cl.Remove("/file", items[0][".id"])
	}
}

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	conn := &connFlags{}
	conn.register(fs)
	dir := fs.String("dir", ".", "local output directory")
	name := fs.String("name", "", "backup name (default backups/<host>-<timestamp>)")
	export := fs.Bool("export", false, "also download a portable text export (.rsc)")
	sensitive := fs.Bool("sensitive", false, "include sensitive data in the export")
	keepRemote := fs.Bool("keep-remote", false, "keep backup files on the router after download")
	backupPassword := fs.String("backup-password", "", "password for the binary backup (RouterOS 7.17+; use the same on restore)")
	fs.Parse(args)

	settings, err := conn.settings()
	if err != nil {
		return err
	}
	cl, err := conn.apiClient()
	if err != nil {
		return fmt.Errorf("API connect to %s:%d: %v", conn.host, conn.apiPort, err)
	}
	defer cl.Close()

	if err := os.MkdirAll(*dir, 0755); err != nil {
		return fmt.Errorf("create local dir %q: %v", *dir, err)
	}
	if err := ensureRemoteDir(cl, "backups"); err != nil {
		return err
	}

	base := *name
	if base == "" {
		base = fmt.Sprintf("backups/%s-%s", helpers.SafeNameComponent(conn.host, "router"), timestamp())
	}
	backupName, err := helpers.NormalizeGeneratedName(base, ".backup", "name")
	if err != nil {
		return err
	}

	// Binary backup (full config, secrets included).
	saveAttrs := map[string]any{"name": backupName}
	if *backupPassword != "" {
		saveAttrs["password"] = *backupPassword
	}
	if _, err := cl.Run("/system/backup/save", saveAttrs, nil, ""); err != nil {
		return fmt.Errorf("system/backup/save: %v", err)
	}
	routerBackup := backupName + ".backup"
	if err := waitForFile(cl, routerBackup, 10); err != nil {
		return err
	}
	localBackup := filepath.Join(*dir, filepath.Base(routerBackup))
	downloader := downloads.NewSCPFileDownloader(settings)
	if err := downloader.DownloadFile(routerBackup, localBackup); err != nil {
		return fmt.Errorf("download %s: %v", routerBackup, err)
	}
	summary := map[string]any{
		"type":        "binary backup",
		"router":      routerBackup,
		"local":       localBackup,
		"fingerprint": settings.SSHFingerprintSHA256,
	}

	// Optional portable text export (secrets hidden unless -sensitive).
	if *export {
		exportName := backupName
		exportAttrs := map[string]any{"file": exportName}
		if *sensitive {
			exportAttrs["show-sensitive"] = ""
		}
		if _, err := cl.Run("/export", exportAttrs, nil, ""); err != nil {
			return fmt.Errorf("export: %v (binary backup was still saved)", err)
		}
		routerExport := exportName + ".rsc"
		if err := waitForFile(cl, routerExport, 10); err != nil {
			return err
		}
		localExport := filepath.Join(*dir, filepath.Base(routerExport))
		if err := downloader.DownloadFile(routerExport, localExport); err != nil {
			return fmt.Errorf("download %s: %v", routerExport, err)
		}
		summary["export_router"] = routerExport
		summary["export_local"] = localExport
		summary["sensitive"] = *sensitive
	}

	if !*keepRemote {
		removeRemoteFile(cl, routerBackup)
		if *export {
			removeRemoteFile(cl, backupName+".rsc")
		}
	}

	printJSON(summary)
	return nil
}

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	conn := &connFlags{}
	conn.register(fs)
	file := fs.String("file", "", "local .backup or .rsc file to restore")
	noPreserve := fs.Bool("no-preserve", false, "skip the automatic pre-restore backup")
	backupPassword := fs.String("backup-password", "", "password of the binary backup (RouterOS 7.17+; must match the one used at save time)")
	fs.Parse(args)

	if *file == "" {
		return errors.New("-file is required")
	}
	ext := strings.ToLower(filepath.Ext(*file))
	if ext != ".backup" && ext != ".rsc" {
		return fmt.Errorf("unsupported file type %q (use .backup or .rsc)", ext)
	}
	settings, err := conn.settings()
	if err != nil {
		return err
	}
	cl, err := conn.apiClient()
	if err != nil {
		return fmt.Errorf("API connect to %s:%d: %v", conn.host, conn.apiPort, err)
	}
	defer cl.Close()

	if err := ensureRemoteDir(cl, "backups"); err != nil {
		return err
	}

	// Safety net: preserve the current config before replacing it.
	if !*noPreserve {
		preserveName := fmt.Sprintf("backups/pre-restore-%s", timestamp())
		if _, err := cl.Run("/system/backup/save", map[string]any{"name": preserveName}, nil, ""); err != nil {
			return fmt.Errorf("pre-restore backup failed (use -no-preserve to skip): %v", err)
		}
		routerPreserve := preserveName + ".backup"
		if err := waitForFile(cl, routerPreserve, 10); err != nil {
			return err
		}
		localPreserve := filepath.Join(filepath.Dir(*file), "pre-restore-"+filepath.Base(*file))
		downloader := downloads.NewSCPFileDownloader(settings)
		if err := downloader.DownloadFile(routerPreserve, localPreserve); err != nil {
			return fmt.Errorf("pre-restore backup downloaded but: %v", err)
		}
		removeRemoteFile(cl, routerPreserve)
		fmt.Printf("pre-restore backup kept at %s\n", localPreserve)
	}

	// Upload the config file to the router.
	uploadName := filepath.Join("backups", filepath.Base(*file))
	downloader := downloads.NewSCPFileDownloader(settings)
	if err := downloader.UploadFile(*file, uploadName); err != nil {
		return fmt.Errorf("upload %s: %v", *file, err)
	}
	routerName := uploadName // /file entries carry the name incl. subdir
	if err := waitForFile(cl, routerName, 10); err != nil {
		return err
	}

	// Apply: binary load or script import.
	if ext == ".backup" {
		loadAttrs := map[string]any{"name": routerName, "password": *backupPassword}
		if _, err := cl.Run("/system/backup/load", loadAttrs, nil, ""); err != nil {
			if !sessionDropped(cl) {
				return fmt.Errorf("backup/load: %v", err)
			}
			fmt.Printf("backup/load applied (session dropped as expected): %v\n", err)
		}
	} else {
		if _, err := cl.Run("/import", map[string]any{"file-name": routerName}, nil, ""); err != nil {
			return fmt.Errorf("import: %v", err)
		}
		removeRemoteFile(cl, routerName)
	}

	printJSON(map[string]any{
		"restored": *file,
		"note":     "binary restore replaces the whole config; if the session dropped, reconnect and verify",
	})
	return nil
}

// sessionDropped reports whether the API client is dead. After a binary
// restore the session usually dies; a live session means the previous
// command failed for a real reason.
func sessionDropped(cl *client.RouterOSClient) bool {
	_, err := cl.Print("/system/identity", nil, nil, nil)
	return err != nil
}

func printJSON(v map[string]any) {
	fmt.Println(helpers.JSONCompact(v))
}
