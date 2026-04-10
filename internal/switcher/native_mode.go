package switcher

import (
	"fmt"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

func (a *projectAdapter) previewNative(payload domain.SwitchPayload, projectRoot string, destinationOverride string) (domain.SwitchPlan, error) {
	switch a.target {
	case domain.ToolCodex:
		return a.previewNativeCodex(payload, projectRoot, destinationOverride)
	case domain.ToolGemini:
		return a.previewNativeGemini(payload, projectRoot, destinationOverride)
	case domain.ToolClaude:
		return a.previewNativeClaude(payload, projectRoot, destinationOverride)
	case domain.ToolOpenCode:
		return a.previewNativeOpenCode(payload, projectRoot, destinationOverride)
	default:
		return domain.SwitchPlan{}, fmt.Errorf("unsupported native target %q", a.target)
	}
}

func (a *projectAdapter) applyNative(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	switch a.target {
	case domain.ToolCodex:
		return a.applyNativeCodex(payload, plan)
	case domain.ToolGemini:
		return a.applyNativeGemini(payload, plan)
	case domain.ToolClaude:
		return a.applyNativeClaude(payload, plan)
	case domain.ToolOpenCode:
		return a.applyNativeOpenCode(payload, plan)
	default:
		return domain.ApplyReport{}, fmt.Errorf("unsupported native target %q", a.target)
	}
}

func (a *projectAdapter) exportNative(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	switch a.target {
	case domain.ToolCodex:
		return a.exportNativeCodex(payload, plan)
	case domain.ToolGemini:
		return a.exportNativeGemini(payload, plan)
	case domain.ToolClaude:
		return a.exportNativeClaude(payload, plan)
	case domain.ToolOpenCode:
		return a.exportNativeOpenCode(payload, plan)
	default:
		return domain.ApplyReport{}, fmt.Errorf("unsupported native target %q", a.target)
	}
}

// applyGlobalSkills installs user-scope/global skills to the target tool's skill directory.
func (a *projectAdapter) applyGlobalSkills(payload domain.SwitchPayload, report domain.ApplyReport) (domain.ApplyReport, error) {
	if len(payload.Skills) == 0 {
		return report, nil
	}

	// Filter user-scope/global skills
	globalSkills := make([]domain.SkillPayload, 0)
	for _, skill := range payload.Skills {
		if skill.Scope == "user" || skill.Scope == "global" {
			globalSkills = append(globalSkills, skill)
		}
	}

	if len(globalSkills) == 0 {
		return report, nil
	}

	// Install to target tool's user-scope skill directory
	targetSkillDir := a.globalSkillDir()
	if targetSkillDir == "" {
		report.Warnings = append(report.Warnings, fmt.Sprintf("Global skills not supported for %s", a.target))
		return report, nil
	}

	installed := 0
	for _, skill := range globalSkills {
		slug := sanitizeSkillName(skill.Name)
		if slug == "" {
			slug = "skill"
		}
		targetPath := fmt.Sprintf("%s/%s.md", targetSkillDir, slug)

		// Check if file already exists
		if _, err := a.fs.ReadFile(targetPath); err == nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("Skill %q already exists at %s, skipping", skill.Name, targetPath))
			continue
		}

		// Write skill file
		if err := a.fs.MkdirAll(targetSkillDir, 0o755); err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("Failed to create skill directory: %v", err))
			continue
		}

		if err := a.fs.WriteFile(targetPath, []byte(skill.Content), 0o644); err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("Failed to write skill %s: %v", skill.Name, err))
			continue
		}

		report.FilesUpdated = append(report.FilesUpdated, targetPath)
		installed++
	}

	if installed > 0 {
		report.Warnings = append(report.Warnings, fmt.Sprintf("Installed %d global skill(s) to %s", installed, a.target))
	}

	return report, nil
}

// applyGlobalMCP installs user-scope/global MCP servers to the target tool's config.
// Note: Full global MCP migration requires tool-specific config format handling.
// This is currently limited to warnings; manual migration recommended.
func (a *projectAdapter) applyGlobalMCP(payload domain.SwitchPayload, report domain.ApplyReport) (domain.ApplyReport, error) {
	if len(payload.MCP.Servers) == 0 {
		return report, nil
	}

	// Count user-scope/global MCP sources
	globalCount := 0
	for _, source := range payload.MCP.Sources {
		if source.Scope == "user" || source.Scope == "global" || source.Scope == "legacy" {
			globalCount++
		}
	}

	if globalCount > 0 {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Found %d global MCP source(s). Manual migration recommended for: %s",
				globalCount, a.target))
	}

	return report, nil
}

// globalSkillDir returns the target tool's user-scope skill directory.
// Returns empty string if not supported.
func (a *projectAdapter) globalSkillDir() string {
	switch a.target {
	case domain.ToolCodex:
		return fmt.Sprintf("%s/skills", a.toolPaths.Dir(domain.ToolCodex, a.homeDir))
	case domain.ToolClaude:
		return fmt.Sprintf("%s/skills", a.toolPaths.Dir(domain.ToolClaude, a.homeDir))
	case domain.ToolOpenCode:
		return fmt.Sprintf("%s/.config/opencode/skills", a.homeDir)
	default:
		// Gemini doesn't have a standard user-scope skill directory
		return ""
	}
}
