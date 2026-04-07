package detect

import (
	"bytes"
	"fmt"
)

func RenderText(report Report) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "Current directory: %s\n", report.CWD)
	fmt.Fprintf(&buf, "Project root: %s\n", report.ProjectRoot)

	for _, tool := range report.Tools {
		foundCount := 0
		missingCount := 0
		for _, artifact := range tool.Artifacts {
			if artifact.Found {
				foundCount++
			} else {
				missingCount++
			}
		}

		state := "not detected"
		if tool.Installed {
			state = "detected"
		}

		fmt.Fprintf(&buf, "\n%s\n", title(tool.Tool))
		fmt.Fprintf(&buf, "  Installed: %s\n", state)
		if tool.Binary.Found {
			fmt.Fprintf(&buf, "  Binary: %s\n", tool.Binary.Path)
		} else {
			fmt.Fprintf(&buf, "  Binary: not found in PATH\n")
		}
		fmt.Fprintf(&buf, "  Artifacts found: %d\n", foundCount)
		fmt.Fprintf(&buf, "  Artifacts missing: %d\n", missingCount)

		if foundCount > 0 {
			fmt.Fprintf(&buf, "  Found:\n")
			for _, artifact := range tool.Artifacts {
				if artifact.Found {
					fmt.Fprintf(&buf, "    - [%s/%s] %s\n", artifact.Scope, artifact.Kind, artifact.Path)
				}
			}
		}

		if len(tool.Notes) > 0 {
			fmt.Fprintf(&buf, "  Notes:\n")
			for _, note := range tool.Notes {
				fmt.Fprintf(&buf, "    - %s\n", note)
			}
		}
	}

	return buf.String()
}

func title(value string) string {
	if value == "" {
		return ""
	}
	if len(value) == 1 {
		return string(value[0] - 32)
	}
	first := value[:1]
	if first >= "a" && first <= "z" {
		first = string(first[0] - 32)
	}
	return first + value[1:]
}
