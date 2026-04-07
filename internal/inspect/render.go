package inspect

import (
	"bytes"
	"fmt"
)

func RenderText(report Report) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "Tool: %s\n", report.Tool)
	fmt.Fprintf(&buf, "Current directory: %s\n", report.CWD)
	fmt.Fprintf(&buf, "Project root: %s\n", report.ProjectRoot)
	if report.Binary.Found {
		fmt.Fprintf(&buf, "Binary: %s\n", report.Binary.Path)
	} else {
		fmt.Fprintf(&buf, "Binary: not found in PATH\n")
	}

	foundAssets := 0
	for _, asset := range report.Assets {
		if asset.Found {
			foundAssets++
		}
	}
	fmt.Fprintf(&buf, "Assets: %d found / %d tracked\n", foundAssets, len(report.Assets))
	if foundAssets > 0 {
		fmt.Fprintf(&buf, "Found assets:\n")
		for _, asset := range report.Assets {
			if asset.Found {
				fmt.Fprintf(&buf, "  - [%s/%s] %s\n", asset.Scope, asset.Kind, asset.Path)
			}
		}
	}

	fmt.Fprintf(&buf, "Sessions: showing %d of %d\n", len(report.Sessions), report.TotalSessions)
	for _, session := range report.Sessions {
		fmt.Fprintf(&buf, "  - %s", session.ID)
		if session.Title != "" {
			fmt.Fprintf(&buf, " | %s", session.Title)
		}
		if session.UpdatedAt != "" {
			fmt.Fprintf(&buf, " | updated %s", session.UpdatedAt)
		}
		fmt.Fprintln(&buf)
		if session.ProjectRoot != "" {
			fmt.Fprintf(&buf, "    project: %s\n", session.ProjectRoot)
		}
		if session.StoragePath != "" {
			fmt.Fprintf(&buf, "    storage: %s\n", session.StoragePath)
		}
		if session.MessageCount > 0 {
			fmt.Fprintf(&buf, "    messages: %d\n", session.MessageCount)
		}
	}

	if len(report.Notes) > 0 {
		fmt.Fprintf(&buf, "Notes:\n")
		for _, note := range report.Notes {
			fmt.Fprintf(&buf, "  - %s\n", note)
		}
	}

	return buf.String()
}
