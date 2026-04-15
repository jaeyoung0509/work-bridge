package actionmenu

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jaeyoung0509/work-bridge/internal/catalog"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/browser"
)

func TestSkillActionMenuPrefersInstallTargets(t *testing.T) {
	t.Parallel()

	model := New(browser.Entry{
		Title: "gemini-helper",
		Badge: "gemini",
		Raw:   catalog.SkillEntry{Name: "gemini-helper", Tool: "gemini"},
	})

	if len(model.options) < 2 {
		t.Fatalf("expected install options plus edit, got %#v", model.options)
	}
	if model.options[0].action != ActionMigrate || model.options[0].target != domain.ToolClaude {
		t.Fatalf("expected first action to install into Claude, got %#v", model.options[0])
	}

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated
	if cmd == nil {
		t.Fatal("expected enter to emit an action")
	}
	msg := cmd()
	selected, ok := msg.(ActionSelectedMsg)
	if !ok {
		t.Fatalf("expected ActionSelectedMsg, got %T", msg)
	}
	if selected.ActionType != ActionMigrate || selected.Target != domain.ToolClaude {
		t.Fatalf("expected migrate-to-Claude action, got %#v", selected)
	}
	_ = model
}

func TestMCPActionMenuWithoutServersFallsBackToEdit(t *testing.T) {
	t.Parallel()

	model := New(browser.Entry{
		Title:       "Global config",
		Description: "No importable MCP servers detected yet",
		Badge:       "codex",
		Raw:         catalog.MCPEntry{Name: "global codex config", Path: "/tmp/config.toml", Tool: "codex"},
	})
	model.SetSize(100, 30)

	if len(model.options) != 1 {
		t.Fatalf("expected edit-only option for MCP config without servers, got %#v", model.options)
	}
	if model.options[0].action != ActionEdit {
		t.Fatalf("expected edit-only action, got %#v", model.options[0])
	}
	view := model.View().Content
	if !strings.Contains(view, "No importable MCP servers were detected in this config yet") {
		t.Fatalf("expected explanatory MCP hint, got %q", view)
	}

	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to emit an edit action")
	}
	msg := cmd()
	selected, ok := msg.(ActionSelectedMsg)
	if !ok {
		t.Fatalf("expected ActionSelectedMsg, got %T", msg)
	}
	if selected.ActionType != ActionEdit {
		t.Fatalf("expected edit action, got %#v", selected)
	}
}
