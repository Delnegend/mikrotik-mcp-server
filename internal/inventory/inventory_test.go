package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const twoDeviceJSON = `[
  {"title":"RouterA","host":"10.0.0.1","password":"pw-a","tags":["lab","eu"],"region":"NL"},
  {"title":"RouterB","host":"10.0.0.2","port":8877,"username":"ops","password":"pw-b","api_ssl":false,"timeout":5}
]`

func TestParseDefaults(t *testing.T) {
	r, err := Parse([]byte(twoDeviceJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Len() != 2 {
		t.Fatalf("Len = %d, want 2", r.Len())
	}

	a, err := r.Get("RouterA")
	if err != nil {
		t.Fatalf("Get RouterA: %v", err)
	}
	if a.Host != "10.0.0.1" || a.Username != "admin" || a.Port != 8728 {
		t.Errorf("defaults not applied: %+v", a)
	}
	if !a.APISSL || !a.TLSVerify || a.Timeout != 10*time.Second || a.SSHPort != 22 || a.SSHUsername != "admin" {
		t.Errorf("boolean/timeout/ssh defaults not applied: %+v", a)
	}
	if len(a.Tags) != 2 || a.Region != "NL" {
		t.Errorf("tags/region = %v/%q", a.Tags, a.Region)
	}

	b, err := r.Get("routerb") // case-insensitive
	if err != nil {
		t.Fatalf("Get routerb: %v", err)
	}
	if b.Port != 8877 || b.Username != "ops" || b.APISSL || b.Timeout != 5*time.Second {
		t.Errorf("explicit fields not honored: %+v", b)
	}
}

func TestGetIsCaseInsensitive(t *testing.T) {
	r, err := Parse([]byte(twoDeviceJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, title := range []string{"routera", "ROUTERA", "RouterB"} {
		if _, err := r.Get(title); err != nil {
			t.Errorf("Get(%q): %v", title, err)
		}
	}
}

func TestGetEmptyTitleSingleDevice(t *testing.T) {
	r := Single(Device{Title: "only", Host: "10.0.0.1"})
	d, err := r.Get("")
	if err != nil {
		t.Fatalf("Get(\"\"): %v", err)
	}
	if d.Host != "10.0.0.1" {
		t.Errorf("host = %q", d.Host)
	}
}

func TestGetEmptyTitleMultiDeviceListsFleet(t *testing.T) {
	r, err := Parse([]byte(twoDeviceJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, err = r.Get("")
	if err == nil {
		t.Fatal("expected error when device omitted for multi-device fleet")
	}
	if !strings.Contains(err.Error(), "RouterA") || !strings.Contains(err.Error(), "RouterB") {
		t.Errorf("error should list the fleet: %v", err)
	}
}

func TestGetUnknownTitleListsFleet(t *testing.T) {
	r, err := Parse([]byte(twoDeviceJSON))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, err = r.Get("Nope")
	if err == nil {
		t.Fatal("expected error for unknown device")
	}
	if !strings.Contains(err.Error(), "available") {
		t.Errorf("error should list available titles: %v", err)
	}
}

func TestParseRejectsEmpty(t *testing.T) {
	if _, err := Parse([]byte("[]")); err == nil {
		t.Fatal("expected error for empty inventory")
	}
}

func TestParseRejectsMissingTitle(t *testing.T) {
	_, err := Parse([]byte(`[{"host":"10.0.0.1"}]`))
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestParseRejectsMissingHost(t *testing.T) {
	_, err := Parse([]byte(`[{"title":"x"}]`))
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestParseRejectsDuplicateTitle(t *testing.T) {
	_, err := Parse([]byte(`[{"title":"a","host":"1.1.1.1"},{"title":"A","host":"2.2.2.2"}]`))
	if err == nil {
		t.Fatal("expected error for duplicate title")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %v", err)
	}
}

func TestParseRejectsMalformedJSON(t *testing.T) {
	_, err := Parse([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseDoesNotEchoCredentials(t *testing.T) {
	_, err := Parse([]byte(`[{"title":"a","host":"10.0.0.1","password":"super-secret"},{"title":"a","host":"2.2.2.2"}]`))
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Errorf("error must not echo credentials: %v", err)
	}
}

func TestFromEnvInline(t *testing.T) {
	t.Setenv("MIKROTIK_INVENTORY", twoDeviceJSON)
	t.Setenv("MIKROTIK_INVENTORY_FILE", "/nonexistent/should-be-ignored")
	if !Configured() {
		t.Fatal("Configured() = false")
	}
	r, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if r.Len() != 2 {
		t.Errorf("Len = %d, want 2", r.Len())
	}
}

func TestFromEnvFile(t *testing.T) {
	t.Setenv("MIKROTIK_INVENTORY", "")
	path := filepath.Join(t.TempDir(), "inventory.json")
	if err := os.WriteFile(path, []byte(twoDeviceJSON), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIKROTIK_INVENTORY_FILE", path)
	r, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if r.Len() != 2 {
		t.Errorf("Len = %d, want 2", r.Len())
	}
}

func TestFromEnvFileMissing(t *testing.T) {
	t.Setenv("MIKROTIK_INVENTORY", "")
	t.Setenv("MIKROTIK_INVENTORY_FILE", "/nonexistent/inventory.json")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error for missing inventory file")
	}
}

func TestDeviceClientBuildsOptions(t *testing.T) {
	d := Device{Host: "10.0.0.1", Port: 8728, Username: "admin", Password: "pw",
		APISSL: false, TLSVerify: false, Timeout: 5 * time.Second}
	cl := d.Client()
	if cl.Host() != "10.0.0.1" || cl.Port() != 8728 || cl.UseSSL() {
		t.Errorf("client options wrong: host=%s port=%d ssl=%v", cl.Host(), cl.Port(), cl.UseSSL())
	}
}
