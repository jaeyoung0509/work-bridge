package switcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	gotoml "github.com/pelletier/go-toml/v2"
	"golang.org/x/sync/errgroup"

	"github.com/jaeyoung0509/work-bridge/internal/doctor"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/exporter"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
	"github.com/jaeyoung0509/work-bridge/internal/platform/jsonx"
	"github.com/jaeyoung0509/work-bridge/internal/platform/stringx"
)

const (
	managedBlockStart = "<!-- work-bridge:start -->"
	managedBlockEnd   = "<!-- work-bridge:end -->"
)

type projectAdapter struct {
	target    domain.Tool
	fs        fsx.FS
	now       func() time.Time
	homeDir   string
	toolPaths domain.ToolPaths
	lookPath  func(string) (string, error)
	runCmd    commandRunner
}

func (a *projectAdapter) Target() domain.Tool {
	return a.target
}

func (a *projectAdapter) Preview(payload domain.SwitchPayload, projectRoot string, mode domain.SwitchMode, destinationOverride string) (domain.SwitchPlan, error) {
	if mode == domain.SwitchModeNative {
		return a.previewNative(payload, projectRoot, destinationOverride)
	}
	return a.previewProject(payload, projectRoot, destinationOverride)
}

func (a *projectAdapter) previewProject(payload domain.SwitchPayload, projectRoot string, destinationOverride string) (domain.SwitchPlan, error) {
	report, err := doctor.Analyze(doctor.Options{
		Bundle: payload.Bundle,
		Target: a.target,
	})
	if err != nil {
		return domain.SwitchPlan{}, err
	}

	destinationRoot := projectRoot
	if strings.TrimSpace(destinationOverride) != "" {
		destinationRoot = destinationOverride
	}
	managed := managedRoot(destinationRoot, a.target)
	instructionPath := getLocator(a.target).InstructionPath(destinationRoot)
	sessionFiles := append(bundleManagedFiles(payload.Bundle, report, managed), instructionPath)
	projectSkills := projectScopedSkills(payload.Skills)
	skillFiles := a.skillFiles(destinationRoot, projectSkills)
	mcpFiles := a.mcpFiles(destinationRoot, managed, payload.MCP)

	plan := domain.SwitchPlan{
		Mode:            domain.SwitchModeProject,
		TargetTool:      a.target,
		ProjectRoot:     projectRoot,
		DestinationRoot: destinationRoot,
		ManagedRoot:     managed,
		Compatibility:   report,
		Session: domain.SwitchComponentPlan{
			State:   readinessFromCompatibility(report),
			Summary: fmt.Sprintf("%d managed session artifacts", len(sessionFiles)),
			Files:   sessionFiles,
		},
		Skills: domain.SwitchComponentPlan{
			State:   domain.SwitchStateReady,
			Summary: fmt.Sprintf("%d skill bundles", len(projectSkills)),
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
	if len(projectSkills) == 0 {
		plan.Skills.Summary = "No skills selected"
	}
	if len(payload.MCP.Servers) == 0 {
		plan.MCP.Summary = "No MCP servers selected"
	}
	sort.Strings(plan.Session.Files)
	sort.Strings(plan.Skills.Files)
	sort.Strings(plan.MCP.Files)

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
	report.AppliedMode = string(domain.SwitchModeProject)
	return report, nil
}

func (a *projectAdapter) ApplyNativeProject(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	return a.applyNative(payload, plan)
}

func (a *projectAdapter) ExportProject(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeProject)
	return report, nil
}

func (a *projectAdapter) ExportNative(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	return a.exportNative(payload, plan)
}

func (a *projectAdapter) applyPlan(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report := domain.ApplyReport{
		TargetTool:      a.target,
		ProjectRoot:     plan.ProjectRoot,
		DestinationRoot: plan.DestinationRoot,
		ManagedRoot:     plan.ManagedRoot,
		Status:          domain.SwitchStateApplied,
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

	var mu sync.Mutex
	var g errgroup.Group

	g.Go(func() error {
		exportManifest, changed, backups, err := a.writeSessionArtifacts(payload.Bundle, plan)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		report.FilesUpdated = append(report.FilesUpdated, changed...)
		report.BackupsCreated = append(report.BackupsCreated, backups...)
		report.Session.Files = append(report.Session.Files, changed...)
		report.Session.Summary = fmt.Sprintf("%d session files applied", len(exportManifest.Files))
		return nil
	})

	g.Go(func() error {
		skillChanged, skillBackups, skillWarnings, err := a.writeSkills(payload, plan)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		report.FilesUpdated = append(report.FilesUpdated, skillChanged...)
		report.BackupsCreated = append(report.BackupsCreated, skillBackups...)
		report.Skills.Files = append(report.Skills.Files, skillChanged...)
		report.Skills.Summary = fmt.Sprintf("%d skill files applied", len(skillChanged))
		report.Warnings = append(report.Warnings, skillWarnings...)
		if len(payload.Skills) == 0 {
			report.Skills.Summary = "No skills selected"
		}
		return nil
	})

	g.Go(func() error {
		mcpChanged, mcpBackups, mcpWarnings, mcpState, err := a.writeMCP(payload, plan)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		report.FilesUpdated = append(report.FilesUpdated, mcpChanged...)
		report.BackupsCreated = append(report.BackupsCreated, mcpBackups...)
		report.MCP.Files = append(report.MCP.Files, mcpChanged...)
		report.MCP.Summary = fmt.Sprintf("%d MCP files applied", len(mcpChanged))
		report.Warnings = append(report.Warnings, mcpWarnings...)
		report.MCP.State = mcpState
		if len(payload.MCP.Servers) == 0 {
			report.MCP.Summary = "No MCP servers selected"
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return report, err
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

	if patchWarnings := a.applyProjectPatches(payload, plan); len(patchWarnings) > 0 {
		report.Warnings = dedupeStrings(append(report.Warnings, patchWarnings...))
		if report.Status == domain.SwitchStateApplied {
			report.Status = domain.SwitchStatePartial
		}
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
	projectSkills := projectScopedSkills(payload.Skills)
	if len(projectSkills) == 0 {
		return nil, nil, nil, nil
	}
	skillsDir := getLocator(a.target).ProjectSkillRoot(plan.DestinationRoot)
	if skillsDir == "" {
		return nil, nil, []string{fmt.Sprintf("skill bundles are not configured for %s", a.target)}, nil
	}
	updated := []string{}
	backups := []string{}
	used := map[string]int{}
	for _, skill := range projectSkills {
		slug := sanitizeSkillName(skill.Name)
		if slug == "" {
			slug = "skill"
		}
		used[slug]++
		if used[slug] > 1 {
			slug = fmt.Sprintf("%s-%d", slug, used[slug])
		}
		targetDir := filepath.Join(skillsDir, slug)
		changed, backup, err := a.writeSkillBundle(targetDir, skill)
		if err != nil {
			return nil, nil, nil, err
		}
		updated = append(updated, changed...)
		backups = append(backups, backup...)
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

	configPath := getLocator(a.target).ConfigPath(plan.DestinationRoot)
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
	targetPath := getLocator(a.target).InstructionPath(plan.DestinationRoot)
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
		fmt.Sprintf("- Mode: `%s`", plan.Mode),
		fmt.Sprintf("- Source: `%s`", payload.Bundle.SourceTool),
		fmt.Sprintf("- Session: `%s`", payload.Bundle.SourceSessionID),
		fmt.Sprintf("- Destination: `%s`", relativeProjectPath(plan.ProjectRoot, plan.DestinationRoot)),
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

func (a *projectAdapter) renderTargetConfig(path string, payload domain.MCPPayload) (string, string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		return a.renderTargetConfigTOML(path, payload)
	default:
		return a.renderTargetConfigJSON(path, payload)
	}
}

func (a *projectAdapter) renderTargetConfigJSON(path string, payload domain.MCPPayload) (string, string, error) {
	config := map[string]any{}
	if existing, err := a.fs.ReadFile(path); err == nil && len(existing) > 0 {
		if err := jsonx.UnmarshalRelaxed(existing, &config); err != nil {
			return "", fmt.Sprintf("skipped native MCP config patch for %s: %v", filepath.Base(path), err), err
		}
	}
	encoded, warnings := a.marshalTargetMCPServers(payload.Servers)
	config[a.mcpConfigField()] = encoded
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", "", err
	}
	return string(data) + "\n", strings.Join(dedupeStrings(warnings), "; "), nil
}

// renderTargetConfigTOML generates a TOML MCP config for Codex.
// Codex reads [mcp.servers.<name>] tables from the project-local .codex/config.toml.
func (a *projectAdapter) renderTargetConfigTOML(path string, payload domain.MCPPayload) (string, string, error) {
	var config map[string]any
	if existing, err := a.fs.ReadFile(path); err == nil && len(existing) > 0 {
		if err := gotoml.Unmarshal(existing, &config); err != nil {
			return "", fmt.Sprintf("skipped Codex TOML MCP patch for %s: %v", filepath.Base(path), err), err
		}
	}
	if config == nil {
		config = map[string]any{}
	}

	servers := map[string]any{}
	for name, srv := range marshalMCPServers(payload.Servers) {
		servers[name] = srv
	}
	// Codex uses [mcp] top-level key with a servers sub-table.
	mcpSection, _ := config["mcp"].(map[string]any)
	if mcpSection == nil {
		mcpSection = map[string]any{}
	}
	mcpSection["servers"] = servers
	config["mcp"] = mcpSection

	out, err := gotoml.Marshal(config)
	if err != nil {
		return "", "", err
	}
	return string(out), "", nil
}

func (a *projectAdapter) renderMergedTargetConfig(path string, servers map[string]domain.MCPServerConfig) (string, []string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		return a.renderMergedTargetConfigTOML(path, servers)
	default:
		return a.renderMergedTargetConfigJSON(path, servers)
	}
}

func (a *projectAdapter) renderMergedTargetConfigJSON(path string, incoming map[string]domain.MCPServerConfig) (string, []string, error) {
	config := map[string]any{}
	if existing, err := a.fs.ReadFile(path); err == nil && len(existing) > 0 {
		if err := jsonx.UnmarshalRelaxed(existing, &config); err != nil {
			return "", nil, err
		}
	}

	existingServers, parseWarnings := extractMCPServers(config)
	merged, mergeWarnings := mergeMCPServerMaps(sliceToMCPServerMap(existingServers), incoming)
	encoded, encodeWarnings := a.marshalTargetMCPServers(merged)
	config[a.mcpConfigField()] = encoded

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", nil, err
	}

	warnings := append([]string{}, parseWarnings...)
	warnings = append(warnings, mergeWarnings...)
	warnings = append(warnings, encodeWarnings...)
	return string(data) + "\n", dedupeStrings(warnings), nil
}

func (a *projectAdapter) renderMergedTargetConfigTOML(path string, incoming map[string]domain.MCPServerConfig) (string, []string, error) {
	var config map[string]any
	if existing, err := a.fs.ReadFile(path); err == nil && len(existing) > 0 {
		if err := gotoml.Unmarshal(existing, &config); err != nil {
			return "", nil, err
		}
	}
	if config == nil {
		config = map[string]any{}
	}

	existingServers, parseWarnings := extractMCPServers(config)
	merged, mergeWarnings := mergeMCPServerMaps(sliceToMCPServerMap(existingServers), incoming)

	mcpSection, _ := config["mcp"].(map[string]any)
	if mcpSection == nil {
		mcpSection = map[string]any{}
	}
	mcpSection["servers"] = marshalMCPServers(merged)
	config["mcp"] = mcpSection

	out, err := gotoml.Marshal(config)
	if err != nil {
		return "", nil, err
	}

	warnings := append([]string{}, parseWarnings...)
	warnings = append(warnings, mergeWarnings...)
	return string(out), dedupeStrings(warnings), nil
}

func (a *projectAdapter) mcpConfigField() string {
	if a.target == domain.ToolOpenCode {
		return "mcp"
	}
	return "mcpServers"
}

func (a *projectAdapter) marshalTargetMCPServers(servers map[string]domain.MCPServerConfig) (map[string]any, []string) {
	if a.target != domain.ToolOpenCode {
		return marshalMCPServers(servers), nil
	}

	keys := make([]string, 0, len(servers))
	for key := range servers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make(map[string]any, len(keys))
	warnings := []string{}
	for _, key := range keys {
		server := servers[key]
		node := map[string]any{
			"enabled": true,
		}
		switch {
		case server.URL != "":
			node["type"] = "remote"
			node["url"] = server.URL
		default:
			node["type"] = "local"
			command := []string{}
			if server.Command != "" {
				command = append(command, server.Command)
			}
			command = append(command, server.Args...)
			node["command"] = command
		}
		if len(server.Env) > 0 {
			env := map[string]string{}
			for envKey, envVal := range server.Env {
				env[envKey] = envVal
			}
			node["environment"] = env
		}
		if server.Cwd != "" {
			warnings = append(warnings, fmt.Sprintf("OpenCode MCP config does not support cwd for server %q; omitting it", server.Name))
		}
		out[key] = node
	}
	return out, dedupeStrings(warnings)
}

func mergeMCPServerMaps(existing map[string]domain.MCPServerConfig, incoming map[string]domain.MCPServerConfig) (map[string]domain.MCPServerConfig, []string) {
	merged := map[string]domain.MCPServerConfig{}
	for name, server := range existing {
		merged[name] = server
	}

	keys := make([]string, 0, len(incoming))
	for key := range incoming {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	warnings := []string{}
	for _, name := range keys {
		server := incoming[name]
		if current, ok := merged[name]; ok {
			if mcpServerConfigsEqual(current, server) {
				continue
			}
			warnings = append(warnings, fmt.Sprintf("Global MCP server %q already exists in the target config with different settings; keeping the existing target entry", name))
			continue
		}
		merged[name] = server
	}
	return merged, dedupeStrings(warnings)
}

func sliceToMCPServerMap(servers []domain.MCPServerConfig) map[string]domain.MCPServerConfig {
	out := make(map[string]domain.MCPServerConfig, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server.Name) == "" {
			continue
		}
		out[server.Name] = server
	}
	return out
}

func mcpServerConfigsEqual(a domain.MCPServerConfig, b domain.MCPServerConfig) bool {
	if a.Name != b.Name || a.Transport != b.Transport || a.Command != b.Command || a.Cwd != b.Cwd || a.URL != b.URL {
		return false
	}
	if len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if a.Args[i] != b.Args[i] {
			return false
		}
	}
	if len(a.Env) != len(b.Env) {
		return false
	}
	for key, value := range a.Env {
		if b.Env[key] != value {
			return false
		}
	}
	return true
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

func (a *projectAdapter) skillFiles(destinationRoot string, skills []domain.SkillPayload) []string {
	if len(skills) == 0 {
		return nil
	}
	skillsRoot := getLocator(a.target).ProjectSkillRoot(destinationRoot)
	if skillsRoot == "" {
		return nil
	}
	files := []string{}
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
		targetDir := filepath.Join(skillsRoot, slug)
		for _, src := range skillFilesForPayload(skill) {
			rel, err := filepath.Rel(skill.RootPath, src)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				continue
			}
			files = append(files, filepath.Join(targetDir, rel))
		}
	}
	sort.Strings(files)
	return dedupeStrings(files)
}

func (a *projectAdapter) mcpFiles(projectRoot string, managedRoot string, payload domain.MCPPayload) []string {
	files := []string{filepath.Join(managedRoot, "mcp.json")}
	if configPath := getLocator(a.target).ConfigPath(projectRoot); configPath != "" && len(payload.Servers) > 0 {
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

func skillFilesForPayload(skill domain.SkillPayload) []string {
	if len(skill.Files) > 0 {
		files := append([]string{}, skill.Files...)
		sort.Strings(files)
		return files
	}
	if strings.TrimSpace(skill.EntryPath) == "" {
		return nil
	}
	return []string{skill.EntryPath}
}

func projectScopedSkills(skills []domain.SkillPayload) []domain.SkillPayload {
	out := make([]domain.SkillPayload, 0, len(skills))
	for _, skill := range skills {
		if skill.Scope == "project" {
			out = append(out, skill)
		}
	}
	return out
}

func (a *projectAdapter) writeSkillBundle(targetDir string, skill domain.SkillPayload) ([]string, []string, error) {
	if strings.TrimSpace(skill.RootPath) == "" || strings.TrimSpace(skill.EntryPath) == "" {
		return nil, nil, fmt.Errorf("skill bundle %q is missing root_path or entry_path", skill.Name)
	}

	var mu sync.Mutex
	updated := []string{}
	backups := []string{}
	var g errgroup.Group
	g.SetLimit(10) // Limit concurrency for file I/O

	for _, src := range skillFilesForPayload(skill) {
		srcFile := src // capture
		g.Go(func() error {
			rel, err := filepath.Rel(skill.RootPath, srcFile)
			if err != nil {
				return err
			}
			rel = filepath.Clean(rel)
			if rel == "." || strings.HasPrefix(rel, "..") {
				return fmt.Errorf("skill bundle %q contains out-of-root file %s", skill.Name, srcFile)
			}
			data, err := a.fs.ReadFile(srcFile)
			if err != nil {
				return err
			}
			changed, backup, err := a.writeFile(filepath.Join(targetDir, rel), string(data))
			if err != nil {
				return err
			}
			mu.Lock()
			if changed {
				updated = append(updated, filepath.Join(targetDir, rel))
			}
			if backup != "" {
				backups = append(backups, backup)
			}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}
	return dedupeStrings(updated), dedupeStrings(backups), nil
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
	return stringx.SanitizeName(value)
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
