package handoff

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
)

func TestSessionOnlyToggleDisablesSkillsAndMCP(t *testing.T) {
	t.Parallel()

	model := New(switcher.WorkspaceItem{Tool: domain.ToolCodex, ID: "session-1", Title: "Codex task"}, "/repo/project", "")
	model = pressKey(t, model, tea.KeyDown)
	model = pressKey(t, model, tea.KeyEnter)
	model = pressKey(t, model, tea.KeyDown)
	model = pressKey(t, model, tea.KeyDown)
	model = pressKey(t, model, tea.KeyEnter)

	if !model.sessionOnly {
		t.Fatal("expected session-only mode to be enabled")
	}
	if model.includeSkills || model.includeMCP {
		t.Fatalf("expected skills and MCP to be disabled in session-only mode, got skills=%v mcp=%v", model.includeSkills, model.includeMCP)
	}
}

func TestExportOverlayEmitsExportRequest(t *testing.T) {
	t.Parallel()

	model := New(switcher.WorkspaceItem{Tool: domain.ToolCodex, ID: "session-1", Title: "Codex task"}, "/repo/project", "/tmp/out")
	model.SetPreview(&switcher.Result{Plan: domain.SwitchPlan{Status: domain.SwitchStateReady}})
	model = pressKey(t, model, tea.KeyDown)
	model = pressKey(t, model, tea.KeyDown)
	model = pressKey(t, model, tea.KeyDown)

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated
	if cmd != nil {
		t.Fatalf("expected opening export overlay to be synchronous, got command %v", cmd)
	}
	if model.overlay != overlayConfirm || model.confirmAction != "export" {
		t.Fatalf("expected export confirm overlay, got overlay=%v action=%q", model.overlay, model.confirmAction)
	}
	if !strings.Contains(model.confirmInput, "/tmp/out") {
		t.Fatalf("expected configured export path in overlay, got %q", model.confirmInput)
	}

	updated, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated
	if cmd == nil {
		t.Fatal("expected export confirmation to emit request")
	}
	msg := cmd()
	exportMsg, ok := msg.(ExportRequestMsg)
	if !ok {
		t.Fatalf("expected ExportRequestMsg, got %T", msg)
	}
	if exportMsg.ExportPath != "/tmp/out" {
		t.Fatalf("expected export path /tmp/out, got %q", exportMsg.ExportPath)
	}
	if model.running != true {
		t.Fatal("expected model to enter running state after export confirmation")
	}
}

func TestApplyWithoutPreviewShowsError(t *testing.T) {
	t.Parallel()

	model := New(switcher.WorkspaceItem{Tool: domain.ToolCodex, ID: "session-1", Title: "Codex task"}, "/repo/project", "")
	model = pressKey(t, model, tea.KeyDown)
	model = pressKey(t, model, tea.KeyDown)
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated
	if cmd != nil {
		t.Fatalf("expected no command when preview is missing, got %v", cmd)
	}
	if model.lastErr == nil || !strings.Contains(model.lastErr.Error(), "preview not ready") {
		t.Fatalf("expected preview-not-ready error, got %v", model.lastErr)
	}
}

func pressKey(t *testing.T, model Model, code rune) Model {
	t.Helper()
	updated, cmd := model.Update(tea.KeyPressMsg{Code: code})
	model = updated
	if cmd != nil {
		msg := cmd()
		t.Fatalf("expected no async command for navigation key %v, got %T", code, msg)
	}
	return model
}
