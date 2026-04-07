package doctor

import (
	"fmt"
	"strings"

	"sessionport/internal/domain"
)

func RenderText(report domain.CompatibilityReport) string {
	var b strings.Builder

	fmt.Fprintln(&b, "Doctor Report")
	fmt.Fprintf(&b, "Source: %s", report.SourceTool)
	if report.SourceSessionID != "" {
		fmt.Fprintf(&b, " session %s", report.SourceSessionID)
	}
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Target: %s\n", report.TargetTool)
	if report.ProjectRoot != "" {
		fmt.Fprintf(&b, "Project root: %s\n", report.ProjectRoot)
	}
	if report.BundleID != "" {
		fmt.Fprintf(&b, "Bundle: %s\n", report.BundleID)
	}

	renderList(&b, "Compatible", report.CompatibleFields)
	renderList(&b, "Partial", report.PartialFields)
	renderList(&b, "Unsupported", report.UnsupportedFields)
	renderList(&b, "Redacted", report.RedactedFields)
	renderList(&b, "Generated artifacts", report.GeneratedArtifacts)
	renderList(&b, "Warnings", report.Warnings)

	return b.String()
}

func renderList(b *strings.Builder, title string, values []string) {
	fmt.Fprintf(b, "%s (%d):\n", title, len(values))
	for _, value := range values {
		fmt.Fprintf(b, "  - %s\n", value)
	}
}
