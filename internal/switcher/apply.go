package switcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/doctor"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/exporter"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
)

const (
	managedBlockStart = "<!-- work-bridge:start -->"
	managedBlockEnd   = "<!-- work-bridge:end -->"
)

type projectAdapter struct {
	target domain.Tool
	fs     fsx.FS
	now    func() time.Time
}

func (a *projectAdapter) Target() domain.Tool {
	return a.target
}

func (a *projectAdapter) Preview(payload domain.SwitchPayload, projectRoot string) (domain.SwitchPlan, error) {
	report, err := doctor.Analyze(doctor.Options{
		Bundle: payload.Bundle,
		Target: a.target,
	})
	if err != nil {
		return domain.SwitchPlan{}, err
	}

	managed := managedRoot(projectRoot, a.target)
	instructionPath := a.instructionPath(projectRoot)
	sessionFiles := append(bundleManagedFiles(payload.Bundle, report, managed), instructionPath)
	skillFiles := a.skillFiles(managed, payload.Skills)
	mcpFiles := a.mcpFiles(projectRoot, managed, payload.MCP)

	plan := domain.SwitchPlan{
		TargetTool:    a.target,
		ProjectRoot:   projectRoot,
		ManagedRoot:   managed,
		Compatibility: report,
		Session: domain.SwitchComponentPlan{
			State:   readinessFromCompatibility(report),
			Summary: fmt.Sprintf("%d managed session artifacts", len(sessionFiles)),
			Files:   sessionFiles,
		},
		Skills: domain.SwitchComponentPlan{
			State:   domain.SwitchStateReady,
			Summary: fmt.Sprintf("%d managed skills", len(payload.Skills)),
			Files:   skillFiles,
		},
		MCP: domain.SwitchComponentPlan{
			State:   readinessFromWarnings(payload.MCP.Warnings),
			Summary: fmt.Sprintf("%d managed MCP servers", len(payload.MCP.Servers)),
			Files:   mcpFiles,
			Warnings: append([]string{},
				payload.MCP.Warnings...,
			),
		},
		Warnings: dedupeStrings(append(append([]string{}, payload.Warnings...), report.Warnings...)),
	}
	if len(payload.Skills) == 0 {
		plan.Skills.Summary = "No skills selected"
	}
	if len(payload.MCP.Servers) == 0 {
		plan.MCP.Summary = "No MCP servers selected"
	}

	for _, file := range sessionFiles {
		plan.PlannedFiles = append(plan.PlannedFiles, a.planChange(file, "session"))
	}
	for _, file := range skillFiles {
		plan.PlannedFiles = append(plan.PlannedFiles, a.planChange(file, "skills"))
	}
	for _, file := range mcpFiles {
		plan.PlannedFiles = append(plan.PlannedFiles, a.planChange(file, "mcp"))
	}

	plan.Status = aggregateState(plan.Session.State, plan.Skills.State, plan.MCP.State)
	if len(plan.Warnings) > 0 && plan.Status == domain.SwitchStateReady {
		plan.Status = domain.SwitchStatePartial
	}
	return plan, nil
}

func (a *projectAdapter) ApplyProject(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = "project_native"
	return report, nil
}

func (a *projectAdapter) ApplyNativeProject(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	return a.ApplyProject(payload, plan)
}

func (a *projectAdapter) ExportProject(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = "export_only"
	return report, nil
}

func (a *projectAdapter) applyPlan(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report := domain.ApplyReport{
		TargetTool:  a.target,
		ProjectRoot: plan.ProjectRoot,
		ManagedRoot: plan.ManagedRoot,
		Status:      domain.SwitchStateApplied,
		Session: domain.ApplyComponentResult{
			State: domain.SwitchStateApplied,
		},
		Skills: domain.ApplyComponentResult{
			State: domain.SwitchStateApplied,
		},
		MCP: domain.ApplyComponentResult{
			State: domain.SwitchStateApplied,
		},
	}

	exportManifest, changed, backups, err := a.writeSessionArtifacts(payload.Bundle, plan)
	if err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, changed...)
	report.BackupsCreated = append(report.BackupsCreated, backups...)
	report.Session.Files = append(report.Session.Files, changed...)
	report.Session.Summary = fmt.Sprintf("%d session files applied", len(exportManifest.Files))

	skillChanged, skillBackups, skillWarnings, err := a.writeSkills(payload, plan)
	if err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, skillChanged...)
	report.BackupsCreated = append(report.BackupsCreated, skillBackups...)
	report.Skills.Files = append(report.Skills.Files, skillChanged...)
	report.Skills.Summary = fmt.Sprintf("%d skill files applied", len(skillChanged))
	report.Warnings = append(report.Warnings, skillWarnings...)
	if len(payload.Skills) == 0 {
		report.Skills.Summary = "No skills selected"
	}

	mcpChanged, mcpBackups, mcpWarnings, mcpState, err := a.writeMCP(payload, plan)
	if err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, mcpChanged...)
	report.BackupsCreated = append(report.BackupsCreated, mcpBackups...)
	report.MCP.Files = append(report.MCP.Files, mcpChanged...)
	report.MCP.Summary = fmt.Sprintf("%d MCP files applied", len(mcpChanged))
	report.Warnings = append(report.Warnings, mcpWarnings...)
	report.MCP.State = mcpState
	if len(payload.MCP.Servers) == 0 {
		report.MCP.Summary = "No MCP servers selected"
	}

	instructionChanged, instructionBackups, err := a.writeInstructionFile(payload, plan)
	if err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, instructionChanged...)
	report.BackupsCreated = append(report.BackupsCreated, instructionBackups...)
	report.Session.Files = append(report.Session.Files, instructionChanged...)

	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	report.BackupsCreated = dedupeStrings(report.BackupsCreated)
	report.Warnings = dedupeStrings(report.Warnings)
	report.Status = aggregateState(report.Session.State, report.Skills.State, report.MCP.State)
	if len(report.Warnings) > 0 && report.Status == domain.SwitchStateApplied {
		report.Status = domain.SwitchStatePartial
	}
	if report.MCP.State == domain.SwitchStatePartial {
		report.Status = domain.SwitchStatePartial
	}
	return report, nil
}

func (a *projectAdapter) writeSessionArtifacts(bundle domain.SessionBundle, plan domain.SwitchPlan) (domain.ExportManifest, []string, []string, error) {
	report, err := doctor.Analyze(doctor.Options{
		Bundle: bundle,
		Target: a.target,
	})
	if err != nil {
		return domain.ExportManifest{}, nil, nil, err
	}
	tempDir, err := os.MkdirTemp("", "work-bridge-switch-*")
	if err != nil {
		return domain.ExportManifest{}, nil, nil, err
	}
	defer os.RemoveAll(tempDir)

	manifest, err := exporter.Export(exporter.Options{
		FS:     a.fs,
		Bundle: bundle,
		Report: report,
		OutDir: tempDir,
	})
	if err != nil {
		return domain.ExportManifest{}, nil, nil, err
	}

	updated := []string{}
	backups := []string{}
	for _, name := range manifest.Files {
		srcPath := filepath.Join(tempDir, name)
		dstPath := filepath.Join(plan.ManagedRoot, name)
		content := ""
		if name == "manifest.json" {
			manifest.OutputDir = plan.ManagedRoot
			content = marshalJSON(manifest)
		} else {
			data, err := a.fs.ReadFile(srcPath)
			if err != nil {
				return domain.ExportManifest{}, nil, nil, err
			}
			content = string(data)
		}
		changed, backup, err := a.writeFile(dstPath, content)
		if err != nil {
			return domain.ExportManifest{}, nil, nil, err
		}
		if changed {
			updated = append(updated, dstPath)
		}
		if backup != "" {
			backups = append(backups, backup)
		}
	}
	manifest.OutputDir = plan.ManagedRoot
	return manifest, updated, backups, nil
}

func (a *projectAdapter) writeSkills(payload domain.SwitchPayload, plan domain.SwitchPlan) ([]string, []string, []string, error) {
	if len(payload.Skills) == 0 {
		return nil, nil, nil, nil
	}
	skillsDir := filepath.Join(plan.ManagedRoot, "skills")
	updated := []string{}
	backups := []string{}
	index := []map[string]any{}
	used := map[string]int{}
	for _, skill := range payload.Skills {
		slug := sanitizeSkillName(skill.Name)
		if slug == "" {
			slug = "skill"
		}
		used[slug]++
		if used[slug] > 1 {
			slug = fmt.Sprintf("%s-%d", slug, used[slug])
		}
		targetPath := filepath.Join(skillsDir, slug+".md")
		changed, backup, err := a.writeFile(targetPath, skill.Content)
		if err != nil {
			return nil, nil, nil, err
		}
		if changed {
			updated = append(updated, targetPath)
		}
		if backup != "" {
			backups = append(backups, backup)
		}
		index = append(index, map[string]any{
			"name":        skill.Name,
			"description": skill.Description,
			"scope":       skill.Scope,
			"tool":        skill.Tool,
			"source_path": skill.Path,
			"target_path": targetPath,
		})
	}
	indexPath := filepath.Join(skillsDir, "index.json")
	changed, backup, err := a.writeFile(indexPath, marshalJSON(index))
	if err != nil {
		return nil, nil, nil, err
	}
	if changed {
		updated = append(updated, indexPath)
	}
	if backup != "" {
		backups = append(backups, backup)
	}
	return dedupeStrings(updated), dedupeStrings(backups), nil, nil
}

func (a *projectAdapter) writeMCP(payload domain.SwitchPayload, plan domain.SwitchPlan) ([]string, []string, []string, domain.SwitchState, error) {
	if len(payload.MCP.Servers) == 0 && len(payload.MCP.Warnings) == 0 {
		return nil, nil, nil, domain.SwitchStateApplied, nil
	}
	mcpPath := filepath.Join(plan.ManagedRoot, "mcp.json")
	content := marshalJSON(map[string]any{
		"servers":  marshalMCPServers(payload.MCP.Servers),
		"warnings": dedupeStrings(payload.MCP.Warnings),
	})
	updated := []string{}
	backups := []string{}
	warnings := append([]string{}, payload.MCP.Warnings...)
	state := readinessFromWarnings(warnings)

	changed, backup, err := a.writeFile(mcpPath, content)
	if err != nil {
		return nil, nil, nil, domain.SwitchStateError, err
	}
	if changed {
		updated = append(updated, mcpPath)
	}
	if backup != "" {
		backups = append(backups, backup)
	}

	configPath := a.configPath(plan.ProjectRoot)
	if configPath == "" || len(payload.MCP.Servers) == 0 {
		return dedupeStrings(updated), dedupeStrings(backups), dedupeStrings(warnings), state, nil
	}

	configContent, warning, err := a.renderTargetConfig(configPath, payload.MCP)
	if warning != "" {
		warnings = append(warnings, warning)
		state = domain.SwitchStatePartial
	}
	if err != nil {
		return dedupeStrings(updated), dedupeStrings(backups), dedupeStrings(warnings), domain.SwitchStatePartial, nil
	}
	changed, backup, err = a.writeFile(configPath, configContent)
	if err != nil {
		return nil, nil, nil, domain.SwitchStateError, err
	}
	if changed {
		updated = append(updated, configPath)
	}
	if backup != "" {
		backups = append(backups, backup)
	}
	return dedupeStrings(updated), dedupeStrings(backups), dedupeStrings(warnings), state, nil
}

func (a *projectAdapter) writeInstructionFile(payload domain.SwitchPayload, plan domain.SwitchPlan) ([]string, []string, error) {
	targetPath := a.instructionPath(plan.ProjectRoot)
	existing, _ := a.fs.ReadFile(targetPath)
	next := upsertManagedBlock(string(existing), a.renderManagedBlock(payload, plan))
	changed, backup, err := a.writeFile(targetPath, next)
	if err != nil {
		return nil, nil, err
	}
	updated := []string{}
	backups := []string{}
	if changed {
		updated = append(updated, targetPath)
	}
	if backup != "" {
		backups = append(backups, backup)
	}
	return updated, backups, nil
}

func (a *projectAdapter) renderManagedBlock(payload domain.SwitchPayload, plan domain.SwitchPlan) string {
	lines := []string{
		managedBlockStart,
		fmt.Sprintf("## work-bridge %s switch", strings.ToUpper(string(a.target))),
		"",
		fmt.Sprintf("- Source: `%s`", payload.Bundle.SourceTool),
		fmt.Sprintf("- Session: `%s`", payload.Bundle.SourceSessionID),
		fmt.Sprintf("- Managed root: `%s`", relativeProjectPath(plan.ProjectRoot, plan.ManagedRoot)),
	}
	if payload.Bundle.TaskTitle != "" {
		lines = append(lines, fmt.Sprintf("- Task: %s", payload.Bundle.TaskTitle))
	}
	if payload.Bundle.CurrentGoal != "" {
		lines = append(lines, "", "### Current Goal", "", payload.Bundle.CurrentGoal)
	}
	if payload.Bundle.Summary != "" {
		lines = append(lines, "", "### Summary", "", payload.Bundle.Summary)
	}
	lines = append(lines, "", "### Managed Files", "")
	for _, file := range plan.Session.Files {
		lines = append(lines, "- `"+relativeProjectPath(plan.ProjectRoot, file)+"`")
	}
	if len(plan.Skills.Files) > 0 {
		lines = append(lines, "", "### Skills", "")
		for _, file := range plan.Skills.Files {
			lines = append(lines, "- `"+relativeProjectPath(plan.ProjectRoot, file)+"`")
		}
	}
	if len(plan.MCP.Files) > 0 {
		lines = append(lines, "", "### MCP", "")
		for _, file := range plan.MCP.Files {
			lines = append(lines, "- `"+relativeProjectPath(plan.ProjectRoot, file)+"`")
		}
	}
	if len(plan.Warnings) > 0 {
		lines = append(lines, "", "### Notes", "")
		for _, warning := range plan.Warnings {
			lines = append(lines, "- "+warning)
		}
	}
	lines = append(lines, managedBlockEnd, "")
	return strings.Join(lines, "\n")
}

func (a *projectAdapter) instructionPath(projectRoot string) string {
	switch a.target {
	case domain.ToolClaude:
		return filepath.Join(projectRoot, "CLAUDE.md")
	case domain.ToolGemini:
		return filepath.Join(projectRoot, "GEMINI.md")
	case domain.ToolCodex, domain.ToolOpenCode:
		return filepath.Join(projectRoot, "AGENTS.md")
	default:
		return filepath.Join(projectRoot, "AGENTS.md")
	}
}

func (a *projectAdapter) configPath(projectRoot string) string {
	switch a.target {
	case domain.ToolClaude:
		return filepath.Join(projectRoot, ".claude", "settings.local.json")
	case domain.ToolGemini:
		return filepath.Join(projectRoot, ".gemini", "settings.json")
	case domain.ToolOpenCode:
		return filepath.Join(projectRoot, ".opencode", "opencode.jsonc")
	default:
		return ""
	}
}

func (a *projectAdapter) renderTargetConfig(path string, payload domain.MCPPayload) (string, string, error) {
	field := "mcpServers"
	if a.target == domain.ToolOpenCode {
		field = "mcp_servers"
	}
	config := map[string]any{}
	if existing, err := a.fs.ReadFile(path); err == nil && len(existing) > 0 {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".jsonc":
			if err := json.Unmarshal(stripJSONCComments(existing), &config); err != nil {
				return "", fmt.Sprintf("skipped native MCP config patch for %s: %v", filepath.Base(path), err), err
			}
		default:
			if err := json.Unmarshal(existing, &config); err != nil {
				return "", fmt.Sprintf("skipped native MCP config patch for %s: %v", filepath.Base(path), err), err
			}
		}
	}
	config[field] = marshalMCPServers(payload.Servers)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", "", err
	}
	return string(data) + "\n", "", nil
}

func marshalMCPServers(servers map[string]domain.MCPServerConfig) map[string]any {
	keys := make([]string, 0, len(servers))
	for key := range servers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		server := servers[key]
		node := map[string]any{}
		if server.Transport != "" {
			node["transport"] = server.Transport
		}
		if server.Command != "" {
			node["command"] = server.Command
		}
		if len(server.Args) > 0 {
			node["args"] = append([]string{}, server.Args...)
		}
		if len(server.Env) > 0 {
			node["env"] = server.Env
		}
		if server.Cwd != "" {
			node["cwd"] = server.Cwd
		}
		if server.URL != "" {
			node["url"] = server.URL
		}
		out[key] = node
	}
	return out
}

func (a *projectAdapter) skillFiles(managedRoot string, skills []domain.SkillPayload) []string {
	if len(skills) == 0 {
		return nil
	}
	files := make([]string, 0, len(skills)+1)
	used := map[string]int{}
	for _, skill := range skills {
		slug := sanitizeSkillName(skill.Name)
		if slug == "" {
			slug = "skill"
		}
		used[slug]++
		if used[slug] > 1 {
			slug = fmt.Sprintf("%s-%d", slug, used[slug])
		}
		files = append(files, filepath.Join(managedRoot, "skills", slug+".md"))
	}
	files = append(files, filepath.Join(managedRoot, "skills", "index.json"))
	return files
}

func (a *projectAdapter) mcpFiles(projectRoot string, managedRoot string, payload domain.MCPPayload) []string {
	files := []string{filepath.Join(managedRoot, "mcp.json")}
	if configPath := a.configPath(projectRoot); configPath != "" && len(payload.Servers) > 0 {
		files = append(files, configPath)
	}
	return files
}

func (a *projectAdapter) planChange(path string, section string) domain.PlannedFileChange {
	action := "create"
	if _, err := a.fs.Stat(path); err == nil {
		action = "update"
	}
	return domain.PlannedFileChange{
		Path:    path,
		Action:  action,
		Section: section,
	}
}

func (a *projectAdapter) writeFile(path string, content string) (bool, string, error) {
	if err := a.fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, "", err
	}
	current, err := a.fs.ReadFile(path)
	if err == nil && string(current) == content {
		return false, "", nil
	}

	backup := ""
	if err == nil {
		backup = fmt.Sprintf("%s.bak.%s", path, a.now().UTC().Format("20060102T150405"))
		if err := a.fs.WriteFile(backup, current, 0o644); err != nil {
			return false, "", err
		}
	}
	if err := a.fs.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, "", err
	}
	return true, backup, nil
}

func upsertManagedBlock(existing string, block string) string {
	start := strings.Index(existing, managedBlockStart)
	end := strings.Index(existing, managedBlockEnd)
	if start >= 0 && end >= 0 && end >= start {
		end += len(managedBlockEnd)
		prefix := strings.TrimSpace(existing[:start])
		suffix := strings.TrimSpace(existing[end:])
		parts := make([]string, 0, 3)
		if prefix != "" {
			parts = append(parts, prefix)
		}
		parts = append(parts, strings.TrimSpace(block))
		if suffix != "" {
			parts = append(parts, suffix)
		}
		return strings.Join(parts, "\n\n") + "\n"
	}
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return strings.TrimSpace(block) + "\n"
	}
	return existing + "\n\n" + strings.TrimSpace(block) + "\n"
}

func sanitizeSkillName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func readinessFromCompatibility(report domain.CompatibilityReport) domain.SwitchState {
	if len(report.PartialFields) > 0 || len(report.UnsupportedFields) > 0 || len(report.Warnings) > 0 {
		return domain.SwitchStatePartial
	}
	return domain.SwitchStateReady
}

func readinessFromWarnings(warnings []string) domain.SwitchState {
	if len(warnings) > 0 {
		return domain.SwitchStatePartial
	}
	return domain.SwitchStateReady
}

func aggregateState(states ...domain.SwitchState) domain.SwitchState {
	out := domain.SwitchStateApplied
	for _, state := range states {
		switch state {
		case domain.SwitchStateError:
			return domain.SwitchStateError
		case domain.SwitchStatePartial:
			out = domain.SwitchStatePartial
		case domain.SwitchStateReady:
			if out != domain.SwitchStatePartial {
				out = domain.SwitchStateReady
			}
		}
	}
	return out
}
