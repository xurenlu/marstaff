package envvars

import (
	"bufio"
	"strings"
	"unicode"
)

// ParseEnvContent parses .env-style content into key-value map.
// Supports: KEY=value, # comments, empty lines, quoted values.
// Keys not in the result should be treated as deleted when doing bulk update.
func ParseEnvContent(content string) map[string]string {
	out := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		if key == "" {
			continue
		}
		// Key must be valid identifier (letters, digits, underscore)
		if !isValidEnvKey(key) {
			continue
		}
		val := strings.TrimSpace(line[idx+1:])
		val = unquoteEnvVal(val)
		out[key] = val
	}
	return out
}

func isValidEnvKey(s string) bool {
	for i, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
		if i == 0 && !unicode.IsLetter(r) && r != '_' {
			return false
		}
	}
	return len(s) > 0
}

func unquoteEnvVal(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
