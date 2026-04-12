package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/sync/errgroup"

	"github.com/jaeyoung0509/work-bridge/internal/catalog"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/inspect"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
	"github.com/jaeyoung0509/work-bridge/internal/platform/jsonx"
	"github.com/jaeyoung0509/work-bridge/internal/platform/stringx"
	"github.com/jaeyoung0509/work-bridge/internal/tui"
)

var errMCPMethod = fmt.Errorf("mcp method error")

type mcpConfigSummary struct {
	Format      string
	Status      string
	ServerNames []string
	Servers     []tui.MCPServerConfig
	ParseSource string
	Warnings    []string
}

type mcpServerCandidate struct {
	Path    string
	Servers map[string]any
	Score   int
	Source  string
}

type mcpConfigProfile struct {
	Name        string
	Path        string
	Source      string
	Tool        domain.Tool
	RawConfig   string
	BinaryFound bool
	BinaryPath  string
	Summary     mcpConfigSummary
}

type mcpEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpCallError struct {
	Code    int
	Message string
}

func (e *mcpCallError) Error() string {
	return fmt.Sprintf("mcp call failed (%d): %s", e.Code, e.Message)
}

type mcpInitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

func (a *App) loadWorkspaceSnapshot(ctx context.Context) (tui.WorkspaceSnapshot, error) {
	cwd, homeDir, err := a.resolveWorkingDirs()
	if err != nil {
		return tui.WorkspaceSnapshot{}, err
	}

	snapshot := tui.WorkspaceSnapshot{
		HomeDir:       homeDir,
		InspectByTool: map[domain.Tool]inspect.Report{},
		Projects:      []tui.ProjectEntry{},
		Skills:        []tui.SkillEntry{},
		MCPProfiles:   []tui.MCPEntry{},
	}

	var mu sync.Mutex
	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error {
		report, err := a.detectWorkspace(ctx)
		if err != nil {
			return err
		}
		mu.Lock()
		snapshot.Detect = report
		mu.Unlock()
		return nil
	})

	for _, tool := range []domain.Tool{domain.ToolCodex, domain.ToolGemini, domain.ToolClaude, domain.ToolOpenCode} {
		tool := tool
		group.Go(func() error {
			report, err := inspect.Run(inspect.Options{
				FS:        a.fs,
				CWD:       cwd,
				HomeDir:   homeDir,
				ToolPaths: a.config.Paths,
				Tool:      string(tool),
				LookPath:  a.look,
				Limit:     100,
			})
			if err != nil {
				return err
			}
			mu.Lock()
			snapshot.InspectByTool[tool] = report
			mu.Unlock()
			return nil
		})
	}

	group.Go(func() error {
		entries, err := catalog.ScanSkills(a.fs, cwd, homeDir)
		if err != nil {
			return err
		}
		skills := make([]tui.SkillEntry, 0, len(entries))
		for _, entry := range entries {
			skill := tui.SkillEntry{
				Name:        entry.Name,
				Description: entry.Description,
				RootPath:    entry.RootPath,
				EntryPath:   entry.EntryPath,
				Path:        entry.Path,
				Files:       append([]string{}, entry.Files...),
				Source:      entry.Source,
				Scope:       entry.Scope,
				Tool:        domain.Tool(entry.Tool),
			}
			if data, readErr := a.fs.ReadFile(entry.EntryPath); readErr == nil {
				skill.Content = string(data)
			}
			skills = append(skills, skill)
		}
		skills = enrichSkillEntries(skills)
		mu.Lock()
		snapshot.Skills = skills
		mu.Unlock()
		return nil
	})

	group.Go(func() error {
		entries, err := catalog.ScanMCP(a.fs, cwd, homeDir, a.config.Paths)
		if err != nil {
			return err
		}
		profiles := a.loadMCPProfiles(entries)
		enriched := buildLogicalMCPEntries(profiles)
		mu.Lock()
		snapshot.MCPProfiles = enriched
		mu.Unlock()
		return nil
	})

	if err := group.Wait(); err != nil {
		return tui.WorkspaceSnapshot{}, err
	}

	projectRoots := a.resolveWorkspaceRoots(cwd, homeDir, snapshot.Detect.ProjectRoot)
	projectEntries, err := catalog.ScanProjects(a.fs, projectRoots)
	if err != nil {
		return tui.WorkspaceSnapshot{}, err
	}
	snapshot.Projects = enrichProjectEntries(projectEntries, snapshot)
	snapshot.HealthSummary = summarizeWorkspaceHealth(snapshot)
	return snapshot, nil
}

func (a *App) loadMCPProfiles(entries []catalog.MCPEntry) []mcpConfigProfile {
	profiles := make([]mcpConfigProfile, 0, len(entries))
	for _, entry := range entries {
		profile := mcpConfigProfile{
			Name:   entry.Name,
			Path:   entry.Path,
			Source: entry.Source,
			Tool:   inferMCPTool(entry.Name, entry.Path),
			Summary: mcpConfigSummary{
				Status: "configured",
			},
		}
		if data, readErr := a.fs.ReadFile(entry.Path); readErr == nil {
			profile.RawConfig = string(data)
			profile.Summary = summarizeMCPConfig(entry.Path, data)
		} else {
			profile.Summary.Status = "broken"
			profile.Summary.Warnings = []string{fmt.Sprintf("read failed: %v", readErr)}
		}
		if profile.Tool != "" {
			binary := toolBinary(profile.Tool)
			if path, lookErr := a.look(binary); lookErr == nil && path != "" {
				profile.BinaryFound = true
				profile.BinaryPath = path
			}
		}
		profiles = append(profiles, profile)
	}
	return profiles
}

func buildLogicalMCPEntries(profiles []mcpConfigProfile) []tui.MCPEntry {
	grouped := map[string]*tui.MCPEntry{}
	entries := make([]tui.MCPEntry, 0, len(profiles))

	for _, profile := range profiles {
		if len(profile.Summary.Servers) == 0 {
			entry := tui.MCPEntry{
				ID:   mcpConfigEntryID(profile.Tool, profile.Path),
				Kind: "config",
				Name: firstNonEmpty(profile.Name, filepath.Base(profile.Path)),
				Tool: profile.Tool,
				Declarations: []tui.MCPDeclaration{
					mcpDeclarationFromProfile(profile, tui.MCPServerConfig{}),
				},
			}
			materializeLogicalMCPEntry(&entry)
			entries = append(entries, entry)
			continue
		}

		for _, server := range profile.Summary.Servers {
			key := mcpServerEntryID(profile.Tool, server.Name)
			entry, ok := grouped[key]
			if !ok {
				entry = &tui.MCPEntry{
					ID:   key,
					Kind: "server",
					Name: server.Name,
					Tool: profile.Tool,
				}
				grouped[key] = entry
			}
			entry.Declarations = append(entry.Declarations, mcpDeclarationFromProfile(profile, server))
		}
	}

	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry := *grouped[key]
		materializeLogicalMCPEntry(&entry)
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Tool == entries[j].Tool {
			if entries[i].Kind == entries[j].Kind {
				if entries[i].Name == entries[j].Name {
					return entries[i].Path < entries[j].Path
				}
				return entries[i].Name < entries[j].Name
			}
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Tool < entries[j].Tool
	})
	return entries
}

func mcpDeclarationFromProfile(profile mcpConfigProfile, server tui.MCPServerConfig) tui.MCPDeclaration {
	details := "config present"
	if server.Name != "" {
		details = fmt.Sprintf("%s via %s", firstNonEmpty(server.Transport, "stdio"), filepath.Base(profile.Path))
	} else if profile.Summary.Status == "broken" {
		details = "config unreadable"
	}
	return tui.MCPDeclaration{
		Label:         profile.Name,
		Path:          profile.Path,
		Source:        profile.Source,
		Scope:         profile.Source,
		Status:        profile.Summary.Status,
		Details:       details,
		Format:        profile.Summary.Format,
		ParseSource:   profile.Summary.ParseSource,
		RawConfig:     profile.RawConfig,
		ParseWarnings: append([]string{}, profile.Summary.Warnings...),
		BinaryFound:   profile.BinaryFound,
		BinaryPath:    profile.BinaryPath,
		Server:        server,
	}
}

func materializeLogicalMCPEntry(entry *tui.MCPEntry) {
	if entry == nil || len(entry.Declarations) == 0 {
		return
	}
	sort.SliceStable(entry.Declarations, func(i, j int) bool {
		if mcpScopeRank(entry.Declarations[i].Scope) == mcpScopeRank(entry.Declarations[j].Scope) {
			return entry.Declarations[i].Path < entry.Declarations[j].Path
		}
		return mcpScopeRank(entry.Declarations[i].Scope) < mcpScopeRank(entry.Declarations[j].Scope)
	})

	effective := entry.Declarations[0]
	entry.Path = effective.Path
	entry.Source = effective.Source
	entry.Scope = effective.Scope
	entry.Status = firstNonEmpty(effective.Status, entry.Status)
	entry.Details = firstNonEmpty(effective.Details, entry.Details)
	entry.Format = firstNonEmpty(effective.Format, entry.Format)
	entry.ParseSource = firstNonEmpty(effective.ParseSource, entry.ParseSource)
	entry.RawConfig = firstNonEmpty(effective.RawConfig, entry.RawConfig)
	entry.ParseWarnings = dedupeStrings(collectMCPDeclarationWarnings(entry.Declarations))
	entry.BinaryFound = effective.BinaryFound
	entry.BinaryPath = effective.BinaryPath
	entry.Transport = firstNonEmpty(effective.Server.Transport, entry.Transport)
	entry.DeclaredServers = len(entry.Declarations)
	if entry.Kind == "server" && effective.Server.Name != "" {
		entry.Servers = []tui.MCPServerConfig{effective.Server}
		entry.ServerNames = []string{effective.Server.Name}
	} else {
		entry.Servers = nil
		entry.ServerNames = nil
	}
	entry.HiddenScopes = hiddenMCPScopes(entry.Declarations)
}

func hiddenMCPScopes(declarations []tui.MCPDeclaration) []string {
	if len(declarations) <= 1 {
		return nil
	}
	hidden := make([]string, 0, len(declarations)-1)
	for i, decl := range declarations {
		if i == 0 {
			continue
		}
		hidden = append(hidden, fmt.Sprintf("%s %s", firstNonEmpty(decl.Scope, decl.Source), decl.Path))
	}
	return dedupeStrings(hidden)
}

func collectMCPDeclarationWarnings(declarations []tui.MCPDeclaration) []string {
	warnings := []string{}
	for _, decl := range declarations {
		for _, warning := range decl.ParseWarnings {
			warnings = append(warnings, strings.TrimSpace(firstNonEmpty(decl.Scope, decl.Source)+": "+warning))
		}
	}
	return warnings
}

func mcpServerEntryID(tool domain.Tool, name string) string {
	return fmt.Sprintf("%s|server|%s", firstNonEmpty(string(tool), "shared"), strings.TrimSpace(name))
}

func mcpConfigEntryID(tool domain.Tool, path string) string {
	return fmt.Sprintf("%s|config|%s", firstNonEmpty(string(tool), "shared"), filepath.Clean(path))
}

func mcpScopeRank(scope string) int {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "local":
		return 0
	case "project":
		return 1
	case "user":
		return 2
	case "global":
		return 3
	case "legacy":
		return 4
	default:
		return 5
	}
}

func (a *App) resolveWorkspaceRoots(cwd string, homeDir string, projectRoot string) []string {
	roots := append([]string{}, a.config.WorkspaceRoots...)
	if len(roots) == 0 {
		roots = []string{
			projectRoot,
			filepath.Join(homeDir, "Projects"),
			filepath.Join(homeDir, "projects"),
			filepath.Join(homeDir, "work"),
			filepath.Join(homeDir, "workspace"),
			filepath.Join(homeDir, "dev"),
		}
	}

	out := []string{}
	seen := map[string]struct{}{}
	for _, root := range roots {
		resolved := resolveWorkspaceRootPath(cwd, homeDir, root)
		if resolved == "" {
			continue
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		info, err := a.fs.Stat(resolved)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[resolved] = struct{}{}
		out = append(out, resolved)
	}
	sort.Strings(out)
	return out
}

func resolveWorkspaceRootPath(cwd string, homeDir string, root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if root == "~" {
		return filepath.Clean(homeDir)
	}
	if strings.HasPrefix(root, "~/") {
		return filepath.Join(homeDir, strings.TrimPrefix(root, "~/"))
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root)
	}
	return filepath.Clean(filepath.Join(cwd, root))
}

func enrichProjectEntries(entries []catalog.ProjectEntry, snapshot tui.WorkspaceSnapshot) []tui.ProjectEntry {
	projects := make([]tui.ProjectEntry, 0, len(entries))
	for _, entry := range entries {
		projects = append(projects, tui.ProjectEntry{
			Name:          entry.Name,
			Root:          entry.Root,
			WorkspaceRoot: entry.WorkspaceRoot,
			Markers:       append([]string{}, entry.Markers...),
			SessionByTool: map[string]int{},
		})
	}

	for _, report := range snapshot.InspectByTool {
		for _, session := range report.Sessions {
			projectPath := firstNonEmpty(session.ProjectRoot, session.StoragePath)
			if idx := projectIndexForPath(projects, projectPath); idx >= 0 {
				projects[idx].SessionCount++
				projects[idx].SessionByTool[string(report.Tool)]++
			}
		}
	}
	for _, skill := range snapshot.Skills {
		if idx := projectIndexForPath(projects, skill.Path); idx >= 0 {
			projects[idx].SkillCount++
		}
	}
	for i := range projects {
		for _, mcp := range snapshot.MCPProfiles {
			if mcpRelevantToProject(mcp, projects[i].Root) {
				projects[i].MCPCount++
			}
		}
	}

	sort.SliceStable(projects, func(i, j int) bool {
		if projects[i].Name == projects[j].Name {
			return projects[i].Root < projects[j].Root
		}
		return projects[i].Name < projects[j].Name
	})
	return projects
}

func projectIndexForPath(projects []tui.ProjectEntry, path string) int {
	if strings.TrimSpace(path) == "" {
		return -1
	}
	cleaned := filepath.Clean(path)
	bestIdx := -1
	bestLen := 0
	for i, project := range projects {
		root := filepath.Clean(project.Root)
		if cleaned == root || strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
			if len(root) > bestLen {
				bestIdx = i
				bestLen = len(root)
			}
		}
	}
	return bestIdx
}

func mcpRelevantToProject(entry tui.MCPEntry, projectRoot string) bool {
	if strings.TrimSpace(projectRoot) == "" {
		return true
	}
	declarations := entry.Declarations
	if len(declarations) == 0 {
		declarations = []tui.MCPDeclaration{{
			Path:   entry.Path,
			Source: entry.Source,
			Scope:  entry.Scope,
		}}
	}
	for _, decl := range declarations {
		switch decl.Scope {
		case "project", "local":
			if idx := projectIndexForPath([]tui.ProjectEntry{{Root: projectRoot}}, decl.Path); idx >= 0 {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func summarizeWorkspaceHealth(snapshot tui.WorkspaceSnapshot) tui.WorkspaceHealthSummary {
	health := tui.WorkspaceHealthSummary{
		ProjectCount: len(snapshot.Projects),
		SkillCount:   len(snapshot.Skills),
		MCPCount:     len(snapshot.MCPProfiles),
	}
	for _, tool := range snapshot.Detect.Tools {
		if tool.Installed {
			health.InstalledTools++
		}
	}
	for _, report := range snapshot.InspectByTool {
		health.SessionCount += len(report.Sessions)
	}
	for _, entry := range snapshot.MCPProfiles {
		if entry.Status == "degraded" || entry.Status == "broken" {
			health.BrokenMCP++
		}
	}
	return health
}

func enrichSkillEntries(skills []tui.SkillEntry) []tui.SkillEntry {
	grouped := map[string][]int{}
	for i := range skills {
		skills[i].GroupKey = normalizedSkillGroup(skills[i].Name)
		skills[i].ContentHash = hashSkillContent(skills[i].Content)
		grouped[skills[i].GroupKey] = append(grouped[skills[i].GroupKey], i)
	}

	for _, indexes := range grouped {
		variants := make([]tui.SkillVariant, 0, len(indexes))
		hasProject := false
		hasExternal := false
		hashes := map[string]struct{}{}
		for _, idx := range indexes {
			entry := skills[idx]
			if entry.Scope == "project" {
				hasProject = true
			} else {
				hasExternal = true
			}
			if entry.ContentHash != "" {
				hashes[entry.ContentHash] = struct{}{}
			}
			variants = append(variants, tui.SkillVariant{
				Path:   entry.Path,
				Scope:  entry.Scope,
				Tool:   entry.Tool,
				Source: entry.Source,
			})
		}
		sort.SliceStable(variants, func(i, j int) bool {
			if variants[i].Scope == variants[j].Scope {
				if variants[i].Tool == variants[j].Tool {
					return variants[i].Path < variants[j].Path
				}
				return variants[i].Tool < variants[j].Tool
			}
			return skillScopeRank(variants[i].Scope) < skillScopeRank(variants[j].Scope)
		})

		conflict := "only-in-user/global"
		switch {
		case hasProject && !hasExternal:
			conflict = "only-in-project"
		case hasProject && hasExternal && len(hashes) <= 1:
			conflict = "both-present"
		case hasProject && hasExternal:
			conflict = "content-diverged"
		}

		for _, idx := range indexes {
			skills[idx].VariantCount = len(indexes)
			skills[idx].ConflictState = conflict
			skills[idx].Variants = append([]tui.SkillVariant{}, variants...)
		}
	}

	sort.SliceStable(skills, func(i, j int) bool {
		if strings.EqualFold(skills[i].Name, skills[j].Name) {
			if skillScopeRank(skills[i].Scope) == skillScopeRank(skills[j].Scope) {
				if skills[i].Tool == skills[j].Tool {
					return skills[i].Path < skills[j].Path
				}
				return skills[i].Tool < skills[j].Tool
			}
			return skillScopeRank(skills[i].Scope) < skillScopeRank(skills[j].Scope)
		}
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
	return skills
}

func normalizedSkillGroup(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "skill"
	}
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	if len(fields) == 0 {
		return "skill"
	}
	return strings.Join(fields, "-")
}

func hashSkillContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:8])
}

func skillScopeRank(scope string) int {
	switch scope {
	case "project":
		return 0
	case "user":
		return 1
	case "global":
		return 2
	default:
		return 3
	}
}

func summarizeMCPConfig(path string, data []byte) mcpConfigSummary {
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if format == "" {
		format = "text"
	}
	parsed := map[string]any{}

	switch format {
	case "toml":
		if err := toml.Unmarshal(data, &parsed); err != nil {
			return mcpConfigSummary{
				Format:   format,
				Status:   "broken",
				Warnings: []string{fmt.Sprintf("parse failed for %s: %v", path, err)},
			}
		}
	case "json", "jsonc":
		if err := jsonx.UnmarshalRelaxed(data, &parsed); err != nil {
			return mcpConfigSummary{
				Format:   format,
				Status:   "broken",
				Warnings: []string{fmt.Sprintf("parse failed for %s: %v", path, err)},
			}
		}
	default:
		return mcpConfigSummary{
			Format:   format,
			Status:   "configured",
			Warnings: []string{"unsupported config format"},
		}
	}

	servers, parseSource, warnings := extractMCPServers(parsed)
	names := make([]string, 0, len(servers))
	for _, server := range servers {
		names = append(names, server.Name)
	}

	status := "configured"
	switch {
	case len(servers) > 0 && len(warnings) == 0:
		status = "parsed"
	case len(servers) > 0 || len(warnings) > 0:
		status = "degraded"
	}

	return mcpConfigSummary{
		Format:      format,
		Status:      status,
		ServerNames: names,
		Servers:     servers,
		ParseSource: parseSource,
		Warnings:    warnings,
	}
}

func extractMCPServers(value any) ([]tui.MCPServerConfig, string, []string) {
	candidates := []mcpServerCandidate{}
	collectMCPServerCandidates(value, nil, &candidates)
	if len(candidates) == 0 {
		return nil, "", nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Score > candidates[j].Score
	})

	best := candidates[0]
	warnings := []string{}
	for _, candidate := range candidates[1:] {
		if candidate.Path == best.Path {
			continue
		}
		if candidate.Score == best.Score {
			warnings = append(warnings, fmt.Sprintf("multiple MCP server sections found; using %s", best.Path))
			break
		}
	}

	servers, serverWarnings := parseMCPServerConfigs(best.Servers)
	warnings = append(warnings, serverWarnings...)
	if len(servers) == 0 {
		warnings = append(warnings, fmt.Sprintf("MCP section %s is empty", best.Path))
	}
	return servers, best.Source, dedupeStrings(warnings)
}

func collectMCPServerCandidates(value any, path []string, out *[]mcpServerCandidate) {
	node, ok := value.(map[string]any)
	if !ok {
		return
	}
	for key, child := range node {
		nextPath := append(append([]string{}, path...), key)
		if childMap, ok := child.(map[string]any); ok {
			if score, source := scoreMCPServerCandidate(nextPath); score > 0 {
				*out = append(*out, mcpServerCandidate{
					Path:    strings.Join(nextPath, "."),
					Servers: childMap,
					Score:   score,
					Source:  source,
				})
			}
			collectMCPServerCandidates(childMap, nextPath, out)
		}
	}
}

func parseMCPServerConfigs(values map[string]any) ([]tui.MCPServerConfig, []string) {
	servers := make([]tui.MCPServerConfig, 0, len(values))
	warnings := []string{}
	for _, name := range sortedMapKeys(values) {
		raw, ok := values[name]
		if !ok {
			continue
		}
		node, ok := raw.(map[string]any)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("server %s is not an object", name))
			continue
		}

		server := tui.MCPServerConfig{
			Name:      name,
			Transport: firstNonEmpty(stringValue(node["transport"]), stringValue(node["type"])),
			Command:   stringValue(node["command"]),
			Args:      stringSliceValue(node["args"]),
			Env:       stringMapValue(node["env"]),
			Cwd:       stringValue(node["cwd"]),
			URL:       firstNonEmpty(stringValue(node["url"]), stringValue(node["endpoint"])),
		}
		if server.Transport == "" {
			switch {
			case server.URL != "":
				server.Transport = "http"
			case server.Command != "":
				server.Transport = "stdio"
			}
		}
		if server.Command == "" && server.URL == "" {
			warnings = append(warnings, fmt.Sprintf("server %s is missing command or url", name))
		}
		servers = append(servers, server)
	}
	return servers, warnings
}

func scoreMCPServerCandidate(path []string) (int, string) {
	if len(path) == 0 {
		return 0, ""
	}
	normalized := make([]string, len(path))
	for i, segment := range path {
		normalized[i] = normalizeMCPKey(segment)
	}
	last := normalized[len(normalized)-1]
	switch {
	case last == "mcpservers" || last == "mcp_servers":
		return 100 - len(path), strings.Join(path, ".")
	case last == "servers" && sliceContains(normalized[:len(normalized)-1], "mcp"):
		return 90 - len(path), strings.Join(path, ".")
	case strings.HasSuffix(last, "mcp_servers"):
		return 85 - len(path), strings.Join(path, ".")
	default:
		return 0, ""
	}
}

func normalizeMCPKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func sliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func inferMCPTool(name string, path string) domain.Tool {
	value := strings.ToLower(name + " " + path)
	switch {
	case strings.Contains(value, "codex"):
		return domain.ToolCodex
	case strings.Contains(value, "gemini"):
		return domain.ToolGemini
	case strings.Contains(value, "claude"):
		return domain.ToolClaude
	case strings.Contains(value, "opencode"):
		return domain.ToolOpenCode
	default:
		return ""
	}
}

func (a *App) probeMCPFromTUI(ctx context.Context, entry tui.MCPEntry) (tui.MCPProbeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	started := time.Now()
	result := tui.MCPProbeResult{
		ProbedAt: a.clock.Now().Format(time.RFC3339),
	}

	if _, err := a.fs.Stat(entry.Path); err != nil {
		result.Warnings = []string{"config file no longer exists"}
		result.Mode = "config-only"
		result.Latency = time.Since(started).Round(time.Millisecond).String()
		return result, nil
	}

	if len(entry.ParseWarnings) > 0 {
		result.Warnings = append(result.Warnings, entry.ParseWarnings...)
	}
	if !entry.BinaryFound && entry.Tool != "" {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s binary not found on PATH", entry.Tool))
	}
	if len(entry.Servers) == 0 {
		result.Warnings = append(result.Warnings, "no MCP servers declared in config")
		result.Mode = "config-only"
		result.Latency = time.Since(started).Round(time.Millisecond).String()
		return result, nil
	}

	runtimeAttempted := 0
	runtimeModes := map[string]struct{}{}
	connected := 0
	aggregateWarnings := append([]string{}, result.Warnings...)
	serverResults := make([]tui.MCPServerProbeResult, 0, len(entry.Servers))

	for _, server := range entry.Servers {
		serverCtx := ctx
		cancel := func() {}
		if _, ok := ctx.Deadline(); !ok {
			serverCtx, cancel = context.WithTimeout(ctx, 4*time.Second)
		}
		serverResult := a.probeMCPServer(serverCtx, entry.Path, server)
		cancel()
		serverResults = append(serverResults, serverResult)
		runtimeAttempted++
		runtimeModes[firstNonEmpty(serverResult.Transport, "stdio")] = struct{}{}
		if serverResult.Reachable {
			connected++
			result.ResourceCount += serverResult.ResourceCount
			result.TemplateCount += serverResult.TemplateCount
			result.ToolCount += serverResult.ToolCount
			result.PromptCount += serverResult.PromptCount
		}
		if serverResult.Error != "" {
			aggregateWarnings = append(aggregateWarnings, fmt.Sprintf("%s: %s", serverResult.Name, serverResult.Error))
		}
		for _, warning := range serverResult.Warnings {
			aggregateWarnings = append(aggregateWarnings, fmt.Sprintf("%s: %s", serverResult.Name, warning))
		}
	}

	result.ServerResults = serverResults
	result.ConnectedServers = connected
	result.Warnings = dedupeStrings(aggregateWarnings)
	switch {
	case runtimeAttempted == 0:
		result.Mode = "config-only"
	case len(runtimeModes) == 1:
		for mode := range runtimeModes {
			result.Mode = "runtime-" + mode
		}
	default:
		result.Mode = "runtime-mixed"
	}
	result.Reachable = connected == len(entry.Servers) && len(entry.ParseWarnings) == 0
	result.Latency = time.Since(started).Round(time.Millisecond).String()
	return result, nil
}

func (a *App) probeMCPServer(ctx context.Context, configPath string, server tui.MCPServerConfig) tui.MCPServerProbeResult {
	started := time.Now()
	result := tui.MCPServerProbeResult{
		Name:      server.Name,
		Transport: firstNonEmpty(server.Transport, "stdio"),
		Command:   firstNonEmpty(server.Command, server.URL),
	}

	switch firstNonEmpty(server.Transport, "stdio") {
	case "stdio":
		probed, err := a.probeMCPStdioServer(ctx, configPath, server)
		if err != nil {
			result.Error = err.Error()
		} else {
			result = probed
		}
	case "http", "streamable-http":
		probed, err := a.probeMCPHTTPServer(ctx, server)
		if err != nil {
			result.Error = err.Error()
		} else {
			result = probed
		}
	case "sse":
		probed, err := a.probeMCPSSEServer(ctx, server)
		if err != nil {
			result.Error = err.Error()
		} else {
			result = probed
		}
	default:
		result.Error = fmt.Sprintf("unsupported %s transport", firstNonEmpty(server.Transport, "unknown"))
	}

	if result.Latency == "" {
		result.Latency = time.Since(started).Round(time.Millisecond).String()
	}
	return result
}

func (a *App) probeMCPStdioServer(ctx context.Context, configPath string, server tui.MCPServerConfig) (tui.MCPServerProbeResult, error) {
	result := tui.MCPServerProbeResult{
		Name:      server.Name,
		Transport: "stdio",
		Command:   server.Command,
	}
	if strings.TrimSpace(server.Command) == "" {
		return result, fmt.Errorf("missing stdio command")
	}

	workDir := resolveMCPServerWorkDir(configPath, server.Cwd)
	command := resolveMCPServerCommand(server.Command, workDir)
	cmd := exec.CommandContext(ctx, command, server.Args...)
	cmd.Dir = workDir
	cmd.Env = mergeServerEnv(os.Environ(), server.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return result, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return result, err
	}
	if err := cmd.Start(); err != nil {
		return result, err
	}

	var stderrBuf bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stderrBuf, stderr)
		close(stderrDone)
	}()

	defer func() {
		_ = stdin.Close()
		waitDone := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-time.After(300 * time.Millisecond):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-waitDone
		}
		<-stderrDone
	}()

	session := mcpClientSession{
		reader: bufio.NewReader(stdout),
		writer: stdin,
	}
	probed, err := probeMCPRuntimeSession(ctx, session, result)
	if err != nil {
		if stderrBuf.Len() > 0 {
			return result, fmt.Errorf("%w; stderr: %s", err, strings.TrimSpace(stderrBuf.String()))
		}
		return result, err
	}
	return probed, nil
}

func (a *App) probeMCPHTTPServer(ctx context.Context, server tui.MCPServerConfig) (tui.MCPServerProbeResult, error) {
	result := tui.MCPServerProbeResult{
		Name:      server.Name,
		Transport: firstNonEmpty(server.Transport, "http"),
		Command:   server.URL,
	}
	if strings.TrimSpace(server.URL) == "" {
		return result, fmt.Errorf("missing %s url", result.Transport)
	}
	session := &mcpHTTPClientSession{
		client: newMCPHTTPClient(),
		url:    server.URL,
	}
	return probeMCPRuntimeSession(ctx, session, result)
}

func (a *App) probeMCPSSEServer(ctx context.Context, server tui.MCPServerConfig) (tui.MCPServerProbeResult, error) {
	result := tui.MCPServerProbeResult{
		Name:      server.Name,
		Transport: "sse",
		Command:   server.URL,
	}
	if strings.TrimSpace(server.URL) == "" {
		return result, fmt.Errorf("missing sse url")
	}
	session, err := newMCPSSEClientSession(ctx, newMCPHTTPClient(), newMCPHTTPClient(), server.URL)
	if err != nil {
		return result, err
	}
	return probeMCPRuntimeSession(ctx, session, result)
}

func hasCapability(capabilities map[string]any, key string) bool {
	if len(capabilities) == 0 {
		return false
	}
	value, ok := capabilities[key]
	if !ok {
		return false
	}
	if boolean, ok := value.(bool); ok {
		return boolean
	}
	return value != nil
}

type mcpRuntimeSession interface {
	initialize(context.Context, int) (mcpInitializeResult, error)
	notify(string, any) error
	listCount(context.Context, *int, string, string) (int, error)
	close() error
}

func probeMCPRuntimeSession(ctx context.Context, session mcpRuntimeSession, base tui.MCPServerProbeResult) (tui.MCPServerProbeResult, error) {
	started := time.Now()
	result := base
	defer func() {
		_ = session.close()
		if result.Latency == "" {
			result.Latency = time.Since(started).Round(time.Millisecond).String()
		}
	}()

	requestID := 1
	initializeResult, err := session.initialize(ctx, requestID)
	if err != nil {
		return result, err
	}
	requestID++

	result.ProtocolVersion = initializeResult.ProtocolVersion
	result.ServerName = initializeResult.ServerInfo.Name
	result.ServerVersion = initializeResult.ServerInfo.Version
	result.Reachable = true

	if err := session.notify("notifications/initialized", map[string]any{}); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("initialized notification failed: %v", err))
	}

	if hasCapability(initializeResult.Capabilities, "resources") {
		count, err := session.listCount(ctx, &requestID, "resources/list", "resources")
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("resources/list failed: %v", err))
		} else {
			result.ResourceCount = count
		}

		count, err = session.listCount(ctx, &requestID, "resources/templates/list", "resourceTemplates")
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("resources/templates/list failed: %v", err))
		} else {
			result.TemplateCount = count
		}
	}

	if hasCapability(initializeResult.Capabilities, "tools") {
		count, err := session.listCount(ctx, &requestID, "tools/list", "tools")
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("tools/list failed: %v", err))
		} else {
			result.ToolCount = count
		}
	}

	if hasCapability(initializeResult.Capabilities, "prompts") {
		count, err := session.listCount(ctx, &requestID, "prompts/list", "prompts")
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("prompts/list failed: %v", err))
		} else {
			result.PromptCount = count
		}
	}

	result.Latency = time.Since(started).Round(time.Millisecond).String()
	return result, nil
}

type mcpClientSession struct {
	reader *bufio.Reader
	writer io.Writer
}

func (s mcpClientSession) initialize(ctx context.Context, id int) (mcpInitializeResult, error) {
	response, err := s.call(ctx, id, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"resources": map[string]any{},
			"tools":     map[string]any{},
			"prompts":   map[string]any{},
		},
		"clientInfo": map[string]any{
			"name":    "work-bridge",
			"version": Version,
		},
	})
	if err != nil {
		return mcpInitializeResult{}, err
	}

	var result mcpInitializeResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return mcpInitializeResult{}, fmt.Errorf("decode initialize result: %w", err)
	}
	return result, nil
}

func (s mcpClientSession) notify(method string, params any) error {
	return writeMCPMessage(s.writer, map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

func (s mcpClientSession) close() error {
	return nil
}

func (s mcpClientSession) listCount(ctx context.Context, requestID *int, method string, field string) (int, error) {
	total := 0
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		response, err := s.call(ctx, *requestID, method, params)
		*requestID += 1
		if err != nil {
			if callErr, ok := err.(*mcpCallError); ok && callErr.Code == -32601 {
				return 0, nil
			}
			return total, err
		}

		var payload map[string]json.RawMessage
		if err := json.Unmarshal(response.Result, &payload); err != nil {
			return total, fmt.Errorf("decode %s result: %w", method, err)
		}

		var items []json.RawMessage
		if raw, ok := payload[field]; ok && len(raw) > 0 {
			if err := json.Unmarshal(raw, &items); err != nil {
				return total, fmt.Errorf("decode %s items: %w", method, err)
			}
			total += len(items)
		}

		var next string
		if raw, ok := payload["nextCursor"]; ok && len(raw) > 0 {
			if err := json.Unmarshal(raw, &next); err != nil {
				return total, fmt.Errorf("decode %s nextCursor: %w", method, err)
			}
		}
		if next == "" {
			return total, nil
		}
		cursor = next
	}
}

func (s mcpClientSession) call(ctx context.Context, id int, method string, params any) (mcpEnvelope, error) {
	if err := writeMCPMessage(s.writer, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}); err != nil {
		return mcpEnvelope{}, err
	}

	for {
		select {
		case <-ctx.Done():
			return mcpEnvelope{}, ctx.Err()
		default:
		}

		message, err := readMCPMessage(s.reader)
		if err != nil {
			return mcpEnvelope{}, err
		}

		if len(message.Method) > 0 {
			if len(message.ID) > 0 {
				if err := s.respondToServerRequest(message); err != nil {
					return mcpEnvelope{}, err
				}
			}
			continue
		}

		if !mcpIDMatches(message.ID, id) {
			continue
		}
		if message.Error != nil {
			return mcpEnvelope{}, &mcpCallError{Code: message.Error.Code, Message: message.Error.Message}
		}
		return message, nil
	}
}

func (s mcpClientSession) respondToServerRequest(message mcpEnvelope) error {
	if len(message.ID) == 0 {
		return nil
	}

	switch message.Method {
	case "ping":
		return writeMCPMessage(s.writer, map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(message.ID),
			"result":  map[string]any{},
		})
	default:
		return writeMCPMessage(s.writer, map[string]any{
			"jsonrpc": "2.0",
			"id":      json.RawMessage(message.ID),
			"error": map[string]any{
				"code":    -32601,
				"message": "method not supported by work-bridge probe",
			},
		})
	}
}

type mcpHTTPClientSession struct {
	client *http.Client
	url    string
}

func (s *mcpHTTPClientSession) initialize(ctx context.Context, id int) (mcpInitializeResult, error) {
	response, err := s.call(ctx, id, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"resources": map[string]any{},
			"tools":     map[string]any{},
			"prompts":   map[string]any{},
		},
		"clientInfo": map[string]any{
			"name":    "work-bridge",
			"version": Version,
		},
	})
	if err != nil {
		return mcpInitializeResult{}, err
	}

	var result mcpInitializeResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return mcpInitializeResult{}, fmt.Errorf("decode initialize result: %w", err)
	}
	return result, nil
}

func (s *mcpHTTPClientSession) notify(method string, params any) error {
	_, err := s.postJSON(context.Background(), map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	return err
}

func (s *mcpHTTPClientSession) listCount(ctx context.Context, requestID *int, method string, field string) (int, error) {
	total := 0
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		response, err := s.call(ctx, *requestID, method, params)
		*requestID += 1
		if err != nil {
			if callErr, ok := err.(*mcpCallError); ok && callErr.Code == -32601 {
				return 0, nil
			}
			return total, err
		}

		var payload map[string]json.RawMessage
		if err := json.Unmarshal(response.Result, &payload); err != nil {
			return total, fmt.Errorf("decode %s result: %w", method, err)
		}

		var items []json.RawMessage
		if raw, ok := payload[field]; ok && len(raw) > 0 {
			if err := json.Unmarshal(raw, &items); err != nil {
				return total, fmt.Errorf("decode %s items: %w", method, err)
			}
			total += len(items)
		}

		var next string
		if raw, ok := payload["nextCursor"]; ok && len(raw) > 0 {
			if err := json.Unmarshal(raw, &next); err != nil {
				return total, fmt.Errorf("decode %s nextCursor: %w", method, err)
			}
		}
		if next == "" {
			return total, nil
		}
		cursor = next
	}
}

func (s *mcpHTTPClientSession) close() error {
	closeHTTPClientIdleConnections(s.client)
	return nil
}

func (s *mcpHTTPClientSession) call(ctx context.Context, id int, method string, params any) (mcpEnvelope, error) {
	envelopes, err := s.postJSON(ctx, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return mcpEnvelope{}, err
	}
	for _, message := range envelopes {
		if len(message.Method) > 0 {
			continue
		}
		if !mcpIDMatches(message.ID, id) {
			continue
		}
		if message.Error != nil {
			return mcpEnvelope{}, &mcpCallError{Code: message.Error.Code, Message: message.Error.Message}
		}
		return message, nil
	}
	return mcpEnvelope{}, errors.New("missing JSON-RPC response from MCP server")
}

func (s *mcpHTTPClientSession) postJSON(ctx context.Context, payload any) ([]mcpEnvelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return decodeMCPHTTPResponse(resp)
}

type mcpSSEClientSession struct {
	streamClient *http.Client
	postClient   *http.Client
	streamResp   *http.Response
	endpoint     string
	messages     chan mcpEnvelope
	errs         chan error
}

func newMCPSSEClientSession(ctx context.Context, streamClient *http.Client, postClient *http.Client, streamURL string) (*mcpSSEClientSession, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("sse %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	session := &mcpSSEClientSession{
		streamClient: streamClient,
		postClient:   postClient,
		streamResp:   resp,
		messages:     make(chan mcpEnvelope, 16),
		errs:         make(chan error, 1),
	}

	endpointCh := make(chan string, 1)
	go session.readLoop(endpointCh)

	select {
	case endpoint := <-endpointCh:
		if endpoint == "" {
			_ = session.close()
			return nil, errors.New("sse endpoint event missing")
		}
		session.endpoint = endpoint
		return session, nil
	case err := <-session.errs:
		_ = session.close()
		return nil, err
	case <-ctx.Done():
		_ = session.close()
		return nil, ctx.Err()
	}
}

func (s *mcpSSEClientSession) initialize(ctx context.Context, id int) (mcpInitializeResult, error) {
	response, err := s.call(ctx, id, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"resources": map[string]any{},
			"tools":     map[string]any{},
			"prompts":   map[string]any{},
		},
		"clientInfo": map[string]any{
			"name":    "work-bridge",
			"version": Version,
		},
	})
	if err != nil {
		return mcpInitializeResult{}, err
	}

	var result mcpInitializeResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return mcpInitializeResult{}, fmt.Errorf("decode initialize result: %w", err)
	}
	return result, nil
}

func (s *mcpSSEClientSession) notify(method string, params any) error {
	_, err := s.postPayload(context.Background(), map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	return err
}

func (s *mcpSSEClientSession) listCount(ctx context.Context, requestID *int, method string, field string) (int, error) {
	total := 0
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		response, err := s.call(ctx, *requestID, method, params)
		*requestID += 1
		if err != nil {
			if callErr, ok := err.(*mcpCallError); ok && callErr.Code == -32601 {
				return 0, nil
			}
			return total, err
		}

		var payload map[string]json.RawMessage
		if err := json.Unmarshal(response.Result, &payload); err != nil {
			return total, fmt.Errorf("decode %s result: %w", method, err)
		}

		var items []json.RawMessage
		if raw, ok := payload[field]; ok && len(raw) > 0 {
			if err := json.Unmarshal(raw, &items); err != nil {
				return total, fmt.Errorf("decode %s items: %w", method, err)
			}
			total += len(items)
		}

		var next string
		if raw, ok := payload["nextCursor"]; ok && len(raw) > 0 {
			if err := json.Unmarshal(raw, &next); err != nil {
				return total, fmt.Errorf("decode %s nextCursor: %w", method, err)
			}
		}
		if next == "" {
			return total, nil
		}
		cursor = next
	}
}

func (s *mcpSSEClientSession) close() error {
	closeHTTPClientIdleConnections(s.postClient)
	closeHTTPClientIdleConnections(s.streamClient)
	if s.streamResp != nil && s.streamResp.Body != nil {
		return s.streamResp.Body.Close()
	}
	return nil
}

func (s *mcpSSEClientSession) call(ctx context.Context, id int, method string, params any) (mcpEnvelope, error) {
	if _, err := s.postPayload(ctx, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}); err != nil {
		return mcpEnvelope{}, err
	}

	for {
		select {
		case message := <-s.messages:
			if len(message.Method) > 0 {
				if len(message.ID) > 0 {
					if err := s.respondToServerRequest(ctx, message); err != nil {
						return mcpEnvelope{}, err
					}
				}
				continue
			}
			if !mcpIDMatches(message.ID, id) {
				continue
			}
			if message.Error != nil {
				return mcpEnvelope{}, &mcpCallError{Code: message.Error.Code, Message: message.Error.Message}
			}
			return message, nil
		case err := <-s.errs:
			return mcpEnvelope{}, err
		case <-ctx.Done():
			return mcpEnvelope{}, ctx.Err()
		}
	}
}

func (s *mcpSSEClientSession) postPayload(ctx context.Context, payload any) ([]mcpEnvelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.postClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("sse post %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if resp.ContentLength == 0 {
		return nil, nil
	}
	return decodeMCPHTTPResponse(resp)
}

func (s *mcpSSEClientSession) respondToServerRequest(ctx context.Context, message mcpEnvelope) error {
	if len(message.ID) == 0 {
		return nil
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(message.ID),
	}
	switch message.Method {
	case "ping":
		payload["result"] = map[string]any{}
	default:
		payload["error"] = map[string]any{
			"code":    -32601,
			"message": "method not supported by work-bridge probe",
		}
	}
	_, err := s.postPayload(ctx, payload)
	return err
}

func (s *mcpSSEClientSession) readLoop(endpointCh chan<- string) {
	reader := bufio.NewReader(s.streamResp.Body)
	endpointSent := false
	for {
		eventName, data, err := readSSEEvent(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				select {
				case s.errs <- err:
				default:
				}
			}
			if !endpointSent {
				select {
				case endpointCh <- "":
				default:
				}
			}
			return
		}
		switch eventName {
		case "endpoint":
			if !endpointSent {
				endpointSent = true
				endpointCh <- strings.TrimSpace(data)
			}
		default:
			var message mcpEnvelope
			if err := json.Unmarshal([]byte(data), &message); err != nil {
				select {
				case s.errs <- fmt.Errorf("decode sse message: %w", err):
				default:
				}
				return
			}
			s.messages <- message
		}
	}
}

func readSSEEvent(reader *bufio.Reader) (string, string, error) {
	eventName := ""
	dataLines := []string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(dataLines) == 0 && eventName == "" {
				continue
			}
			return eventName, strings.Join(dataLines, "\n"), nil
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func newMCPHTTPClient() *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	if ok && base != nil {
		transport := base.Clone()
		return &http.Client{Transport: transport}
	}
	return &http.Client{}
}

func closeHTTPClientIdleConnections(client *http.Client) {
	if client == nil || client.Transport == nil {
		return
	}
	type idleCloser interface {
		CloseIdleConnections()
	}
	if closer, ok := client.Transport.(idleCloser); ok {
		closer.CloseIdleConnections()
	}
}

func decodeMCPHTTPResponse(resp *http.Response) ([]mcpEnvelope, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, nil
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") || strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, "data:") {
		return decodeMCPEventStream(body)
	}
	var message mcpEnvelope
	if err := json.Unmarshal(body, &message); err != nil {
		return nil, fmt.Errorf("decode mcp http response: %w", err)
	}
	return []mcpEnvelope{message}, nil
}

func decodeMCPEventStream(body []byte) ([]mcpEnvelope, error) {
	reader := bufio.NewReader(bytes.NewReader(body))
	envelopes := []mcpEnvelope{}
	for {
		eventName, data, err := readSSEEvent(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if eventName == "endpoint" || strings.TrimSpace(data) == "" {
			continue
		}
		var message mcpEnvelope
		if err := json.Unmarshal([]byte(data), &message); err != nil {
			return nil, fmt.Errorf("decode sse envelope: %w", err)
		}
		envelopes = append(envelopes, message)
	}
	return envelopes, nil
}

func writeMCPMessage(w io.Writer, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

func readMCPMessage(r *bufio.Reader) (mcpEnvelope, error) {
	contentLength := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return mcpEnvelope{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			fmt.Sscanf(line, "Content-Length: %d", &contentLength)
			if contentLength == 0 {
				fmt.Sscanf(line, "content-length: %d", &contentLength)
			}
		}
	}
	if contentLength <= 0 {
		return mcpEnvelope{}, fmt.Errorf("missing content-length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return mcpEnvelope{}, err
	}

	var message mcpEnvelope
	if err := json.Unmarshal(body, &message); err != nil {
		return mcpEnvelope{}, fmt.Errorf("decode mcp message: %w", err)
	}
	return message, nil
}

func mcpIDMatches(raw json.RawMessage, id int) bool {
	return strings.TrimSpace(string(raw)) == fmt.Sprintf("%d", id)
}

func resolveMCPServerWorkDir(configPath string, cwd string) string {
	base := filepath.Dir(configPath)
	if strings.TrimSpace(cwd) == "" {
		return base
	}
	if filepath.IsAbs(cwd) {
		return filepath.Clean(cwd)
	}
	return filepath.Join(base, cwd)
}

func resolveMCPServerCommand(command string, workDir string) string {
	if command == "" || filepath.IsAbs(command) || !strings.Contains(command, string(filepath.Separator)) {
		return command
	}
	return filepath.Join(workDir, command)
}

func mergeServerEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	values := map[string]string{}
	for _, entry := range base {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	merged := make([]string, 0, len(values))
	for key, value := range values {
		merged = append(merged, key+"="+value)
	}
	sort.Strings(merged)
	return merged
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func stringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		out := append([]string{}, typed...)
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringValue(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func stringMapValue(value any) map[string]string {
	switch typed := value.(type) {
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, val := range typed {
			out[key] = val
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(typed))
		for key, val := range typed {
			switch rendered := val.(type) {
			case string:
				out[key] = rendered
			default:
				out[key] = fmt.Sprint(rendered)
			}
		}
		return out
	default:
		return nil
	}
}

func toolBinary(tool domain.Tool) string {
	switch tool {
	case domain.ToolCodex:
		return "codex"
	case domain.ToolGemini:
		return "gemini"
	case domain.ToolClaude:
		return "claude"
	case domain.ToolOpenCode:
		return "opencode"
	default:
		return ""
	}
}

func (a *App) installSkillFromTUI(ctx context.Context, entry tui.SkillEntry, target tui.SkillTarget) (tui.SkillInstallResult, error) {
	_ = ctx

	srcDir := firstNonEmpty(entry.RootPath, filepath.Dir(entry.Path))
	targetPath := filepath.Clean(target.Path)
	if strings.TrimSpace(targetPath) == "" {
		return tui.SkillInstallResult{}, fmt.Errorf("skill target path is required")
	}
	targetDir := filepath.Dir(targetPath)
	result := tui.SkillInstallResult{
		InstalledPath: targetPath,
		TargetID:      target.ID,
		TargetLabel:   target.Label,
		TargetScope:   target.Scope,
	}

	if filepath.Clean(srcDir) == filepath.Clean(targetDir) {
		result.Warnings = append(result.Warnings, "skill is already at the selected target")
		return result, nil
	}

	if info, statErr := a.fs.Stat(targetDir); statErr == nil && info.IsDir() {
		result.Overwrote = true
	}

	if err := copyDir(a.fs, srcDir, targetDir); err != nil {
		return tui.SkillInstallResult{}, err
	}

	return result, nil
}

func copyDir(fs fsx.FS, src string, dst string) error {
	if err := fs.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := fs.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(fs, srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := fs.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := fs.WriteFile(dstPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sanitizeSkillName(value string) string {
	return stringx.SanitizeName(value)
}

func firstNonEmpty(values ...string) string {
	return stringx.FirstNonEmpty(values...)
}

func dedupeStrings(values []string) []string {
	return stringx.Dedupe(values)
}
