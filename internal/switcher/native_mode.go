package switcher

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

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

	if a.target == domain.ToolGemini {
		return a.applyGeminiGlobalSkills(globalSkills, report)
	}

	// Install to target tool's user-scope skill directory
	targetSkillDir := a.globalSkillDir()
	if targetSkillDir == "" {
		report.Warnings = append(report.Warnings, fmt.Sprintf("Global skills not supported for %s", a.target))
		return report, nil
	}
	if err := a.fs.MkdirAll(targetSkillDir, 0o755); err != nil {
		return report, fmt.Errorf("create global skill directory %s: %w", targetSkillDir, err)
	}

	installed := 0
	used := map[string]int{}
	for _, skill := range globalSkills {
		slug := sanitizeSkillName(skill.Name)
		if slug == "" {
			slug = "skill"
		}
		used[slug]++
		if used[slug] > 1 {
			slug = fmt.Sprintf("%s-%d", slug, used[slug])
		}
		targetPath := filepath.Join(targetSkillDir, slug+".md")

		existing, err := a.fs.ReadFile(targetPath)
		if err == nil {
			if normalizeSkillContent(string(existing)) == normalizeSkillContent(skill.Content) {
				continue
			}
			report.Warnings = append(report.Warnings, fmt.Sprintf("Skill %q already exists with different content at %s; leaving the existing file unchanged", skill.Name, targetPath))
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return report, fmt.Errorf("stat global skill %s: %w", targetPath, err)
		}

		if err := a.fs.WriteFile(targetPath, []byte(skill.Content), 0o644); err != nil {
			return report, fmt.Errorf("write global skill %s: %w", targetPath, err)
		}

		report.FilesUpdated = append(report.FilesUpdated, targetPath)
		report.Skills.Files = append(report.Skills.Files, targetPath)
		installed++
	}

	if installed > 0 && report.Skills.Summary == "" {
		report.Skills.Summary = fmt.Sprintf("%d skill files applied", len(report.Skills.Files))
	}
	report.Skills.Files = dedupeStrings(report.Skills.Files)
	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	report.Warnings = dedupeStrings(report.Warnings)
	return report, nil
}

// applyGlobalMCP installs user-scope/global MCP servers to the target tool's config.
// Note: Full global MCP migration requires tool-specific config format handling.
// This is currently limited to warnings; manual migration recommended.
func (a *projectAdapter) applyGlobalMCP(payload domain.SwitchPayload, report domain.ApplyReport) (domain.ApplyReport, error) {
	if len(payload.MCP.Sources) == 0 {
		return report, nil
	}

	globalSourceCount := 0
	globalServerCount := 0
	for _, source := range payload.MCP.Sources {
		if source.Scope != "user" && source.Scope != "global" && source.Scope != "legacy" {
			continue
		}
		if len(source.Servers) == 0 {
			continue
		}
		globalSourceCount++
		globalServerCount += len(source.Servers)
	}

	if globalServerCount > 0 {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("Found %d user-scope MCP server(s) across %d source file(s). Native global MCP apply is not implemented for %s yet",
				globalServerCount, globalSourceCount, a.target))
	}

	report.Warnings = dedupeStrings(report.Warnings)
	return report, nil
}

func (a *projectAdapter) applyNativeGlobalArtifacts(payload domain.SwitchPayload, report domain.ApplyReport) (domain.ApplyReport, error) {
	var err error
	report, err = a.applyGlobalSkills(payload, report)
	if err != nil {
		return report, err
	}
	report, err = a.applyGlobalMCP(payload, report)
	if err != nil {
		return report, err
	}
	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	report.Warnings = dedupeStrings(report.Warnings)
	if len(report.Warnings) > 0 && report.Status == domain.SwitchStateApplied {
		report.Status = domain.SwitchStatePartial
	}
	return report, nil
}

// globalSkillDir returns the target tool's user-scope skill directory.
// Returns empty string if not supported.
func (a *projectAdapter) globalSkillDir() string {
	switch a.target {
	case domain.ToolCodex:
		return filepath.Join(a.toolPaths.Dir(domain.ToolCodex, a.homeDir), "skills")
	case domain.ToolClaude:
		return filepath.Join(a.toolPaths.Dir(domain.ToolClaude, a.homeDir), "skills")
	case domain.ToolOpenCode:
		return filepath.Join(a.homeDir, ".config", "opencode", "skills")
	default:
		// Gemini doesn't have a standard user-scope skill directory
		return ""
	}
}

func (a *projectAdapter) applyGeminiGlobalSkills(skills []domain.SkillPayload, report domain.ApplyReport) (domain.ApplyReport, error) {
	targetPath := filepath.Join(a.toolPaths.Dir(domain.ToolGemini, a.homeDir), "GEMINI.md")
	existing, _ := a.fs.ReadFile(targetPath)
	next := upsertManagedBlock(string(existing), renderGeminiGlobalSkillsBlock(skills))

	changed, backup, err := a.writeFile(targetPath, next)
	if err != nil {
		return report, fmt.Errorf("write gemini global skills %s: %w", targetPath, err)
	}
	if changed {
		report.FilesUpdated = append(report.FilesUpdated, targetPath)
		report.Skills.Files = append(report.Skills.Files, targetPath)
	}
	if backup != "" {
		report.BackupsCreated = append(report.BackupsCreated, backup)
	}

	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	report.BackupsCreated = dedupeStrings(report.BackupsCreated)
	report.Skills.Files = dedupeStrings(report.Skills.Files)
	return report, nil
}

func renderGeminiGlobalSkillsBlock(skills []domain.SkillPayload) string {
	lines := []string{
		managedBlockStart,
		"## work-bridge imported global skills",
	}
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			name = "Imported Skill"
		}
		lines = append(lines, "", "### "+name)
		if description := strings.TrimSpace(skill.Description); description != "" {
			lines = append(lines, "", description)
		}
		content := strings.TrimSpace(skill.Content)
		if content != "" {
			lines = append(lines, "", content)
		}
	}
	lines = append(lines, managedBlockEnd, "")
	return strings.Join(lines, "\n")
}

func normalizeSkillContent(content string) string {
	return strings.TrimSpace(content)
}
