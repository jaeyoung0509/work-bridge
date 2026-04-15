package cmdpalette

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestEnterExecutesExactCommand(t *testing.T) {
	t.Parallel()

	model := New(DefaultCommands()).Open("/skills")
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated
	if model.Active() {
		t.Fatal("expected palette to close after executing a command")
	}
	if cmd == nil {
		t.Fatal("expected enter to emit an exec message")
	}
	msg := cmd()
	execMsg, ok := msg.(ExecMsg)
	if !ok {
		t.Fatalf("expected ExecMsg, got %T", msg)
	}
	if execMsg.Command != "/skills" {
		t.Fatalf("expected /skills command, got %q", execMsg.Command)
	}
}

func TestArrowSelectionExecutesHighlightedSuggestion(t *testing.T) {
	t.Parallel()

	model := New(DefaultCommands()).Open("/")
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated
	if model.Active() {
		t.Fatal("expected palette to close after selection")
	}
	if cmd == nil {
		t.Fatal("expected selection to emit exec message")
	}
	msg := cmd()
	execMsg, ok := msg.(ExecMsg)
	if !ok {
		t.Fatalf("expected ExecMsg, got %T", msg)
	}
	if execMsg.Command != "/sessions" {
		t.Fatalf("expected /sessions after moving down once, got %q", execMsg.Command)
	}
}

func TestEscapeCancelsPalette(t *testing.T) {
	t.Parallel()

	model := New(DefaultCommands()).Open("/")
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated
	if model.Active() {
		t.Fatal("expected palette to close on escape")
	}
	if cmd == nil {
		t.Fatal("expected escape to emit cancel message")
	}
	if _, ok := cmd().(CancelMsg); !ok {
		t.Fatalf("expected CancelMsg, got %T", cmd())
	}
}

func TestTabCompletesMatchingSuggestion(t *testing.T) {
	t.Parallel()

	model := New(DefaultCommands()).Open("/m")
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	model = updated
	if model.Input() != "/mcp" {
		t.Fatalf("expected tab completion to pick /mcp, got %q", model.Input())
	}
}
