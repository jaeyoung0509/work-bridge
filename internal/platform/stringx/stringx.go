// Package stringx provides shared string utility functions used across
// multiple packages in work-bridge.
package stringx

import (
	"bufio"
	"bytes"
	"strings"
)

// Dedupe removes duplicates and empty strings from a slice while preserving
// the order of first occurrence. Returns nil for empty or nil input.
func Dedupe(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// FirstNonEmpty returns the first non-blank string from the given values.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// Truncate shortens a string to the given limit, appending "..." if truncated.
func Truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

// StringField extracts the first non-empty string value from a map by trying
// keys in order.
func StringField(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// SanitizeName converts a string into a filesystem-safe slug containing only
// lowercase alphanumeric characters and hyphens.
func SanitizeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// StripJSONCComments removes // and /* */ comments from JSONC data.
func StripJSONCComments(data []byte) []byte {
	lines := splitLines(data)
	if len(lines) == 0 {
		return data
	}
	var b strings.Builder
	inBlock := false
	for _, line := range lines {
		text := string(line)
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		if inBlock {
			if end := strings.Index(text, "*/"); end >= 0 {
				inBlock = false
				text = text[end+2:]
			} else {
				continue
			}
		}
		for {
			start := strings.Index(text, "/*")
			if start < 0 {
				break
			}
			end := strings.Index(text[start+2:], "*/")
			if end < 0 {
				text = text[:start]
				inBlock = true
				break
			}
			text = text[:start] + text[start+2+end+2:]
		}
		if idx := strings.Index(text, "//"); idx >= 0 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		text = strings.TrimRight(text, ",")
		b.WriteString(text)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func splitLines(data []byte) [][]byte {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	var lines [][]byte
	for scanner.Scan() {
		lines = append(lines, scanner.Bytes())
	}
	return lines
}
