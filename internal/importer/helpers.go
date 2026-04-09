package importer

import (
	"bufio"
	"bytes"
	"strings"
)

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func splitJSONLLines(data []byte) ([][]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)

	lines := [][]byte{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, []byte(line))
	}

	return lines, scanner.Err()
}

func stringField(raw map[string]any, keys ...string) string {
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

func stripJSONCComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return data
	}
	var b strings.Builder
	inBlock := false
	for _, line := range lines {
		text := line
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
