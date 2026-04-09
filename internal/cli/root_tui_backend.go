package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	"github.com/jaeyoung0509/work-bridge/internal/tui"
)

var (
	jsoncLineComment  = regexp.MustCompile(`(?m)^\s*//.*$`)
	jsoncBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	errMCPMethod      = fmt.Errorf("mcp method error")
)

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
				Path:        entry.Path,
				Source:      entry.Source,
			}
			if data, readErr := a.fs.ReadFile(entry.Path); readErr == nil {
				skill.Content = string(data)
			}
			skills = append(skills, skill)
		}
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
		enriched := make([]tui.MCPEntry, 0, len(entries))
		for _, entry := range entries {
			profile := tui.MCPEntry{
				Name:   entry.Name,
				Path:   entry.Path,
				Source: entry.Source,
				Status: "configured",
				Tool:   inferMCPTool(entry.Name, entry.Path),
			}
			if data, readErr := a.fs.ReadFile(entry.Path); readErr == nil {
				profile.RawConfig = string(data)
				summary := summarizeMCPConfig(entry.Path, data)
				profile.Format = summary.Format
				profile.Status = summary.Status
				profile.ParseWarnings = summary.Warnings
				profile.ServerNames = append([]string{}, summary.ServerNames...)
				profile.Servers = append([]tui.MCPServerConfig{}, summary.Servers...)
				profile.DeclaredServers = len(summary.ServerNames)
				profile.ParseSource = summary.ParseSource
				if len(summary.ServerNames) > 0 {
					profile.Details = fmt.Sprintf("%d MCP server(s)", len(summary.ServerNames))
				} else {
					profile.Details = "config present"
				}
			} else {
				profile.Status = "broken"
				profile.ParseWarnings = []string{fmt.Sprintf("read failed: %v", readErr)}
				profile.Details = "config unreadable"
			}
			if profile.Tool != "" {
				binary := toolBinary(profile.Tool)
				if path, lookErr := a.look(binary); lookErr == nil && path != "" {
					profile.BinaryFound = true
					profile.BinaryPath = path
				}
			}
			enriched = append(enriched, profile)
		}
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
		})
	}

	for _, report := range snapshot.InspectByTool {
		for _, session := range report.Sessions {
			projectPath := firstNonEmpty(session.ProjectRoot, session.StoragePath)
			if idx := projectIndexForPath(projects, projectPath); idx >= 0 {
				projects[idx].SessionCount++
			}
		}
	}
	for _, skill := range snapshot.Skills {
		if idx := projectIndexForPath(projects, skill.Path); idx >= 0 {
			projects[idx].SkillCount++
		}
	}
	for _, mcp := range snapshot.MCPProfiles {
		if idx := projectIndexForPath(projects, mcp.Path); idx >= 0 {
			projects[idx].MCPCount++
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
				Warnings: []string{fmt.Sprintf("parse failed: %v", err)},
			}
		}
	case "json", "jsonc":
		sanitized := jsoncBlockComment.ReplaceAllString(jsoncLineComment.ReplaceAllString(string(data), ""), "")
		if err := json.Unmarshal([]byte(sanitized), &parsed); err != nil {
			return mcpConfigSummary{
				Format:   format,
				Status:   "broken",
				Warnings: []string{fmt.Sprintf("parse failed: %v", err)},
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
		if serverResult.Transport == "" || serverResult.Transport == "stdio" {
			runtimeAttempted++
		}
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
	case runtimeAttempted > 0:
		result.Mode = "runtime-stdio"
	default:
		result.Mode = "config-only"
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
	case "http", "streamable-http", "sse":
		result.Error = fmt.Sprintf("unsupported %s transport", server.Transport)
		result.Warnings = append(result.Warnings, "runtime probing currently supports stdio MCP servers only")
	default:
		result.Error = fmt.Sprintf("unsupported %s transport", firstNonEmpty(server.Transport, "unknown"))
	}

	if result.Latency == "" {
		result.Latency = time.Since(started).Round(time.Millisecond).String()
	}
	return result
}

func (a *App) probeMCPStdioServer(ctx context.Context, configPath string, server tui.MCPServerConfig) (tui.MCPServerProbeResult, error) {
	started := time.Now()
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

	requestID := 1
	initializeResult, err := session.initialize(ctx, requestID)
	if err != nil {
		if stderrBuf.Len() > 0 {
			return result, fmt.Errorf("%w; stderr: %s", err, strings.TrimSpace(stderrBuf.String()))
		}
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

func (a *App) installSkillFromTUI(ctx context.Context, entry tui.SkillEntry) (tui.SkillInstallResult, error) {
	_ = ctx

	cwd, _, err := a.resolveWorkingDirs()
	if err != nil {
		return tui.SkillInstallResult{}, err
	}

	srcDir := filepath.Dir(entry.Path)
	targetName := filepath.Base(srcDir)
	if targetName == "" || targetName == "." || targetName == string(filepath.Separator) {
		targetName = sanitizeSkillName(entry.Name)
	}
	if targetName == "" {
		targetName = "skill"
	}

	targetDir := filepath.Join(cwd, "skills", targetName)
	result := tui.SkillInstallResult{
		InstalledPath: filepath.Join(targetDir, "SKILL.md"),
	}

	if filepath.Clean(srcDir) == filepath.Clean(targetDir) {
		result.Warnings = append(result.Warnings, "skill is already project-local")
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
