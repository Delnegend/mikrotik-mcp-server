package testutil

import "github.com/mark3labs/mcp-go/mcp"

func MkReq(name string, args ...any) mcp.CallToolRequest {
	argMap := make(map[string]any)
	for i := 0; i+1 < len(args); i += 2 {
		if key, ok := args[i].(string); ok {
			argMap[key] = args[i+1]
		}
	}
	return mcp.CallToolRequest{
		Params: struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments,omitempty"`
			Meta      *struct {
				ProgressToken mcp.ProgressToken `json:"progressToken,omitempty"`
			} `json:"_meta,omitempty"`
		}{Name: name, Arguments: argMap},
	}
}
