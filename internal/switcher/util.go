package switcher

import (
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/platform/stringx"
)

func firstNonEmpty(values ...string) string {
	return stringx.FirstNonEmpty(values...)
}

func dedupeStrings(values []string) []string {
	return stringx.Dedupe(values)
}

func pathWithinRoot(path string, root string) bool {
	path = strings.TrimSpace(path)
	root = strings.TrimSpace(root)
	if path == "" || root == "" {
		return false
	}
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string('/')) || strings.HasPrefix(path, root+string('\\'))
}
