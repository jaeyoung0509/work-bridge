package browser

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestArrowKeysMoveSelection(t *testing.T) {
	t.Parallel()

	model := NewModel("Projects")
	model.SetSize(80, 20)
	model.SetEntries([]Entry{
		{Key: "alpha", Title: "alpha", Description: "first", Section: "Repo"},
		{Key: "beta", Title: "beta", Description: "second", Section: "Repo"},
	})

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(Model)

	selected, ok := model.SelectedEntry()
	if !ok {
		t.Fatal("expected a selected entry after navigation")
	}
	if selected.Key != "beta" {
		t.Fatalf("expected beta to be selected after down key, got %q", selected.Key)
	}
}

func TestMouseClickSelectsClickedEntry(t *testing.T) {
	t.Parallel()

	model := NewModel("Projects")
	model.SetSize(80, 20)
	model.SetEntries([]Entry{
		{Key: "alpha", Title: "alpha", Description: "first", Section: "Repo"},
		{Key: "beta", Title: "beta", Description: "second", Section: "Repo"},
	})

	updated, cmd := model.Update(tea.MouseClickMsg{X: 0, Y: 9, Button: tea.MouseLeft})
	model = updated.(Model)

	selected, ok := model.SelectedEntry()
	if !ok {
		t.Fatal("expected a selected entry after click")
	}
	if selected.Key != "beta" {
		t.Fatalf("expected beta to be selected after click, got %q", selected.Key)
	}
	if cmd == nil {
		t.Fatal("expected click to emit a selected message")
	}
	msg := cmd()
	selectedMsg, ok := msg.(SelectedMsg)
	if !ok {
		t.Fatalf("expected SelectedMsg, got %T", msg)
	}
	if selectedMsg.Entry.Key != "beta" {
		t.Fatalf("expected SelectedMsg for beta, got %q", selectedMsg.Entry.Key)
	}
}
