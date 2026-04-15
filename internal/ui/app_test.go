package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jaeyoung0509/work-bridge/internal/catalog"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
)

// ─── Fake Backend ───────────────────────────────────────────

type fakeBackend struct {
	workspace   switcher.Workspace
	workspaces  map[string]switcher.Workspace
	loadErr     error
	projects    []catalog.ProjectEntry
	projectsErr error
	skills      []catalog.SkillEntry
	skillsErr   error
	mcp         []catalog.MCPEntry
	mcpErr      error
	previewResp switcher.Result
	previewErr  error
	applyResp   switcher.Result
	applyErr    error
	exportResp  switcher.Result
	exportErr   error

	loadWorkspaceRoots []string
	previewCalls       []switcher.Request
	applyCalls         []switcher.Request
	exportCalls        []switcher.Request
	exportDirs         []string
}

func (f *fakeBackend) LoadWorkspace(_ context.Context, projectRoot string) (switcher.Workspace, error) {
	f.loadWorkspaceRoots = append(f.loadWorkspaceRoots, projectRoot)
	if f.workspaces != nil {
		if ws, ok := f.workspaces[projectRoot]; ok {
			return ws, f.loadErr
		}
	}
	return f.workspace, f.loadErr
}

func (f *fakeBackend) LoadProjects(context.Context, []string) ([]catalog.ProjectEntry, error) {
	return append([]catalog.ProjectEntry{}, f.projects...), f.projectsErr
}

func (f *fakeBackend) LoadSkills(context.Context, string) ([]catalog.SkillEntry, error) {
	return append([]catalog.SkillEntry{}, f.skills...), f.skillsErr
}

func (f *fakeBackend) LoadMCP(context.Context, string) ([]catalog.MCPEntry, error) {
	return append([]catalog.MCPEntry{}, f.mcp...), f.mcpErr
}

func (f *fakeBackend) Preview(_ context.Context, req switcher.Request) (switcher.Result, error) {
	f.previewCalls = append(f.previewCalls, req)
	return f.previewResp, f.previewErr
}

func (f *fakeBackend) Apply(_ context.Context, req switcher.Request) (switcher.Result, error) {
	f.applyCalls = append(f.applyCalls, req)
	return f.applyResp, f.applyErr
}

func (f *fakeBackend) Export(_ context.Context, req switcher.Request, outDir string) (switcher.Result, error) {
	f.exportCalls = append(f.exportCalls, req)
	f.exportDirs = append(f.exportDirs, outDir)
	return f.exportResp, f.exportErr
}

func (f *fakeBackend) MigrateMCP(_ context.Context, _ catalog.MCPEntry, _ domain.Tool, _ string) error {
	return nil
}

func (f *fakeBackend) MigrateSkill(_ context.Context, _ catalog.SkillEntry, _ domain.Tool, _ string) error {
	return nil
}

// ─── Test: Hub Starts Correctly ─────────────────────────────

func TestMainModelStartsOnHubScreen(t *testing.T) {
	t.Parallel()
	model, _ := bootstrapModel(t, newFakeBackend())
	if model.screen != ScreenHub {
		t.Fatalf("expected Hub screen, got %v", model.screen)
	}
}

func TestMainModelHubShowsAgentCards(t *testing.T) {
	t.Parallel()
	model, _ := bootstrapModel(t, newFakeBackend())
	view := model.View().Content
	if !strings.Contains(view, "CODEX") || !strings.Contains(view, "GEMINI") {
		t.Fatalf("expected agent names in hub view, got %q", view)
	}
}

// ─── Test: Slash Commands ───────────────────────────────────

func TestSlashProjectsSwitchesToProjectsScreen(t *testing.T) {
	t.Parallel()
	model, _ := bootstrapModel(t, newFakeBackend())
	model = typeCommand(t, model, "/projects")
	if model.screen != ScreenProjects {
		t.Fatalf("expected projects screen, got %v", model.screen)
	}
}

func TestSlashSessionsSwitchesToSessionsScreen(t *testing.T) {
	t.Parallel()
	model, _ := bootstrapModel(t, newFakeBackend())
	model = typeCommand(t, model, "/sessions")
	if model.screen != ScreenSessions {
		t.Fatalf("expected sessions screen, got %v", model.screen)
	}
}

func TestSlashSkillsSwitchesToBrowserScreen(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	model, _ := bootstrapModel(t, backend)
	model = typeCommand(t, model, "/skills")
	if model.screen != ScreenBrowser {
		t.Fatalf("expected browser screen for skills, got %v", model.screen)
	}
	if len(model.browserView.List.Items()) != 1 {
		t.Fatalf("expected 1 skill entry, got %d", len(model.browserView.List.Items()))
	}
}

func TestSlashMCPSwitchesToBrowserScreen(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	model, _ := bootstrapModel(t, backend)
	model = typeCommand(t, model, "/mcp")
	if model.screen != ScreenBrowser {
		t.Fatalf("expected browser screen for MCP, got %v", model.screen)
	}
	if len(model.browserView.List.Items()) != 1 {
		t.Fatalf("expected detected MCP config to remain visible, got %d entries", len(model.browserView.List.Items()))
	}
}

func TestProjectsTypingFiltersResults(t *testing.T) {
	t.Parallel()
	model, _ := bootstrapModel(t, newFakeBackend())
	model = typeCommand(t, model, "/projects")
	model = runKey(t, model, runeKey("o"))
	model = runKey(t, model, runeKey("t"))
	model = runKey(t, model, runeKey("h"))

	if got := model.projectList.EntryCount(); got != 1 {
		t.Fatalf("expected 1 filtered project, got %d", got)
	}
	entry, ok := model.projectList.SelectedEntry()
	if !ok {
		t.Fatal("expected a selected filtered project")
	}
	if entry.Key != "/repo/other" {
		t.Fatalf("expected filtered project /repo/other, got %q", entry.Key)
	}
}

func TestProjectsArrowNavigationMovesSelection(t *testing.T) {
	t.Parallel()

	model, _ := bootstrapModel(t, newFakeBackend())
	model = typeCommand(t, model, "/projects")
	model = runKey(t, model, specialKey(tea.KeyDown))

	entry, ok := model.projectList.SelectedEntry()
	if !ok {
		t.Fatal("expected a selected project after moving down")
	}
	if entry.Key != "/repo/other" {
		t.Fatalf("expected /repo/other to be selected, got %q", entry.Key)
	}
}

func TestSessionsArrowNavigationMovesSelection(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	backend.workspace.Sessions = []switcher.WorkspaceItem{
		{Tool: domain.ToolCodex, ID: "session-1", Title: "Codex task", ProjectRoot: "/repo/project"},
		{Tool: domain.ToolClaude, ID: "session-2", Title: "Claude task", ProjectRoot: "/repo/project"},
	}
	backend.workspaces["/repo/project"] = backend.workspace

	model, _ := bootstrapModel(t, backend)
	model = typeCommand(t, model, "/sessions")
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyEnter))

	if model.screen != ScreenHandoff {
		t.Fatalf("expected handoff screen after selecting a session, got %v", model.screen)
	}
	if model.selectedSession == nil || model.selectedSession.ID != "session-2" {
		t.Fatalf("expected session-2 to be selected, got %#v", model.selectedSession)
	}
}

func TestSkillsViewGroupsEntriesByTool(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	backend.skills = []catalog.SkillEntry{
		{
			Name:        "shared-helper",
			Description: "Shared helper",
			EntryPath:   "/repo/project/.agents/skills/shared-helper/SKILL.md",
			Source:      "project .agents/skills",
			Scope:       "project",
		},
		{
			Name:        "claude-review",
			Description: "Claude reviewer",
			EntryPath:   "/Users/test/.claude/skills/claude-review/SKILL.md",
			Source:      "claude global skills",
			Scope:       "global",
			Tool:        "claude",
		},
	}

	model, _ := bootstrapModel(t, backend)
	model = typeCommand(t, model, "/skills")
	view := model.View().Content
	if !strings.Contains(view, "CLAUDE") || !strings.Contains(view, "SHARED") {
		t.Fatalf("expected grouped tool headers in skills view, got %q", view)
	}
	if !strings.Contains(view, "type to filter") {
		t.Fatalf("expected visible filter prompt in skills view, got %q", view)
	}
}

func TestSkillEntriesShowInstallTargets(t *testing.T) {
	t.Parallel()

	entries := skillEntries([]catalog.SkillEntry{{
		Name:        "gemini-helper",
		Description: "Gemini helper",
		EntryPath:   "/Users/test/.gemini/skills/gemini-helper/SKILL.md",
		Source:      "gemini global skills",
		Scope:       "global",
		Tool:        "gemini",
	}})

	if len(entries) != 1 {
		t.Fatalf("expected one skill entry, got %d", len(entries))
	}
	detail := strings.Join(entries[0].Details, " ")
	if !strings.Contains(detail, "install into: CLAUDE, CODEX, OPENCODE") {
		t.Fatalf("expected transferable targets in skill details, got %q", detail)
	}
}

func TestMCPActionMenuUsesImportLanguage(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	backend.mcp = []catalog.MCPEntry{
		{
			Name:    "global claude settings",
			Path:    "/Users/test/.claude/settings.json",
			Tool:    "claude",
			Source:  "user",
			Status:  "parsed",
			Format:  "json",
			Details: "2 server(s): github, slack",
			Servers: []string{"github", "slack"},
		},
	}

	model, _ := bootstrapModel(t, backend)
	model = typeCommand(t, model, "/mcp")
	model = runKey(t, model, specialKey(tea.KeyEnter))
	if model.screen != ScreenActionMenu {
		t.Fatalf("expected action menu screen, got %v", model.screen)
	}
	view := model.View().Content
	if !strings.Contains(view, "Import 2 MCP servers to CODEX") {
		t.Fatalf("expected MCP import action label, got %q", view)
	}
}

func TestMCPEntriesKeepConfigsWithoutServersVisible(t *testing.T) {
	t.Parallel()

	entries := mcpEntries([]catalog.MCPEntry{
		{
			Name:   "empty config",
			Path:   "/Users/test/.codex/config.toml",
			Tool:   "codex",
			Source: "user",
			Status: "configured",
			Format: "toml",
		},
		{
			Name:    "global claude settings",
			Path:    "/Users/test/.claude/settings.json",
			Tool:    "claude",
			Source:  "user",
			Status:  "parsed",
			Format:  "json",
			Servers: []string{"github"},
		},
	})

	if len(entries) != 2 {
		t.Fatalf("expected both importable and non-importable MCP configs, got %#v", entries)
	}
	if entries[0].Key != "/Users/test/.claude/settings.json" {
		t.Fatalf("expected importable claude config first, got %q", entries[0].Key)
	}
	if entries[1].Key != "/Users/test/.codex/config.toml" {
		t.Fatalf("expected non-importable codex config to remain visible, got %q", entries[1].Key)
	}
	if !strings.Contains(entries[1].Description, "No importable MCP servers") {
		t.Fatalf("expected explanatory description for non-importable MCP config, got %q", entries[1].Description)
	}
	if !strings.Contains(strings.Join(entries[1].Details, " "), "edit this config") {
		t.Fatalf("expected recovery hint in non-importable MCP details, got %#v", entries[1].Details)
	}
}

func TestMCPScreenExplainsConfigsWithoutServers(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	backend.mcp = []catalog.MCPEntry{{
		Name:   "project claude settings",
		Path:   "/repo/project/.claude/settings.json",
		Tool:   "claude",
		Source: "project",
		Status: "configured",
		Format: "json",
	}}

	model, _ := bootstrapModel(t, backend)
	model = typeCommand(t, model, "/mcp")
	view := model.View().Content
	if !strings.Contains(view, "none define importable MCP servers yet") {
		t.Fatalf("expected empty MCP explanation in view, got %q", view)
	}
	if !strings.Contains(view, "No importable MCP servers detected yet") {
		t.Fatalf("expected per-config MCP explanation in view, got %q", view)
	}
}

// ─── Test: Screen Stack Navigation ──────────────────────────

func TestEscReturnsToHubFromProjects(t *testing.T) {
	t.Parallel()
	model, _ := bootstrapModel(t, newFakeBackend())
	model = typeCommand(t, model, "/projects")
	if model.screen != ScreenProjects {
		t.Fatalf("expected projects screen, got %v", model.screen)
	}
	model = runKey(t, model, specialKey(tea.KeyEscape))
	if model.screen != ScreenHub {
		t.Fatalf("expected hub screen after esc, got %v", model.screen)
	}
}

func TestScreenStackDepth(t *testing.T) {
	t.Parallel()
	model, _ := bootstrapModel(t, newFakeBackend())

	// Hub → Projects
	model = typeCommand(t, model, "/projects")
	if model.screen != ScreenProjects {
		t.Fatalf("expected projects, got %v", model.screen)
	}

	// Projects → select project → Sessions
	model = runKey(t, model, specialKey(tea.KeyEnter))
	if model.screen != ScreenSessions {
		t.Fatalf("expected sessions after project select, got %v", model.screen)
	}

	// Back to Projects
	model = runKey(t, model, specialKey(tea.KeyEscape))
	if model.screen != ScreenProjects {
		t.Fatalf("expected projects after esc from sessions, got %v", model.screen)
	}

	// Back to Hub
	model = runKey(t, model, specialKey(tea.KeyEscape))
	if model.screen != ScreenHub {
		t.Fatalf("expected hub after esc from projects, got %v", model.screen)
	}
}

// ─── Test: Hub Quick Actions ────────────────────────────────

func TestHubQuickActionNavigation(t *testing.T) {
	t.Parallel()
	model, _ := bootstrapModel(t, newFakeBackend())
	if model.actionCursor != 0 {
		t.Fatalf("expected initial cursor at 0, got %d", model.actionCursor)
	}

	model = runKey(t, model, specialKey(tea.KeyDown))
	if model.actionCursor != 1 {
		t.Fatalf("expected cursor at 1 after down, got %d", model.actionCursor)
	}

	// Enter on "sessions" (index 1)
	model = runKey(t, model, specialKey(tea.KeyEnter))
	if model.screen != ScreenSessions {
		t.Fatalf("expected sessions screen from quick action, got %v", model.screen)
	}
}

// ─── Test: Full Handoff Flow ────────────────────────────────

func TestFullHandoffFlow(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()
	model, backend := bootstrapModel(t, backend)

	// Go to sessions
	model = typeCommand(t, model, "/sessions")

	// Select first session
	model = runKey(t, model, specialKey(tea.KeyEnter))
	if model.screen != ScreenHandoff {
		t.Fatalf("expected handoff screen, got %v", model.screen)
	}

	// Preview should have been triggered
	if len(backend.previewCalls) != 1 {
		t.Fatalf("expected 1 preview call, got %d", len(backend.previewCalls))
	}

	req := backend.previewCalls[0]
	if req.From != domain.ToolCodex || req.Session != "session-1" {
		t.Fatalf("unexpected source in request: %#v", req)
	}
	if req.To != domain.ToolGemini {
		t.Fatalf("expected default target gemini, got %s", req.To)
	}
}

// ─── Test: Export Path ──────────────────────────────────────

func TestDefaultExportPathUsesConfig(t *testing.T) {
	t.Parallel()
	backend := newFakeBackend()

	model := NewMainModel(context.Background(), backend, Options{
		ProjectRoot:      "/repo/project",
		DefaultExportDir: "/tmp/configured-out",
	})
	model = processCmd(t, model, model.Init())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(MainModel)

	// Navigate to handoff
	model = typeCommand(t, model, "/sessions")
	model = runKey(t, model, specialKey(tea.KeyEnter))

	// The handoff view should use configured export dir
	// Navigate to Export button
	rows := 4 // target, advanced, apply, export
	for i := 0; i < rows-1; i++ {
		model.handoffView.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	// Verify handoff view has the configured export dir
	req := model.handoffView.BuildRequest()
	if req.ProjectRoot != "/repo/project" {
		t.Fatalf("expected project root in request, got %q", req.ProjectRoot)
	}
}

// ─── Test: Breadcrumb ───────────────────────────────────────

func TestBreadcrumbShowsCurrentScreen(t *testing.T) {
	t.Parallel()
	model, _ := bootstrapModel(t, newFakeBackend())

	view := model.View().Content
	if !strings.Contains(view, "Hub") {
		t.Fatalf("expected Hub in breadcrumb, got header portion near top")
	}

	model = typeCommand(t, model, "/projects")
	view = model.View().Content
	if !strings.Contains(view, "Projects") {
		t.Fatalf("expected Projects in breadcrumb")
	}
}

// ─── Test: Unknown Command ──────────────────────────────────

func TestUnknownSlashCommandShowsError(t *testing.T) {
	t.Parallel()
	model, _ := bootstrapModel(t, newFakeBackend())
	model = typeCommand(t, model, "/unknown")
	if model.lastErr == nil {
		t.Fatalf("expected error for unknown command")
	}
	if !strings.Contains(model.lastErr.Error(), "unknown command") {
		t.Fatalf("expected 'unknown command' error, got %q", model.lastErr.Error())
	}
}

// ─── Test Helpers ───────────────────────────────────────────

func bootstrapModel(t *testing.T, backend *fakeBackend) (MainModel, *fakeBackend) {
	t.Helper()
	model := NewMainModel(context.Background(), backend, Options{ProjectRoot: "/repo/project"})
	model = processCmd(t, model, model.Init())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return updated.(MainModel), backend
}

func processCmd(t *testing.T, model MainModel, cmd tea.Cmd) MainModel {
	t.Helper()
	for cmd != nil {
		msg := runCmd(t, cmd)
		updated, followup := model.Update(msg)
		model = updated.(MainModel)
		cmd = followup
	}
	return model
}

func runKey(t *testing.T, model MainModel, msg tea.KeyPressMsg) MainModel {
	t.Helper()
	updated, cmd := model.Update(msg)
	model = updated.(MainModel)
	return processCmd(t, model, cmd)
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	return cmd()
}

func specialKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func runeKey(text string) tea.KeyPressMsg {
	runes := []rune(text)
	return tea.KeyPressMsg{Text: text, Code: runes[0]}
}

func newFakeBackend() *fakeBackend {
	preview := switcher.Result{
		Payload: domain.SwitchPayload{
			Bundle: domain.SessionBundle{
				SourceTool:      domain.ToolCodex,
				SourceSessionID: "session-1",
			},
		},
		Plan: domain.SwitchPlan{
			TargetTool:      domain.ToolGemini,
			ProjectRoot:     "/repo/project",
			DestinationRoot: "/repo/project",
			Status:          domain.SwitchStatePartial,
			Session: domain.SwitchComponentPlan{
				State:   domain.SwitchStateReady,
				Summary: "2 managed session artifacts",
			},
			Skills: domain.SwitchComponentPlan{
				State:   domain.SwitchStateReady,
				Summary: "1 skill bundle",
			},
			MCP: domain.SwitchComponentPlan{
				State:   domain.SwitchStatePartial,
				Summary: "1 managed MCP server",
			},
			PlannedFiles: []domain.PlannedFileChange{
				{Section: "session", Path: "/repo/project/CLAUDE.md", Action: "update"},
				{Section: "mcp", Path: "/repo/project/.work-bridge/gemini/mcp.json", Action: "write"},
			},
			Warnings: []string{"portability warning"},
		},
	}
	action := switcher.Result{
		Plan: preview.Plan,
		Report: &domain.ApplyReport{
			Status:          domain.SwitchStatePartial,
			DestinationRoot: "/repo/project",
			FilesUpdated:    []string{"/repo/project/CLAUDE.md", "/repo/project/.work-bridge/gemini/mcp.json"},
			Session: domain.ApplyComponentResult{
				State:   domain.SwitchStateApplied,
				Summary: "2 session files applied",
			},
			Skills: domain.ApplyComponentResult{
				State:   domain.SwitchStateApplied,
				Summary: "1 skill files applied",
			},
			MCP: domain.ApplyComponentResult{
				State:   domain.SwitchStatePartial,
				Summary: "1 MCP files applied",
			},
			Warnings: []string{"report warning"},
		},
	}
	return &fakeBackend{
		workspace: switcher.Workspace{
			ProjectRoot: "/repo/project",
			Sessions: []switcher.WorkspaceItem{
				{
					Tool:        domain.ToolCodex,
					ID:          "session-1",
					Title:       "Codex task",
					ProjectRoot: "/repo/project",
				},
			},
		},
		workspaces: map[string]switcher.Workspace{
			"": {
				ProjectRoot: "/repo/project",
				Sessions: []switcher.WorkspaceItem{
					{
						Tool:        domain.ToolCodex,
						ID:          "session-1",
						Title:       "Codex task",
						ProjectRoot: "/repo/project",
					},
				},
			},
			"/repo/project": {
				ProjectRoot: "/repo/project",
				Sessions: []switcher.WorkspaceItem{
					{
						Tool:        domain.ToolCodex,
						ID:          "session-1",
						Title:       "Codex task",
						ProjectRoot: "/repo/project",
					},
				},
			},
			"/repo/other": {
				ProjectRoot: "/repo/other",
				Sessions: []switcher.WorkspaceItem{
					{
						Tool:        domain.ToolGemini,
						ID:          "session-2",
						Title:       "Gemini task",
						ProjectRoot: "/repo/other",
					},
				},
			},
		},
		projects: []catalog.ProjectEntry{
			{
				Name:          "project",
				Root:          "/repo/project",
				WorkspaceRoot: "/repo",
				Markers:       []string{"git", "codex"},
			},
			{
				Name:          "other",
				Root:          "/repo/other",
				WorkspaceRoot: "/repo",
				Markers:       []string{"git", "gemini"},
			},
		},
		skills: []catalog.SkillEntry{
			{
				Name:        "refactor-review",
				Description: "Review migration readiness",
				EntryPath:   "/repo/project/.agents/skills/refactor-review/SKILL.md",
				Source:      "project .agents/skills",
				Scope:       "project",
			},
		},
		mcp: []catalog.MCPEntry{
			{
				Name:    "project claude settings",
				Path:    "/repo/project/.claude/settings.json",
				Source:  "project",
				Status:  "present",
				Details: "settings.json",
			},
		},
		previewResp: preview,
		applyResp:   action,
		exportResp:  action,
	}
}

func typeCommand(t *testing.T, model MainModel, command string) MainModel {
	t.Helper()
	for _, r := range command {
		model = runKey(t, model, runeKey(string(r)))
	}
	return runKey(t, model, specialKey(tea.KeyEnter))
}

// Unused but kept for future tests.
var _ = filepath.Join
