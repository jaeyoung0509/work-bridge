package switchui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
)

func TestModelPreviewAndExportFlow(t *testing.T) {
	workspace := switcher.Workspace{
		ProjectRoot: "/workspace/demo",
		Sessions: []switcher.WorkspaceItem{
			{
				Tool:        domain.ToolGemini,
				ID:          "session-1",
				Title:       "Gemini session",
				ProjectRoot: "/workspace/demo",
			},
		},
	}
	result := switcher.Result{
		Payload: domain.SwitchPayload{
			Bundle: domain.SessionBundle{
				SourceTool:      domain.ToolGemini,
				SourceSessionID: "session-1",
			},
		},
		Plan: domain.SwitchPlan{
			TargetTool:  domain.ToolClaude,
			ProjectRoot: "/workspace/demo/.work-bridge/exports/claude",
			ManagedRoot: "/workspace/demo/.work-bridge/exports/claude/.work-bridge/claude",
			Status:      domain.SwitchStateReady,
			Session:     domain.SwitchComponentPlan{State: domain.SwitchStateReady},
			Skills:      domain.SwitchComponentPlan{State: domain.SwitchStateReady},
			MCP:         domain.SwitchComponentPlan{State: domain.SwitchStatePartial},
		},
		Report: &domain.ApplyReport{
			AppliedMode:  "export_only",
			Status:       domain.SwitchStatePartial,
			FilesUpdated: []string{"/workspace/demo/.work-bridge/exports/claude/CLAUDE.md"},
		},
	}

	model := NewModel(context.Background(), Backend{
		LoadWorkspace: func(context.Context) (switcher.Workspace, error) { return workspace, nil },
		Preview: func(context.Context, switcher.Request) (switcher.Result, error) {
			return result, nil
		},
		Export: func(_ context.Context, req switcher.Request, out string) (switcher.Result, error) {
			if req.From != domain.ToolGemini || req.To != domain.ToolCodex {
				t.Fatalf("unexpected request: %#v", req)
			}
			if out != filepath.Join("/workspace/demo", ".work-bridge", "exports", "codex") {
				t.Fatalf("unexpected export root %q", out)
			}
			exported := result
			exported.Plan.TargetTool = domain.ToolCodex
			return exported, nil
		},
	})

	updated, cmd := model.Update(workspaceLoadedMsg{workspace: workspace})
	if cmd != nil {
		t.Fatalf("expected no command on workspace load")
	}
	model = updated.(Model)
	model.targetIdx = 0

	updated, cmd = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd == nil {
		t.Fatalf("expected export command")
	}
	msg := cmd()
	exportMsg, ok := msg.(exportReadyMsg)
	if !ok {
		t.Fatalf("expected exportReadyMsg, got %#v", msg)
	}
	if exportMsg.err != nil {
		t.Fatalf("unexpected export error: %v", exportMsg.err)
	}

	updated, _ = updated.(Model).Update(exportMsg)
	model = updated.(Model)
	if model.result == nil || model.result.Report == nil {
		t.Fatalf("expected export result to be stored")
	}
	view := model.View().Content
	if !strings.Contains(view, "mode: export_only") {
		t.Fatalf("expected export mode in view, got %q", view)
	}
}

func TestModelPreviewUsesSelectedSessionAndProjectScope(t *testing.T) {
	model := NewModel(context.Background(), Backend{})
	model.workspace = switcher.Workspace{
		ProjectRoot: "/workspace/demo",
		Sessions: []switcher.WorkspaceItem{
			{Tool: domain.ToolCodex, ID: "a"},
			{Tool: domain.ToolClaude, ID: "b"},
		},
	}
	model.sessionIdx = 1
	model.targetIdx = 2

	req := model.currentRequest()
	if req.From != domain.ToolClaude {
		t.Fatalf("expected selected source tool, got %q", req.From)
	}
	if req.Session != "b" {
		t.Fatalf("expected selected session id, got %q", req.Session)
	}
	if req.ProjectRoot != "/workspace/demo" {
		t.Fatalf("expected workspace project root, got %q", req.ProjectRoot)
	}
	if req.To != domain.ToolClaude {
		t.Fatalf("expected target index to map to claude, got %q", req.To)
	}
}
