package server

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestArgString(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         "json:\"name\""
			Arguments map[string]any "json:\"arguments,omitempty\""
			Meta      *struct {
				ProgressToken mcp.ProgressToken "json:\"progressToken,omitempty\""
			} "json:\"_meta,omitempty\""
		}{
			Name: "test",
			Arguments: map[string]any{
				"foo": "bar",
				"num": float64(42),
			},
		},
	}

	if v := argString(req, "foo", "default"); v != "bar" {
		t.Errorf("argString = %q, want bar", v)
	}
	if v := argString(req, "missing", "default"); v != "default" {
		t.Errorf("argString default = %q", v)
	}
	if v := argString(req, "num", ""); v != "42" {
		t.Errorf("argString num = %q", v)
	}
}

func TestArgBool(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         "json:\"name\""
			Arguments map[string]any "json:\"arguments,omitempty\""
			Meta      *struct {
				ProgressToken mcp.ProgressToken "json:\"progressToken,omitempty\""
			} "json:\"_meta,omitempty\""
		}{
			Name: "test",
			Arguments: map[string]any{
				"enabled":  true,
				"disabled": false,
			},
		},
	}

	if v := argBool(req, "enabled", false); !v {
		t.Error("argBool enabled = false, want true")
	}
	if v := argBool(req, "disabled", true); v {
		t.Error("argBool disabled = true, want false")
	}
	if v := argBool(req, "missing", true); !v {
		t.Error("argBool missing default should be true")
	}
}

func TestArgBoolNullable(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         "json:\"name\""
			Arguments map[string]any "json:\"arguments,omitempty\""
			Meta      *struct {
				ProgressToken mcp.ProgressToken "json:\"progressToken,omitempty\""
			} "json:\"_meta,omitempty\""
		}{
			Name: "test",
			Arguments: map[string]any{
				"enabled":  true,
				"disabled": false,
			},
		},
	}

	if v := argBoolNullable(req, "enabled"); v == nil || *v != true {
		t.Error("argBoolNullable enabled should be true")
	}
	if v := argBoolNullable(req, "disabled"); v == nil || *v != false {
		t.Error("argBoolNullable disabled should be false")
	}
	if v := argBoolNullable(req, "missing"); v != nil {
		t.Error("argBoolNullable missing should be nil")
	}
}

func TestArgFloat(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         "json:\"name\""
			Arguments map[string]any "json:\"arguments,omitempty\""
			Meta      *struct {
				ProgressToken mcp.ProgressToken "json:\"progressToken,omitempty\""
			} "json:\"_meta,omitempty\""
		}{
			Name: "test",
			Arguments: map[string]any{
				"count": float64(5),
			},
		},
	}

	if v := argFloat(req, "count", 0); v != 5.0 {
		t.Errorf("argFloat = %f, want 5.0", v)
	}
	if v := argFloat(req, "missing", 10.0); v != 10.0 {
		t.Errorf("argFloat default = %f", v)
	}
}

func TestArgStringSlice(t *testing.T) {
	req := mcp.CallToolRequest{
		Params: struct {
			Name      string         "json:\"name\""
			Arguments map[string]any "json:\"arguments,omitempty\""
			Meta      *struct {
				ProgressToken mcp.ProgressToken "json:\"progressToken,omitempty\""
			} "json:\"_meta,omitempty\""
		}{
			Name: "test",
			Arguments: map[string]any{
				"items": []any{"a", "b", "c"},
			},
		},
	}

	slice := argStringSlice(req, "items")
	if len(slice) != 3 || slice[0] != "a" || slice[2] != "c" {
		t.Errorf("argStringSlice = %v", slice)
	}
	if v := argStringSlice(req, "missing"); v != nil {
		t.Errorf("argStringSlice missing = %v, want nil", v)
	}
}
