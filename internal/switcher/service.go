package switcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/catalog"
	"github.com/jaeyoung0509/work-bridge/internal/detect"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/importer"
	"github.com/jaeyoung0509/work-bridge/internal/inspect"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
	"golang.org/x/sync/errgroup"
)

type Service struct {
	fs        fsx.FS
	cwd       string
	homeDir   string
	toolPaths domain.ToolPaths
	redaction domain.RedactionPolicy
	lookPath  func(string) (string, error)
	now       func() time.Time
}

type Options struct {
	FS        fsx.FS
	CWD       string
	HomeDir   string
	ToolPaths domain.ToolPaths
	Redaction domain.RedactionPolicy
	LookPath  func(string) (string, error)
	Now       func() time.Time
}

type Request struct {
	From          domain.Tool
	Session       string
	To            domain.Tool
	ProjectRoot   string
	IncludeSkills bool
	IncludeMCP    bool
	DryRun        bool
}

type Result struct {
	Payload domain.SwitchPayload `json:"payload"`
	Plan    domain.SwitchPlan    `json:"plan"`
	Report  *domain.ApplyReport  `json:"report,omitempty"`
}

type Workspace struct {
	ProjectRoot string          `json:"project_root"`
	Sessions    []WorkspaceItem `json:"sessions"`
}

type WorkspaceItem struct {
	Tool        domain.Tool `json:"tool"`
	ID          string      `json:"id"`
	Title       string      `json:"title,omitempty"`
	ProjectRoot string      `json:"project_root,omitempty"`
	UpdatedAt   string      `json:"updated_at,omitempty"`
}

func New(opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		fs:        opts.FS,
		cwd:       opts.CWD,
		homeDir:   opts.HomeDir,
		toolPaths: opts.ToolPaths,
		redaction: opts.Redaction,
		lookPath:  opts.LookPath,
		now:       now,
	}
}

func (s *Service) LoadWorkspace(ctx context.Context) (Workspace, error) {
	projectRoot, err := s.resolveProjectRoot("")
	if err != nil {
		return Workspace{}, err
	}

	items := make([]WorkspaceItem, 0, 32)
	var mu errCollector
	group, ctx := errgroup.WithContext(ctx)
	for _, tool := range []domain.Tool{domain.ToolCodex, domain.ToolGemini, domain.ToolClaude, domain.ToolOpenCode} {
		tool := tool
		group.Go(func() error {
			report, err := inspect.Run(inspect.Options{
				FS:        s.fs,
				CWD:       projectRoot,
				HomeDir:   s.homeDir,
				ToolPaths: s.toolPaths,
				Tool:      string(tool),
				LookPath:  s.lookPath,
				Limit:     100,
			})
			if err != nil {
				return err
			}
			filtered := make([]WorkspaceItem, 0, len(report.Sessions))
			for _, session := range report.Sessions {
				if !pathWithinRoot(session.ProjectRoot, projectRoot) {
					continue
				}
				filtered = append(filtered, WorkspaceItem{
					Tool:        tool,
					ID:          session.ID,
					Title:       firstNonEmpty(session.Title, session.ID),
					ProjectRoot: session.ProjectRoot,
					UpdatedAt:   session.UpdatedAt,
				})
			}
			mu.AppendItems(&items, filtered)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return Workspace{}, err
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			if items[i].Tool == items[j].Tool {
				return items[i].Title < items[j].Title
			}
			return items[i].Tool < items[j].Tool
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})

	return Workspace{
		ProjectRoot: projectRoot,
		Sessions:    items,
	}, nil
}

func (s *Service) Preview(ctx context.Context, req Request) (Result, error) {
	resolved, err := s.resolveRequest(ctx, req)
	if err != nil {
		return Result{}, err
	}
	adapter, err := s.adapterFor(resolved.req.To)
	if err != nil {
		return Result{}, err
	}
	plan, err := adapter.Preview(resolved.payload, resolved.req.ProjectRoot)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Payload: resolved.payload,
		Plan:    plan,
	}, nil
}

func (s *Service) Apply(ctx context.Context, req Request) (Result, error) {
	resolved, err := s.resolveRequest(ctx, req)
	if err != nil {
		return Result{}, err
	}
	adapter, err := s.adapterFor(resolved.req.To)
	if err != nil {
		return Result{}, err
	}
	plan, err := adapter.Preview(resolved.payload, resolved.req.ProjectRoot)
	if err != nil {
		return Result{}, err
	}
	report, err := adapter.ApplyProject(resolved.payload, plan)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Payload: resolved.payload,
		Plan:    plan,
		Report:  &report,
	}, nil
}

func (s *Service) Export(ctx context.Context, req Request, outRoot string) (Result, error) {
	resolved, err := s.resolveRequest(ctx, req)
	if err != nil {
		return Result{}, err
	}
	exportRoot, err := s.resolveOutputRoot(outRoot)
	if err != nil {
		return Result{}, err
	}
	adapter, err := s.adapterFor(resolved.req.To)
	if err != nil {
		return Result{}, err
	}
	plan, err := adapter.Preview(resolved.payload, exportRoot)
	if err != nil {
		return Result{}, err
	}
	if req.DryRun {
		return Result{
			Payload: resolved.payload,
			Plan:    plan,
		}, nil
	}
	report, err := adapter.ExportProject(resolved.payload, plan)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Payload: resolved.payload,
		Plan:    plan,
		Report:  &report,
	}, nil
}

type resolvedRequest struct {
	req     Request
	payload domain.SwitchPayload
}

func (s *Service) resolveRequest(ctx context.Context, req Request) (resolvedRequest, error) {
	if s.fs == nil {
		return resolvedRequest{}, errors.New("fs is required")
	}
	if !req.From.IsKnown() {
		return resolvedRequest{}, fmt.Errorf("unsupported source tool %q", req.From)
	}
	if !req.To.IsKnown() {
		return resolvedRequest{}, fmt.Errorf("unsupported target tool %q", req.To)
	}
	projectRoot, err := s.resolveProjectRoot(req.ProjectRoot)
	if err != nil {
		return resolvedRequest{}, err
	}
	req.ProjectRoot = projectRoot
	if req.Session == "" {
		req.Session = "latest"
	}
	if !req.IncludeSkills && !req.IncludeMCP {
		// session-only preview/apply is valid
	}

	sessionID, err := s.resolveSessionID(ctx, req.From, req.Session, projectRoot)
	if err != nil {
		return resolvedRequest{}, err
	}

	bundle, err := importer.Import(importer.Options{
		FS:         s.fs,
		CWD:        projectRoot,
		HomeDir:    s.homeDir,
		ToolPaths:  s.toolPaths,
		Tool:       string(req.From),
		Session:    sessionID,
		ImportedAt: s.now().Format(time.RFC3339),
		Redaction:  s.redaction,
		LookPath:   s.lookPath,
	})
	if err != nil {
		return resolvedRequest{}, err
	}

	skills := []domain.SkillPayload{}
	if req.IncludeSkills {
		skills, err = s.collectSkills(projectRoot)
		if err != nil {
			return resolvedRequest{}, err
		}
	}

	mcp := domain.MCPPayload{
		Sources: []domain.MCPSource{},
		Servers: map[string]domain.MCPServerConfig{},
	}
	if req.IncludeMCP {
		mcp, err = s.collectMCP(projectRoot)
		if err != nil {
			return resolvedRequest{}, err
		}
	}

	payload := domain.SwitchPayload{
		Bundle:   bundle,
		Skills:   skills,
		MCP:      mcp,
		Warnings: dedupeStrings(append([]string{}, bundle.Warnings...)),
	}
	if !pathWithinRoot(bundle.ProjectRoot, projectRoot) {
		payload.Warnings = dedupeStrings(append(payload.Warnings, "selected session did not map cleanly to the requested project; using the best available session match"))
	}
	return resolvedRequest{
		req:     req,
		payload: payload,
	}, nil
}

func (s *Service) resolveProjectRoot(projectRoot string) (string, error) {
	if strings.TrimSpace(projectRoot) == "" {
		report, err := detect.Run(detect.Options{
			FS:        s.fs,
			CWD:       s.cwd,
			HomeDir:   s.homeDir,
			ToolPaths: s.toolPaths,
			LookPath:  s.lookPath,
		})
		if err != nil {
			return "", err
		}
		projectRoot = report.ProjectRoot
	}
	if !filepath.IsAbs(projectRoot) {
		projectRoot = filepath.Clean(filepath.Join(s.cwd, projectRoot))
	}
	info, err := s.fs.Stat(projectRoot)
	if err != nil {
		return "", fmt.Errorf("project %q: %w", projectRoot, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project %q is not a directory", projectRoot)
	}
	return filepath.Clean(projectRoot), nil
}

func (s *Service) resolveOutputRoot(outRoot string) (string, error) {
	outRoot = strings.TrimSpace(outRoot)
	if outRoot == "" {
		return "", errors.New("output root is required")
	}
	if !filepath.IsAbs(outRoot) {
		outRoot = filepath.Join(s.cwd, outRoot)
	}
	return filepath.Clean(outRoot), nil
}

func (s *Service) resolveSessionID(ctx context.Context, tool domain.Tool, requested string, projectRoot string) (string, error) {
	if requested != "" && requested != "latest" {
		return requested, nil
	}
	report, err := inspect.Run(inspect.Options{
		FS:        s.fs,
		CWD:       projectRoot,
		HomeDir:   s.homeDir,
		ToolPaths: s.toolPaths,
		Tool:      string(tool),
		LookPath:  s.lookPath,
		Limit:     100,
	})
	if err != nil {
		return "", err
	}
	for _, session := range report.Sessions {
		if pathWithinRoot(session.ProjectRoot, projectRoot) {
			return session.ID, nil
		}
	}
	if len(report.Sessions) > 0 {
		return report.Sessions[0].ID, nil
	}
	return "", &importer.SessionNotFoundError{Tool: string(tool), Session: requested}
}

func (s *Service) collectSkills(projectRoot string) ([]domain.SkillPayload, error) {
	entries, err := catalog.ScanSkills(s.fs, projectRoot, s.homeDir)
	if err != nil {
		return nil, err
	}
	skills := make([]domain.SkillPayload, 0, len(entries))
	for _, entry := range entries {
		if entry.Scope == "project" && !pathWithinRoot(entry.Path, projectRoot) {
			continue
		}
		data, err := s.fs.ReadFile(entry.Path)
		if err != nil {
			return nil, err
		}
		skills = append(skills, domain.SkillPayload{
			Name:        entry.Name,
			Description: entry.Description,
			Path:        entry.Path,
			Scope:       entry.Scope,
			Tool:        domain.Tool(entry.Tool),
			Content:     string(data),
		})
	}
	sort.SliceStable(skills, func(i, j int) bool {
		if skills[i].Name == skills[j].Name {
			if skills[i].Scope == skills[j].Scope {
				return skills[i].Path < skills[j].Path
			}
			return skills[i].Scope < skills[j].Scope
		}
		return skills[i].Name < skills[j].Name
	})
	return skills, nil
}

func (s *Service) collectMCP(projectRoot string) (domain.MCPPayload, error) {
	entries, err := catalog.ScanMCP(s.fs, projectRoot, s.homeDir, s.toolPaths)
	if err != nil {
		return domain.MCPPayload{}, err
	}

	payload := domain.MCPPayload{
		Sources:  []domain.MCPSource{},
		Servers:  map[string]domain.MCPServerConfig{},
		Warnings: []string{},
	}

	type candidate struct {
		scopeRank int
		server    domain.MCPServerConfig
	}
	merged := map[string]candidate{}
	for _, entry := range entries {
		if entry.Source == "project" || entry.Source == "local" || pathWithinRoot(entry.Path, projectRoot) {
			// keep project-scoped entries
		} else if entry.Source != "user" && entry.Source != "global" && entry.Source != "legacy" {
			continue
		}

		data, err := s.fs.ReadFile(entry.Path)
		if err != nil {
			return domain.MCPPayload{}, err
		}
		summary := summarizeMCPConfig(entry.Path, data)
		source := domain.MCPSource{
			Path:          entry.Path,
			Scope:         entry.Source,
			Tool:          inferMCPTool(entry.Name, entry.Path),
			Format:        summary.Format,
			Status:        summary.Status,
			ServerNames:   append([]string{}, summary.ServerNames...),
			Servers:       append([]domain.MCPServerConfig{}, summary.Servers...),
			ParseWarnings: append([]string{}, summary.Warnings...),
			RawConfig:     string(data),
		}
		payload.Sources = append(payload.Sources, source)
		payload.Warnings = append(payload.Warnings, summary.Warnings...)

		for _, server := range summary.Servers {
			current, ok := merged[server.Name]
			rank := mcpScopeRank(entry.Source)
			if !ok || rank < current.scopeRank {
				merged[server.Name] = candidate{
					scopeRank: rank,
					server:    server,
				}
			}
		}
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		payload.Servers[name] = merged[name].server
	}
	payload.Warnings = dedupeStrings(payload.Warnings)
	sort.SliceStable(payload.Sources, func(i, j int) bool {
		if payload.Sources[i].Scope == payload.Sources[j].Scope {
			return payload.Sources[i].Path < payload.Sources[j].Path
		}
		return mcpScopeRank(payload.Sources[i].Scope) < mcpScopeRank(payload.Sources[j].Scope)
	})
	return payload, nil
}

func (s *Service) adapterFor(target domain.Tool) (domain.TargetAdapter, error) {
	return &projectAdapter{
		target: target,
		fs:  s.fs,
		now: s.now,
	}, nil
}

func managedRoot(projectRoot string, target domain.Tool) string {
	return filepath.Join(projectRoot, ".work-bridge", string(target))
}

func bundleManagedFiles(bundle domain.SessionBundle, report domain.CompatibilityReport, root string) []string {
	files := make([]string, 0, len(report.GeneratedArtifacts)+1)
	for _, name := range report.GeneratedArtifacts {
		files = append(files, filepath.Join(root, name))
	}
	files = append(files, filepath.Join(root, "manifest.json"))
	return files
}

func relativeProjectPath(projectRoot string, path string) string {
	if rel, err := filepath.Rel(projectRoot, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func marshalJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}\n"
	}
	return string(data) + "\n"
}

type errCollector struct {
	itemsMu sync.Mutex
}

func (c *errCollector) AppendItems(dst *[]WorkspaceItem, items []WorkspaceItem) {
	c.itemsMu.Lock()
	defer c.itemsMu.Unlock()
	*dst = append(*dst, items...)
}
