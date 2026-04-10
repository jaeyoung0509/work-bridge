package importer

import (
	"bufio"
	"bytes"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/platform/stringx"
)

func truncateText(value string, limit int) string {
	return stringx.Truncate(value, limit)
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
	return stringx.StringField(raw, keys...)
}

func stripJSONCComments(data []byte) []byte {
	return stringx.StripJSONCComments(data)
}
