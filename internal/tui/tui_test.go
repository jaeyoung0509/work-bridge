package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jaeyoung0509/work-bridge/internal/detect"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/inspect"
)

func TestWorkspaceActionsCoverSessionSkillAndMCPFlows(t *testing.T) {
	t.Parallel()

	var exportedTarget domain.Tool
	var exportedOutDir string

	backend := Backend{
		LoadWorkspaceSnapshot: func(context.Context) (WorkspaceSnapshot, error) {
			return WorkspaceSnapshot{
				Detect: detect.Report{
					CWD:         "/workspace/repo",
					ProjectRoot: "/workspace/repo",
					Tools: []detect.ToolReport{
						{Tool: "codex", Installed: true},
						{Tool: "claude", Installed: true},
					},
				},
				InspectByTool: map[domain.Tool]inspect.Report{
					domain.ToolCodex: {
						Tool: "codex",
						Sessions: []inspect.Session{
							{ID: "session-1", Title: "Write portability layer", ProjectRoot: "/workspace/repo", StoragePath: "/workspace/session-1.json"},
						},
						Notes: []string{"session inventory loaded"},
					},
				},
				Projects: []ProjectEntry{
					{Name: "repo", Root: "/workspace/repo", WorkspaceRoot: "/workspace", Markers: []string{"git", "codex"}, SessionCount: 1},
				},
				Skills: []SkillEntry{
					{Name: "frontend-design", Description: "Design frontend flows", Path: "/skills/frontend-design/SKILL.md", Source: "codex user", Scope: "user", Tool: domain.ToolCodex, Content: "# frontend-design"},
				},
				MCPProfiles: []MCPEntry{
					{Name: "claude settings", Path: "/configs/claude/settings.json", Status: "configured", Details: "1 declared server(s)", Tool: domain.ToolClaude, DeclaredServers: 1, RawConfig: `{"mcpServers":{"github":{}}}`},
				},
				HealthSummary: WorkspaceHealthSummary{
					InstalledTools: 2,
					ProjectCount:   1,
					SessionCount:   1,
					SkillCount:     1,
					MCPCount:       1,
				},
			}, nil
		},
		ImportSession: func(context.Context, domain.Tool, string) (domain.SessionBundle, error) {
			bundle := domain.NewSessionBundle(domain.ToolCodex, "/workspace/repo")
			bundle.SourceSessionID = "session-1"
			bundle.BundleID = "bundle-session-1"
			bundle.TaskTitle = "Write portability layer"
			bundle.CurrentGoal = "Write portability layer"
			bundle.Summary = "Portability test bundle"
			return bundle, nil
		},
		DoctorBundle: func(_ context.Context, _ domain.SessionBundle, target domain.Tool) (domain.CompatibilityReport, error) {
			return domain.CompatibilityReport{
				TargetTool:        target,
				CompatibleFields:  []string{"task_title"},
				PartialFields:     []string{"settings_snapshot"},
				UnsupportedFields: []string{"vendor_specific_options"},
			}, nil
		},
		ExportBundle: func(_ context.Context, _ domain.SessionBundle, target domain.Tool, outDir string) (domain.ExportManifest, error) {
			exportedTarget = target
			exportedOutDir = outDir
			return domain.ExportManifest{
				TargetTool: target,
				OutputDir:  outDir,
				Files:      []string{"CLAUDE.work-bridge.md", "manifest.json"},
			}, nil
		},
		InstallSkill: func(_ context.Context, _ SkillEntry, target SkillTarget) (SkillInstallResult, error) {
			return SkillInstallResult{InstalledPath: target.Path, TargetID: target.ID, TargetLabel: target.Label, TargetScope: target.Scope}, nil
		},
		ProbeMCP: func(_ context.Context, _ MCPEntry) (MCPProbeResult, error) {
			return MCPProbeResult{Reachable: true, ResourceCount: 1, Warnings: []string{"config-level only"}}, nil
		},
		DefaultExportDir: "/tmp/work-bridge-export",
	}

	m := NewModel(context.Background(), backend)

	if cmd := m.loadWorkspaceCmd(); cmd != nil {
		updated, _ := m.Update(cmd())
		m = updated.(Model)
	}

	if got := len(m.filteredSessions()); got != 1 {
		t.Fatalf("expected 1 session, got %d", got)
	}

	if cmd := m.importSelectedCmd(); cmd != nil {
		msg := cmd().(bundleImportedMsg)
		msg.autoDoctor = false
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}

	sessionKey := sessionKeyFor(domain.ToolCodex, "session-1")
	if _, ok := m.bundleBySession[sessionKey]; !ok {
		t.Fatalf("expected imported bundle cache")
	}

	if cmd := m.doctorSelectedCmd(); cmd != nil {
		updated, _ := m.Update(cmd())
		m = updated.(Model)
	}

	if _, ok := m.doctorByKey[doctorKey(sessionKey, domain.ToolCodex)]; !ok {
		t.Fatalf("expected doctor cache for current target")
	}

	m.targetIdx = 2
	if cmd := m.exportSelectedCmd(); cmd != nil {
		updated, _ := m.Update(cmd())
		m = updated.(Model)
	}

	if _, ok := m.exportByKey[doctorKey(sessionKey, domain.ToolClaude)]; !ok {
		t.Fatalf("expected export manifest cache for claude target")
	}
	if exportedTarget != domain.ToolClaude {
		t.Fatalf("expected export target claude, got %q", exportedTarget)
	}
	if !strings.Contains(exportedOutDir, "session-1-to-claude") {
		t.Fatalf("expected export output dir to include session target, got %q", exportedOutDir)
	}

	m.activeSection = sectionSkills
	if cmd := m.installSelectedSkillCmd(); cmd != nil {
		updated, _ := m.Update(cmd())
		m = updated.(Model)
	}

	if _, ok := m.installByPath["/skills/frontend-design/SKILL.md"]; !ok {
		t.Fatalf("expected skill install result cache")
	}

	m.activeSection = sectionMCP
	if cmd := m.probeSelectedMCP(); cmd != nil {
		updated, _ := m.Update(cmd())
		m = updated.(Model)
	}

	probeKey := mcpEntryKey(MCPEntry{Name: "claude settings", Path: "/configs/claude/settings.json", Tool: domain.ToolClaude})
	if probe, ok := m.probeByID[probeKey]; !ok || !probe.Reachable {
		t.Fatalf("expected successful mcp probe, got %#v", probe)
	}
}

func TestViewRendersAcrossResponsiveModes(t *testing.T) {
	t.Parallel()

	m := NewModel(context.Background(), Backend{})
	m.snapshot = WorkspaceSnapshot{
		Detect: detect.Report{CWD: "/workspace/repo", ProjectRoot: "/workspace/repo"},
		InspectByTool: map[domain.Tool]inspect.Report{
			domain.ToolCodex: {
				Tool: "codex",
				Sessions: []inspect.Session{
					{ID: "session-1", Title: "Ship UI", ProjectRoot: "/workspace/repo"},
				},
			},
		},
		Projects:    []ProjectEntry{{Name: "repo", Root: "/workspace/repo", WorkspaceRoot: "/workspace", Markers: []string{"git"}}},
		Skills:      []SkillEntry{{Name: "skill-one", Description: "desc", Path: "/skills/skill-one/SKILL.md", Scope: "project"}},
		MCPProfiles: []MCPEntry{{Name: "codex config", Path: "/configs/config.toml", Status: "configured", Tool: domain.ToolCodex}},
		HealthSummary: WorkspaceHealthSummary{
			InstalledTools: 1,
			ProjectCount:   1,
			SessionCount:   1,
			SkillCount:     1,
			MCPCount:       1,
		},
	}

	widths := []int{130, 100, 80}
	for _, width := range widths {
		m.width = width
		m.height = 32
		view := m.View()
		if !view.AltScreen {
			t.Fatalf("expected alt screen for width %d", width)
		}
		if view.WindowTitle != "work-bridge" {
			t.Fatalf("expected window title for width %d, got %q", width, view.WindowTitle)
		}
		if !strings.Contains(view.Content, "work-bridge") {
			t.Fatalf("expected rendered content for width %d", width)
		}
	}
}

func TestMouseClickUsesRenderedHitboxesInCompactNav(t *testing.T) {
	t.Parallel()

	m := NewModel(context.Background(), Backend{})
	m.width = 80
	m.height = 24
	m.snapshot = WorkspaceSnapshot{
		Detect: detect.Report{CWD: "/workspace/repo", ProjectRoot: "/workspace/repo"},
		InspectByTool: map[domain.Tool]inspect.Report{
			domain.ToolCodex: {Tool: "codex", Sessions: []inspect.Session{{ID: "session-1", Title: "Ship UI"}}},
		},
		Skills:      []SkillEntry{{Name: "skill-one", Description: "desc", Path: "/skills/skill-one/SKILL.md", Scope: "project"}},
		MCPProfiles: []MCPEntry{{Name: "claude settings", Path: "/configs/claude/settings.json", Status: "parsed", Tool: domain.ToolClaude}},
	}

	layout := m.currentLayout()
	firstWidth := lipgloss.Width(badgeStyle(m.navLabel(sectionSessions), "accent"))
	x := layout.nav.X + 2 + firstWidth + 1
	y := layout.nav.Y + 2

	updated, _ := m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	m = updated.(Model)

	if m.activeSection != sectionProjects {
		t.Fatalf("expected compact nav click to switch to projects, got %v", m.activeSection)
	}
	if m.focus != focusNav {
		t.Fatalf("expected nav focus after compact nav click, got %v", m.focus)
	}
}

func TestMouseClickAndWheelDrivePreviewInteraction(t *testing.T) {
	t.Parallel()

	contentLines := make([]string, 0, 24)
	for i := 0; i < 24; i++ {
		contentLines = append(contentLines, "line "+strings.Repeat("x", i%5+1))
	}

	m := NewModel(context.Background(), Backend{})
	m.width = 130
	m.height = 28
	m.activeSection = sectionSkills
	m.snapshot = WorkspaceSnapshot{
		Detect: detect.Report{CWD: "/workspace/repo", ProjectRoot: "/workspace/repo"},
		Skills: []SkillEntry{
			{Name: "alpha", Description: "desc", Path: "/skills/alpha/SKILL.md", Scope: "project", Content: strings.Join(contentLines, "\n")},
			{Name: "beta", Description: "desc", Path: "/skills/beta/SKILL.md", Scope: "project", Content: strings.Join(contentLines, "\n")},
		},
	}
	m.skillTabIdx = 1

	layout := m.currentLayout()
	listClickX := layout.list.X + 3
	listClickY := layout.list.Y + 4

	updated, _ := m.Update(tea.MouseClickMsg(tea.Mouse{X: listClickX, Y: listClickY, Button: tea.MouseLeft}))
	m = updated.(Model)
	if m.skillIdx != 1 {
		t.Fatalf("expected list click to select second skill, got %d", m.skillIdx)
	}
	if m.focus != focusList {
		t.Fatalf("expected list focus after list click, got %v", m.focus)
	}

	updated, _ = m.Update(tea.MouseWheelMsg(tea.Mouse{X: layout.preview.X + 3, Y: layout.preview.Y + 4, Button: tea.MouseWheelDown}))
	m = updated.(Model)
	if m.skillPreviewOffset <= 0 {
		t.Fatalf("expected preview wheel to advance preview offset, got %d", m.skillPreviewOffset)
	}

	m.width = 80
	m.height = 24
	m.activeSection = sectionMCP
	m.focus = focusPreview
	m.snapshot.MCPProfiles = []MCPEntry{
		{Name: "claude settings", Path: "/configs/claude/settings.json", Status: "parsed", Tool: domain.ToolClaude, RawConfig: `{"mcpServers":{"github":{}}}`},
	}
	m.mcpTabIdx = 0

	layout = m.currentLayout()
	summaryWidth := lipgloss.Width(badgeStyle("[Summary]", "accent"))
	tabX := layout.preview.X + 2 + summaryWidth + 1
	tabY := layout.preview.Y + 2

	updated, _ = m.Update(tea.MouseClickMsg(tea.Mouse{X: tabX, Y: tabY, Button: tea.MouseLeft}))
	m = updated.(Model)

	if m.currentPreviewTab() != previewRaw {
		t.Fatalf("expected preview tab click to switch to raw, got %q", m.currentPreviewTab())
	}
	if m.focus != focusPreview {
		t.Fatalf("expected preview focus after preview tab click, got %v", m.focus)
	}
}

func TestProjectSelectionScopesWorkspaceAndCanBeCleared(t *testing.T) {
	t.Parallel()

	m := NewModel(context.Background(), Backend{})
	m.width = 130
	m.height = 28
	m.activeSection = sectionProjects
	m.snapshot = WorkspaceSnapshot{
		Detect: detect.Report{CWD: "/workspace/repo", ProjectRoot: "/workspace/repo"},
		InspectByTool: map[domain.Tool]inspect.Report{
			domain.ToolCodex: {
				Tool: "codex",
				Sessions: []inspect.Session{
					{ID: "repo-root", Title: "Root session", ProjectRoot: "/workspace/repo"},
					{ID: "repo-nested", Title: "Nested session", ProjectRoot: "/workspace/repo/service"},
					{ID: "other", Title: "Other session", ProjectRoot: "/workspace/other"},
				},
			},
		},
		Projects: []ProjectEntry{
			{Name: "other", Root: "/workspace/other", WorkspaceRoot: "/workspace", SessionCount: 1, SessionByTool: map[string]int{"codex": 1}},
			{Name: "repo", Root: "/workspace/repo", WorkspaceRoot: "/workspace", SessionCount: 1, SkillCount: 1, MCPCount: 1, SessionByTool: map[string]int{"codex": 1}},
			{Name: "service", Root: "/workspace/repo/service", WorkspaceRoot: "/workspace", SessionCount: 1, SkillCount: 1, MCPCount: 1, SessionByTool: map[string]int{"codex": 1}},
		},
		Skills: []SkillEntry{
			{Name: "repo-skill", Path: "/workspace/repo/skills/root/SKILL.md", Scope: "project"},
			{Name: "service-skill", Path: "/workspace/repo/service/skills/nested/SKILL.md", Scope: "project"},
			{Name: "other-skill", Path: "/workspace/other/skills/other/SKILL.md", Scope: "project"},
		},
		MCPProfiles: []MCPEntry{
			{Name: "repo mcp", Path: "/workspace/repo/.claude/settings.json"},
			{Name: "service mcp", Path: "/workspace/repo/service/.claude/settings.json"},
			{Name: "other mcp", Path: "/workspace/other/.claude/settings.json"},
		},
	}

	if got := len(m.filteredSessions()); got != 3 {
		t.Fatalf("expected unscoped sessions, got %d", got)
	}

	m.setProjectIndex(1)

	if !samePath(m.activeProjectRoot, "/workspace/repo") {
		t.Fatalf("expected active project root /workspace/repo, got %q", m.activeProjectRoot)
	}
	if got := len(m.filteredSessions()); got != 1 {
		t.Fatalf("expected only root project session, got %d", got)
	}
	if got := len(m.filteredSkills()); got != 1 {
		t.Fatalf("expected only root project skill, got %d", got)
	}
	if got := len(m.filteredMCP()); got != 1 {
		t.Fatalf("expected only root project mcp, got %d", got)
	}

	preview := m.renderProjectPreview()
	for _, want := range []string{"Scope: active", "Sessions by Tool", "- codex: 1"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("expected project preview to contain %q, got %q", want, preview)
		}
	}

	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c'}))
	m = updated.(Model)

	if m.activeProjectRoot != "" {
		t.Fatalf("expected cleared active project root, got %q", m.activeProjectRoot)
	}
	if got := len(m.filteredSessions()); got != 3 {
		t.Fatalf("expected restored session view after clear, got %d", got)
	}
}

func TestMouseClickOnActiveProjectClearsScope(t *testing.T) {
	t.Parallel()

	m := NewModel(context.Background(), Backend{})
	m.width = 120
	m.height = 28
	m.activeSection = sectionProjects
	m.snapshot = WorkspaceSnapshot{
		Projects: []ProjectEntry{
			{Name: "repo", Root: "/workspace/repo", WorkspaceRoot: "/workspace"},
			{Name: "other", Root: "/workspace/other", WorkspaceRoot: "/workspace"},
		},
		InspectByTool: map[domain.Tool]inspect.Report{
			domain.ToolCodex: {Tool: "codex", Sessions: []inspect.Session{{ID: "repo", ProjectRoot: "/workspace/repo"}, {ID: "other", ProjectRoot: "/workspace/other"}}},
		},
	}
	m.setProjectIndex(1)

	layout := m.currentLayout()
	clickX := layout.list.X + 3
	clickY := layout.list.Y + 4

	updated, _ := m.Update(tea.MouseClickMsg(tea.Mouse{X: clickX, Y: clickY, Button: tea.MouseLeft}))
	m = updated.(Model)

	if m.activeProjectRoot != "" {
		t.Fatalf("expected active project to clear on repeated click, got %q", m.activeProjectRoot)
	}
	if got := len(m.filteredSessions()); got != 2 {
		t.Fatalf("expected all sessions after clear, got %d", got)
	}
}

func TestActiveProjectRecomputesEffectiveMCPDeclaration(t *testing.T) {
	t.Parallel()

	m := NewModel(context.Background(), Backend{})
	m.snapshot = WorkspaceSnapshot{
		Projects: []ProjectEntry{
			{Name: "repo", Root: "/workspace/repo", WorkspaceRoot: "/workspace"},
			{Name: "other", Root: "/workspace/other", WorkspaceRoot: "/workspace"},
		},
		MCPProfiles: []MCPEntry{
			{
				ID:   "claude|server|github",
				Kind: "server",
				Name: "github",
				Tool: domain.ToolClaude,
				Declarations: []MCPDeclaration{
					{
						Label:       "global claude settings",
						Path:        "/home/me/.claude/settings.json",
						Source:      "user",
						Scope:       "user",
						Status:      "parsed",
						RawConfig:   `{"mcpServers":{"github":{"command":"mcp-github-user"}}}`,
						BinaryFound: true,
						BinaryPath:  "/opt/bin/claude",
						Server: MCPServerConfig{
							Name:      "github",
							Transport: "stdio",
							Command:   "mcp-github-user",
						},
					},
					{
						Label:       "project claude settings",
						Path:        "/workspace/other/.claude/settings.json",
						Source:      "project",
						Scope:       "project",
						Status:      "parsed",
						RawConfig:   `{"mcpServers":{"github":{"command":"mcp-github-other"}}}`,
						BinaryFound: true,
						BinaryPath:  "/opt/bin/claude",
						Server: MCPServerConfig{
							Name:      "github",
							Transport: "stdio",
							Command:   "mcp-github-other",
						},
					},
				},
			},
		},
	}

	m.activeProjectRoot = "/workspace/repo"
	items := m.filteredMCP()
	if len(items) != 1 {
		t.Fatalf("expected github entry to remain visible via user scope, got %d", len(items))
	}
	if items[0].Scope != "user" || items[0].Path != "/home/me/.claude/settings.json" {
		t.Fatalf("expected user declaration to be effective for repo project, got %#v", items[0])
	}

	m.activeProjectRoot = "/workspace/other"
	items = m.filteredMCP()
	if len(items) != 1 {
		t.Fatalf("expected github entry for other project, got %d", len(items))
	}
	if items[0].Scope != "project" || items[0].Path != "/workspace/other/.claude/settings.json" {
		t.Fatalf("expected project declaration to win for other project, got %#v", items[0])
	}

	raw := m.renderMCPPreview()
	for _, want := range []string{"Effective scope: project", "Hidden / Overridden", "global claude settings"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected MCP preview to contain %q, got %q", want, raw)
		}
	}
}

func TestSkillSyncTargetsPreferProjectAndCanCycleToExternalScopes(t *testing.T) {
	t.Parallel()

	var syncedTarget SkillTarget

	backend := Backend{
		InstallSkill: func(_ context.Context, _ SkillEntry, target SkillTarget) (SkillInstallResult, error) {
			syncedTarget = target
			return SkillInstallResult{
				InstalledPath: target.Path,
				TargetID:      target.ID,
				TargetLabel:   target.Label,
				TargetScope:   target.Scope,
			}, nil
		},
	}

	m := NewModel(context.Background(), backend)
	m.activeSection = sectionSkills
	m.snapshot = WorkspaceSnapshot{
		HomeDir: "/home/me",
		Detect:  detect.Report{ProjectRoot: "/workspace/repo"},
		Skills: []SkillEntry{
			{
				Name:          "frontend-design",
				Path:          "/home/me/.codex/skills/frontend-design/SKILL.md",
				Scope:         "user",
				Tool:          domain.ToolCodex,
				Source:        "codex user",
				ConflictState: "only-in-user/global",
				VariantCount:  1,
			},
			{
				Name:          "lint-helper",
				Path:          "/workspace/repo/skills/lint-helper/SKILL.md",
				Scope:         "project",
				Source:        "project skills",
				ConflictState: "only-in-project",
				VariantCount:  1,
			},
		},
	}

	item, ok := m.selectedSkill()
	if !ok {
		t.Fatalf("expected selected skill")
	}
	target, ok := m.selectedSkillTarget(item)
	if !ok {
		t.Fatalf("expected default target")
	}
	if target.Scope != "project" || target.Path != "/workspace/repo/skills/frontend-design/SKILL.md" {
		t.Fatalf("expected default project sync target, got %#v", target)
	}

	if cmd := m.installSelectedSkillCmd(); cmd != nil {
		updated, _ := m.Update(cmd())
		m = updated.(Model)
	}
	if syncedTarget.Path != "/workspace/repo/skills/frontend-design/SKILL.md" {
		t.Fatalf("expected project sync path, got %#v", syncedTarget)
	}

	m.setSkillIndex(1)
	m.moveSkillTarget(1)
	projectSkill, _ := m.selectedSkill()
	target, ok = m.selectedSkillTarget(projectSkill)
	if !ok {
		t.Fatalf("expected cycled target for project skill")
	}
	if target.Scope != "user" {
		t.Fatalf("expected external user target after cycling, got %#v", target)
	}

	preview := m.renderSkillPreview()
	for _, want := range []string{"Selected Target", "Action: create", "Conflict: only-in-project"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("expected skill preview to contain %q, got %q", want, preview)
		}
	}
}
