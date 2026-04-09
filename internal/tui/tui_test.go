package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"sessionport/internal/detect"
	"sessionport/internal/domain"
	"sessionport/internal/inspect"
)

func TestModelTransitionsThroughWorkspaceActions(t *testing.T) {
	t.Parallel()

	var exportedTarget domain.Tool
	var exportedOutDir string

	backend := Backend{
		Detect: func(context.Context) (detect.Report, error) {
			return detect.Report{
				CWD:         "/workspace/repo",
				ProjectRoot: "/workspace/repo",
				Tools: []detect.ToolReport{
					{Tool: "codex", Installed: true},
					{Tool: "gemini", Installed: false},
					{Tool: "claude", Installed: false},
					{Tool: "opencode", Installed: false},
				},
			}, nil
		},
		Inspect: func(context.Context, string) (inspect.Report, error) {
			return inspect.Report{
				Tool: "codex",
				Sessions: []inspect.Session{
					{ID: "session-1", Title: "Write portability layer", StoragePath: "/workspace/session-1.json"},
				},
			}, nil
		},
		Import: func(context.Context, string, string) (domain.SessionBundle, error) {
			bundle := domain.NewSessionBundle(domain.ToolCodex, "/workspace/repo")
			bundle.SourceSessionID = "session-1"
			bundle.BundleID = "bundle-session-1"
			bundle.TaskTitle = "Write portability layer"
			bundle.CurrentGoal = "Write portability layer"
			bundle.Summary = "Portability test bundle"
			return bundle, nil
		},
		Doctor: func(_ context.Context, _ domain.SessionBundle, target domain.Tool) (domain.CompatibilityReport, error) {
			return domain.CompatibilityReport{
				TargetTool:        target,
				CompatibleFields:  []string{"task_title"},
				PartialFields:     []string{"settings_snapshot"},
				UnsupportedFields: []string{"vendor_specific_options"},
			}, nil
		},
		Export: func(_ context.Context, _ domain.SessionBundle, target domain.Tool, _ string) (domain.ExportManifest, error) {
			exportedTarget = target
			exportedOutDir = "/tmp/sessionport-export"
			return domain.ExportManifest{
				TargetTool: target,
				OutputDir:  exportedOutDir,
				Files:      []string{"CLAUDE.sessionport.md", "manifest.json"},
			}, nil
		},
		ScanSkills: func(context.Context) ([]SkillEntry, error) {
			return []SkillEntry{{Name: "frontend-design", Description: "Design frontend flows"}}, nil
		},
		ScanMCP: func(context.Context) ([]MCPEntry, error) {
			return []MCPEntry{{Name: "claude settings", Status: "present"}}, nil
		},
		DefaultExportDir: "/tmp/sessionport-export",
	}

	m := NewModel(context.Background(), backend)

	if cmd := m.loadSnapshotCmd(); cmd != nil {
		msg := cmd()
		var next tea.Cmd
		updated, next := m.Update(msg)
		m = updated.(Model)
		if next != nil {
			msg = next()
			updated, _ = m.Update(msg)
			m = updated.(Model)
		}
	}

	if got := m.activeTool; got != "codex" {
		t.Fatalf("expected active tool codex, got %q", got)
	}
	if got := m.currentInspect().Tool; got != "codex" {
		t.Fatalf("expected inspect tool codex, got %q", got)
	}

	m.activeView = viewSessions
	if cmd := m.importSelectedCmd(); cmd != nil {
		msg := cmd()
		var next tea.Cmd
		updated, next := m.Update(msg)
		m = updated.(Model)
		if next != nil {
			msg = next()
			updated, _ = m.Update(msg)
			m = updated.(Model)
		}
	}

	if m.bundle == nil {
		t.Fatalf("expected bundle to be imported")
	}
	if m.doctorReport == nil {
		t.Fatalf("expected doctor report to be populated")
	}

	m.targetIdx = 3
	if cmd := m.exportSelectedCmd(); cmd != nil {
		msg := cmd()
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}

	if m.exportManifest == nil {
		t.Fatalf("expected export manifest to be populated")
	}
	if exportedTarget != domain.ToolOpenCode {
		t.Fatalf("expected export target to be recorded, got %q", exportedTarget)
	}
	if exportedOutDir == "" {
		t.Fatalf("expected export output dir to be recorded")
	}

	view := m.View()
	if !view.AltScreen {
		t.Fatalf("expected alt screen to be enabled")
	}
	if view.WindowTitle != "sessionport" {
		t.Fatalf("expected window title to be set, got %q", view.WindowTitle)
	}
}
