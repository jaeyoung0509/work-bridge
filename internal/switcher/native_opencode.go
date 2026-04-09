package switcher

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

// previewNativeOpenCode provides a plan for OpenCode native mode.
func (a *projectAdapter) previewNativeOpenCode(payload domain.SwitchPayload, projectRoot string, destinationOverride string) (domain.SwitchPlan, error) {
	plan, err := a.previewProject(payload, projectRoot, destinationOverride)
	if err != nil {
		return plan, err
	}
	plan.Mode = domain.SwitchModeNative
	plan.DestinationRoot = a.toolPaths.Dir(domain.ToolOpenCode, a.homeDir)
	if strings.TrimSpace(destinationOverride) != "" {
		plan.DestinationRoot = destinationOverride
	}
	return plan, nil
}

// applyNativeOpenCode writes a staged JSON file and invokes `opencode import <file>`.
func (a *projectAdapter) applyNativeOpenCode(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeNative)

	if _, err := exec.LookPath("opencode"); err != nil {
		return report, fmt.Errorf("opencode CLI is not installed or not in PATH: %w", err)
	}

	tempFile := filepath.Join(plan.ProjectRoot, ".opencode_staged.json")
	data, err := json.Marshal(payload.Bundle)
	if err != nil {
		return report, err
	}

	if err := a.fs.WriteFile(tempFile, data, 0o600); err != nil {
		return report, err
	}
	defer a.fs.Remove(tempFile) // Cleanup

	cmd := exec.Command("opencode", "import", tempFile)
	cmd.Dir = plan.ProjectRoot
	if err := cmd.Run(); err != nil {
		return report, fmt.Errorf("opencode import failed: %w", err)
	}

	report.Warnings = append(report.Warnings, "OpenCode session applied via CLI.")
	return report, nil
}

// exportNativeOpenCode writes the raw format payload natively as a delegate-compatible payload.
func (a *projectAdapter) exportNativeOpenCode(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeNative)

	exportPath := filepath.Join(plan.DestinationRoot, ".opencode_export.json")
	data, err := json.MarshalIndent(payload.Bundle, "", "  ")
	if err != nil {
		return report, err
	}

	if err := a.fs.MkdirAll(plan.DestinationRoot, 0o755); err != nil {
		return report, err
	}
	if err := a.fs.WriteFile(exportPath, data, 0o644); err != nil {
		return report, err
	}

	report.FilesUpdated = append(report.FilesUpdated, exportPath)
	return report, nil
}
