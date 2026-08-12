package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func WorkspaceRoot() string {
	dir, _ := os.Getwd()
	return dir
}

func BuildEqualityQueries(filters map[string]any) []string {
	var queries []string
	for field, value := range filters {
		if value == nil {
			continue
		}
		queries = append(queries, field+"="+fmt.Sprint(value))
	}
	sort.Strings(queries)
	return queries
}

func RequireExactlyOneLocator(entityName string, locators map[string]string) (string, string, error) {
	var selected []struct{ k, v string }
	for field, value := range locators {
		v := strings.TrimSpace(value)
		if v != "" {
			selected = append(selected, struct{ k, v string }{field, v})
		}
	}
	if len(selected) != 1 {
		opts := make([]string, 0, len(locators))
		for k := range locators {
			opts = append(opts, k)
		}
		return "", "", fmt.Errorf("exactly one %s locator is required: %s", entityName, strings.Join(opts, ", "))
	}
	return selected[0].k, selected[0].v, nil
}

func NormalizeGeneratedName(name, extension, fieldName string) (string, error) {
	value := strings.TrimSpace(name)
	if value == "" {
		return "", fmt.Errorf("%s is required", fieldName)
	}
	if strings.HasSuffix(value, "/") {
		return "", fmt.Errorf("%s must not end with '/'", fieldName)
	}
	if strings.HasSuffix(strings.ToLower(value), strings.ToLower(extension)) {
		value = value[:len(value)-len(extension)]
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", fieldName)
	}
	return value, nil
}

func FileExistsInDirectory(fileName, directory string) bool {
	normDir := strings.Trim(strings.TrimSpace(directory), "/")
	if normDir == "" {
		return true
	}
	normName := strings.Trim(strings.TrimSpace(fileName), "/")
	return normName == normDir || strings.HasPrefix(normName, normDir+"/")
}

func ResolveLocalPath(root, userPath string) (string, error) {
	if strings.TrimSpace(userPath) == "" {
		return filepath.Join(root, "backups"), nil
	}
	if !filepath.IsAbs(userPath) {
		userPath = filepath.Join(root, userPath)
	}
	userPath = filepath.Clean(userPath)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if userPath != rootAbs && !strings.HasPrefix(userPath, rootAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("local path %q is outside the workspace root %q", userPath, rootAbs)
	}
	return userPath, nil
}

func NormalizeRouterFilePath(routerPath string) (string, error) {
	value := strings.Trim(strings.TrimSpace(routerPath), "/")
	if value == "" {
		return "", fmt.Errorf("router_path is required")
	}
	for part := range strings.SplitSeq(value, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("router_path must not contain empty, '.' or '..' path segments")
		}
	}
	return value, nil
}

func RequireAttributes(attrs map[string]any) (map[string]any, error) {
	if len(attrs) == 0 {
		return nil, fmt.Errorf("attributes are required")
	}
	return attrs, nil
}

func NormalizeFirewallTable(table string) (string, error) {
	value := strings.TrimSpace(strings.ToLower(table))
	if value != "filter" && value != "nat" {
		return "", fmt.Errorf("table must be either 'filter' or 'nat'")
	}
	return value, nil
}

func NormalizeMoveDestination(destination string) (string, error) {
	value := strings.TrimSpace(destination)
	if value == "" {
		return "", fmt.Errorf("destination is required")
	}
	return value, nil
}

func NormalizeRequiredString(value, fieldName string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", fmt.Errorf("%s is required", fieldName)
	}
	return normalized, nil
}

func SafeNameComponent(value, defaultVal string) string {
	cleaned := strings.Builder{}
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			cleaned.WriteRune(r)
		} else {
			cleaned.WriteRune('-')
		}
	}
	parts := strings.Split(cleaned.String(), "-")
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	result := strings.Join(nonEmpty, "-")
	if result == "" {
		return defaultVal
	}
	return result
}

func UniqueLocalPath(directory, fileName string) string {
	candidate := filepath.Join(directory, fileName)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(fileName)
	stem := fileName[:len(fileName)-len(ext)]
	counter := 1
	for {
		candidate = filepath.Join(directory, fmt.Sprintf("%s-%d%s", stem, counter, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		counter++
	}
}

func RequireAttributeFields(attrs map[string]any, requiredFields []string) (map[string]any, error) {
	normalized, err := RequireAttributes(attrs)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, field := range requiredFields {
		val, ok := normalized[field]
		if !ok {
			missing = append(missing, field)
			continue
		}
		s := fmt.Sprint(val)
		if strings.TrimSpace(s) == "" {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		if len(missing) == 1 {
			return nil, fmt.Errorf("%s is required", missing[0])
		}
		return nil, fmt.Errorf("required attributes are missing: %s", strings.Join(missing, ", "))
	}
	return normalized, nil
}

func ParseBool(value string, defaultVal bool) bool {
	if value == "" {
		return defaultVal
	}
	v := strings.TrimSpace(strings.ToLower(value))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func ValuesAsAny(m map[string]string) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		result[k] = v
	}
	return result
}

func RecordsAsAny(records []map[string]string) []map[string]any {
	result := make([]map[string]any, 0, len(records))
	for _, r := range records {
		result = append(result, ValuesAsAny(r))
	}
	return result
}

func IntFromEnv(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		return defaultVal
	}
	return n
}

func FloatFromEnv(key string, defaultVal float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err != nil {
		return defaultVal
	}
	return f
}

func JSONCompact(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSpace(buf.String())
}
