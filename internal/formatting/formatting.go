package formatting

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const EMPTY_DISPLAY = "-"

func CallToolResultFromRecords(title string, items []map[string]any, summaryNoun string, columns [][2]string) (*mcp.CallToolResult, error) {
	count := len(items)
	s := summaryNoun
	if count != 1 {
		s += "s"
	}
	summary := fmt.Sprintf("%s: %d %s", title, count, s)
	lines := []string{summary, ""}
	lines = append(lines, renderTable(items, columns)...)

	text := strings.Join(lines, "\n")
	result := mcp.NewToolResultText(text)
	result.Meta = mcp.NewMetaFromMap(map[string]any{"structuredContent": map[string]any{"result": items}})
	return result, nil
}

func CallToolResultFromRecord(title, summary string, record map[string]any, preferredFields []string) (*mcp.CallToolResult, error) {
	lines := []string{summary, ""}
	lines = append(lines, renderKeyValueTable(record, preferredFields)...)

	text := strings.Join(lines, "\n")
	result := mcp.NewToolResultText(text)
	result.Meta = mcp.NewMetaFromMap(map[string]any{"structuredContent": record})
	return result, nil
}

func renderKeyValueTable(record map[string]any, preferredFields []string) []string {
	ordered := append([]string{}, preferredFields...)
	for k := range record {
		found := slices.Contains(preferredFields, k)
		if !found {
			ordered = append(ordered, k)
		}
	}

	var rows [][2]string
	for _, field := range ordered {
		if v, ok := record[field]; ok {
			rows = append(rows, [2]string{field, displayValue(v)})
		}
	}
	return renderMarkdownTable([2]string{"Field", "Value"}, rows)
}

func renderTable(items []map[string]any, columns [][2]string) []string {
	var headerSlice []string
	for _, c := range columns {
		headerSlice = append(headerSlice, c[1])
	}

	var rows [][]string
	for _, item := range items {
		var row []string
		for _, c := range columns {
			row = append(row, displayValue(item[c[0]]))
		}
		rows = append(rows, row)
	}
	return renderMarkdownTableMulti(headerSlice, rows)
}

func renderMarkdownTable(headers [2]string, rows [][2]string) []string {
	return renderMarkdownTableMulti([]string{headers[0], headers[1]}, rowsToMulti(rows))
}

func renderMarkdownTableMulti(headers []string, rows [][]string) []string {
	var lines []string

	line := "| " + strings.Join(headers, " | ") + " |"
	lines = append(lines, line)

	var sep strings.Builder
	sep.WriteString("|")
	for range headers {
		sep.WriteString(" --- |")
	}
	lines = append(lines, sep.String())

	if len(rows) == 0 {
		var empty strings.Builder
		empty.WriteString("| ")
		for i := range headers {
			if i > 0 {
				empty.WriteString(" | ")
			}
			empty.WriteString(EMPTY_DISPLAY)
		}
		empty.WriteString(" |")
		lines = append(lines, empty.String())
		return lines
	}

	for _, row := range rows {
		escaped := make([]string, len(row))
		for i, v := range row {
			escaped[i] = escapeTableCell(v)
		}
		lines = append(lines, "| "+strings.Join(escaped, " | ")+" |")
	}
	return lines
}

func rowsToMulti(rows [][2]string) [][]string {
	result := make([][]string, len(rows))
	for i, r := range rows {
		result[i] = []string{r[0], r[1]}
	}
	return result
}

func displayValue(value any) string {
	if value == nil {
		return EMPTY_DISPLAY
	}
	switch v := value.(type) {
	case bool:
		if v {
			return "yes"
		}
		return "no"
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return EMPTY_DISPLAY
		}
		lower := strings.ToLower(text)
		if lower == "true" {
			return "yes"
		}
		if lower == "false" {
			return "no"
		}
		return text
	default:
		text := strings.TrimSpace(fmt.Sprint(v))
		if text == "" {
			return EMPTY_DISPLAY
		}
		return text
	}
}

func FormatHealthcheckResult(title string, data map[string]any, preferredFields []string) (*mcp.CallToolResult, error) {
	lines := []string{title, ""}
	lines = append(lines, renderKeyValueTable(data, preferredFields)...)

	if status, ok := data["status"].(string); ok {
		diagnosis := diagnoseHealth(status, data)
		if diagnosis != "" {
			lines = append(lines, "", "**Likely issue:** "+diagnosis)
		}
	}

	text := strings.Join(lines, "\n")
	result := mcp.NewToolResultText(text)
	result.Meta = mcp.NewMetaFromMap(map[string]any{"structuredContent": data})
	return result, nil
}

func diagnoseHealth(status string, data map[string]any) string {
	if status == "healthy" {
		return ""
	}

	api, _ := data["api"].(map[string]any)
	scp, _ := data["scp"].(map[string]any)
	pwd, _ := data["passwordless"].(map[string]any)

	apiCode, _ := api["code"].(string)
	scpCode, _ := scp["code"].(string)
	pwdCode, _ := pwd["code"].(string)

	if apiCode == "api.auth_failed" {
		return "RouterOS API authentication failed — check MIKROTIK_USER and MIKROTIK_PASSWORD"
	}
	if apiCode == "api.connect_failed" {
		return "Cannot connect to RouterOS API — check host and port"
	}
	if scpCode == "scp.config_missing" {
		return "SCP configuration is incomplete — check MIKROTIK_SCP_PRIVATE_KEY or password credentials"
	}
	if scpCode == "scp.auth_failed" {
		return "SCP authentication failed — check SCP username and credentials"
	}
	if scpCode == "scp.connect_failed" {
		return "Cannot connect to SCP server — check MIKROTIK_SCP_HOST and port"
	}
	if pwdCode == "passwordless.fingerprint_missing" {
		return "Passwordless rotation requires MIKROTIK_SCP_HOST_FINGERPRINT_SHA256"
	}
	if pwdCode == "passwordless.key_required" {
		return "Passwordless rotation requires MIKROTIK_SCP_PRIVATE_KEY"
	}
	if pwdCode == "passwordless.ssh_unavailable" {
		return "Passwordless rotation failed because SCP is unavailable"
	}
	if pwdCode == "passwordless.exec_failed" {
		return "Passwordless SSH command execution failed"
	}
	return "Multiple issues detected — review the healthcheck table above"
}

func escapeTableCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
