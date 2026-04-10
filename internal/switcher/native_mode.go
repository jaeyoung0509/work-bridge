package switcher

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
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
		targetPath, candidatePaths := a.globalSkillPaths(targetSkillDir, slug)

		shouldWrite := true
		for _, candidatePath := range candidatePaths {
			existing, err := a.fs.ReadFile(candidatePath)
			if err == nil {
				if normalizeSkillContent(string(existing)) == normalizeSkillContent(skill.Content) {
					shouldWrite = false
					break
				}
				report.Warnings = append(report.Warnings, fmt.Sprintf("Skill %q already exists with different content at %s; leaving the existing file unchanged", skill.Name, candidatePath))
				shouldWrite = false
				break
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return report, fmt.Errorf("stat global skill %s: %w", candidatePath, err)
			}
		}
		if !shouldWrite {
			continue
		}

		changed, backup, err := a.writeFile(targetPath, skill.Content)
		if err != nil {
			return report, fmt.Errorf("write global skill %s: %w", targetPath, err)
		}
		if !changed {
			continue
		}

		report.FilesUpdated = append(report.FilesUpdated, targetPath)
		report.Skills.Files = append(report.Skills.Files, targetPath)
		if backup != "" {
			report.BackupsCreated = append(report.BackupsCreated, backup)
		}
		installed++
	}

	if installed > 0 && report.Skills.Summary == "" {
		report.Skills.Summary = fmt.Sprintf("%d skill files applied", len(report.Skills.Files))
	}
	report.Skills.Files = dedupeStrings(report.Skills.Files)
	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	report.BackupsCreated = dedupeStrings(report.BackupsCreated)
	report.Warnings = dedupeStrings(report.Warnings)
	return report, nil
}

// applyGlobalMCP installs user-scope/global MCP servers to the target tool's global config.
func (a *projectAdapter) applyGlobalMCP(payload domain.SwitchPayload, report domain.ApplyReport) (domain.ApplyReport, error) {
	if len(payload.MCP.Sources) == 0 {
		return report, nil
	}

	servers, warnings := collectGlobalMCPServers(payload.MCP.Sources)
	if len(servers) == 0 {
		report.Warnings = dedupeStrings(append(report.Warnings, warnings...))
		return report, nil
	}

	targetPath := a.globalMCPConfigPath()
	if targetPath == "" {
		report.Warnings = append(report.Warnings, fmt.Sprintf("Global MCP config path is not configured for %s", a.target))
		report.Warnings = dedupeStrings(report.Warnings)
		return report, nil
	}

	content, renderWarnings, err := a.renderMergedTargetConfig(targetPath, servers)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("Skipped global MCP apply for %s config %s: %v", a.target, targetPath, err))
		report.Warnings = dedupeStrings(append(report.Warnings, warnings...))
		return report, nil
	}

	changed, backup, err := a.writeFile(targetPath, content)
	if err != nil {
		return report, fmt.Errorf("write global MCP config %s: %w", targetPath, err)
	}
	if changed {
		report.FilesUpdated = append(report.FilesUpdated, targetPath)
		report.MCP.Files = append(report.MCP.Files, targetPath)
	}
	if backup != "" {
		report.BackupsCreated = append(report.BackupsCreated, backup)
	}

	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	report.BackupsCreated = dedupeStrings(report.BackupsCreated)
	report.MCP.Files = dedupeStrings(report.MCP.Files)
	report.Warnings = dedupeStrings(append(append(report.Warnings, warnings...), renderWarnings...))
	return report, nil
}

func collectGlobalMCPServers(sources []domain.MCPSource) (map[string]domain.MCPServerConfig, []string) {
	type scopedSource struct {
		scopeRank int
		path      string
		servers   []domain.MCPServerConfig
	}

	filtered := make([]scopedSource, 0, len(sources))
	for _, source := range sources {
		if source.Scope != "user" && source.Scope != "global" && source.Scope != "legacy" {
			continue
		}
		if len(source.Servers) == 0 {
			continue
		}
		filtered = append(filtered, scopedSource{
			scopeRank: mcpScopeRank(source.Scope),
			path:      source.Path,
			servers:   source.Servers,
		})
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].scopeRank == filtered[j].scopeRank {
			return filtered[i].path < filtered[j].path
		}
		return filtered[i].scopeRank < filtered[j].scopeRank
	})

	servers := map[string]domain.MCPServerConfig{}
	sourceByServer := map[string]string{}
	warnings := []string{}
	for _, source := range filtered {
		for _, server := range source.servers {
			if strings.TrimSpace(server.Name) == "" {
				continue
			}
			if existing, ok := servers[server.Name]; ok {
				if !mcpServerConfigsEqual(existing, server) {
					warnings = append(warnings, fmt.Sprintf("Global MCP server %q is declared by multiple source configs; keeping the first entry from %s", server.Name, sourceByServer[server.Name]))
				}
				continue
			}
			servers[server.Name] = server
			sourceByServer[server.Name] = source.path
		}
	}
	return servers, dedupeStrings(warnings)
}

func (a *projectAdapter) globalSkillPaths(targetSkillDir string, slug string) (string, []string) {
	switch a.target {
	case domain.ToolOpenCode:
		primary := filepath.Join(targetSkillDir, slug, "SKILL.md")
		return primary, []string{primary, filepath.Join(targetSkillDir, slug+".md")}
	default:
		primary := filepath.Join(targetSkillDir, slug+".md")
		return primary, []string{primary, filepath.Join(targetSkillDir, slug, "SKILL.md")}
	}
}

func (a *projectAdapter) globalMCPConfigPath() string {
	switch a.target {
	case domain.ToolCodex:
		return filepath.Join(a.toolPaths.Dir(domain.ToolCodex, a.homeDir), "config.toml")
	case domain.ToolClaude:
		return filepath.Join(a.toolPaths.Dir(domain.ToolClaude, a.homeDir), "settings.json")
	case domain.ToolGemini:
		return filepath.Join(a.toolPaths.Dir(domain.ToolGemini, a.homeDir), "settings.json")
	case domain.ToolOpenCode:
		candidates := []string{
			filepath.Join(a.homeDir, ".config", "opencode", "opencode.jsonc"),
			filepath.Join(a.homeDir, ".config", "opencode", "opencode.json"),
			filepath.Join(a.toolPaths.Dir(domain.ToolOpenCode, a.homeDir), "opencode.jsonc"),
			filepath.Join(a.toolPaths.Dir(domain.ToolOpenCode, a.homeDir), "opencode.json"),
		}
		for _, candidate := range candidates {
			if _, err := a.fs.Stat(candidate); err == nil {
				return candidate
			}
		}
		return filepath.Join(a.homeDir, ".config", "opencode", "opencode.json")
	default:
		return ""
	}
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
