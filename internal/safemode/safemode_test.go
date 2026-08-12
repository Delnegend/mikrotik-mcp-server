package safemode

import (
	"regexp"
	"strings"
	"testing"
)

func TestBuildCLISortedKeys(t *testing.T) {
	got := BuildCLI("/ip/firewall/address-list/add", map[string]any{
		"address": "198.51.100.7",
		"list":    "blocked",
	})
	want := "/ip/firewall/address-list/add address=198.51.100.7 list=blocked"
	if got != want {
		t.Errorf("BuildCLI = %q, want %q", got, want)
	}
}

func TestBuildCLIQuotesValuesWithSpaces(t *testing.T) {
	got := BuildCLI("/interface/set", map[string]any{"comment": "hello world"})
	if got != `/interface/set comment="hello world"` {
		t.Errorf("BuildCLI = %q", got)
	}
}

func TestBuildCLIQuotesEmbeddedDoubleQuote(t *testing.T) {
	got := BuildCLI("/user/set", map[string]any{"comment": `say "hi"`})
	if got != `/user/set comment="say \"hi\""` {
		t.Errorf("BuildCLI = %q", got)
	}
}

func TestBuildCLISkipsNil(t *testing.T) {
	got := BuildCLI("/ip/address/add", map[string]any{"address": "10.0.0.1", "comment": nil})
	if strings.Contains(got, "comment") {
		t.Errorf("nil attribute should be skipped: %q", got)
	}
}

func TestBuildCLIItemID(t *testing.T) {
	got := BuildCLI("/ip/firewall/filter/remove", map[string]any{".id": "*5"})
	if got != "/ip/firewall/filter/remove .id=*5" {
		t.Errorf("BuildCLI = %q", got)
	}
}

func TestBuildCLIEmptyAttrs(t *testing.T) {
	if got := BuildCLI("/system/reboot", nil); got != "/system/reboot" {
		t.Errorf("BuildCLI = %q", got)
	}
}

func TestFindPromptEnd(t *testing.T) {
	cases := []struct {
		name string
		buf  string
		re   *regexp.Regexp
		want int
	}{
		{"safe at end", "[admin@MikroTik] <SAFE> > ", reSafePrompt, 0},
		{"safe with crlf", "x\n[admin@MikroTik] <SAFE> > \r\n", reSafePrompt, 2},
		{"safe mid-output", "foo ] <SAFE> > bar", reSafePrompt, -1},
		{"normal prompt", "output\n[admin@MikroTik] > ", rePrompt, 7},
		{"no prompt", "hello world", rePrompt, -1},
		{"safe marker then normal prompt", "enabled <SAFE> mode\n[admin@MikroTik] > ", rePrompt, 20},
	}
	for _, tc := range cases {
		if got := findPromptEnd([]byte(tc.buf), tc.re); got != tc.want {
			t.Errorf("%s: findPromptEnd(%q) = %d, want %d", tc.name, tc.buf, got, tc.want)
		}
	}
}
