package formatting

import (
	"fmt"
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
	return mcp.NewToolResultText(text), nil
}

func CallToolResultFromRecord(title, summary string, record map[string]any, preferredFields []string) (*mcp.CallToolResult, error) {
	lines := []string{summary, ""}
	lines = append(lines, renderKeyValueTable(record, preferredFields)...)

	text := strings.Join(lines, "\n")
	return mcp.NewToolResultText(text), nil
}

func CallToolResultText(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: text}},
	}
}

func CallToolResultError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: msg}},
	}
}

func renderKeyValueTable(record map[string]any, preferredFields []string) []string {
	ordered := append([]string{}, preferredFields...)
	for k := range record {
		found := false
		for _, pf := range preferredFields {
			if pf == k {
				found = true
				break
			}
		}
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
	headers := [2]string{}
	for i, c := range columns {
		if i == 0 {
			headers[0] = c[1]
		} else {
			headers[1] = c[1]
		}
	}

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

	sep := "|"
	for range headers {
		sep += " --- |"
	}
	lines = append(lines, sep)

	if len(rows) == 0 {
		empty := "| "
		for i := range headers {
			if i > 0 {
				empty += " | "
			}
			empty += EMPTY_DISPLAY
		}
		empty += " |"
		lines = append(lines, empty)
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

func escapeTableCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}
