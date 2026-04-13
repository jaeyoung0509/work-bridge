package common

import (
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
)

// keyAliases maps common key names to display symbols.
var keyAliases = map[string]string{
	"enter":  "↵",
	"escape": "esc",
	"space":  "␣",
	"ctrl+c": "C-c",
	"tab":    "tab",
	"left":   "←",
	"right":  "→",
}

// RenderHelp renders a help bar from a map of key → description.
func RenderHelp(keys map[string]string) string {
	var parts []string
	order := []string{"enter", "space", "escape", "tab", "ctrl+c", "/", "c", "r", "q"}
	seen := make(map[string]bool)

	for _, k := range order {
		if desc, ok := keys[k]; ok {
			display := k
			if alias, ok := keyAliases[k]; ok {
				display = alias
			}
			parts = append(parts, display+" "+desc)
			seen[k] = true
		}
	}
	for k, desc := range keys {
		if !seen[k] {
			display := k
			if alias, ok := keyAliases[k]; ok {
				display = alias
			}
			parts = append(parts, display+" "+desc)
		}
	}

	text := styles.HelpBar.Render("  " + strings.Join(parts, "  "))
	return text
}
