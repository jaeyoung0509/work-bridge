package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/session"
)

type AppState int

const (
	StateSelectSession AppState = iota
	// StatePreview
	// StateApplying
)

type MainModel struct {
	state       AppState
	ctx         context.Context
	backend     *switcher.Service
	sessionView session.Model
	quitting    bool
}

type workspaceLoadedMsg struct {
	workspace switcher.Workspace
	err       error
}

func NewMainModel(ctx context.Context, backend *switcher.Service) MainModel {
	return MainModel{
		state:       StateSelectSession,
		ctx:         ctx,
		backend:     backend,
		sessionView: session.NewModel(),
	}
}

func (m MainModel) Init() tea.Cmd {
	return m.loadWorkspaceCmd()
}

func (m MainModel) loadWorkspaceCmd() tea.Cmd {
	return func() tea.Msg {
		ws, err := m.backend.LoadWorkspace(m.ctx)
		return workspaceLoadedMsg{workspace: ws, err: err}
	}
}

func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}

	case workspaceLoadedMsg:
		if msg.err == nil {
			m.sessionView.SetSessions(msg.workspace.Sessions)
		}
		return m, nil

	case session.SessionSelectedMsg:
		// Transition to Preview View later
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	switch m.state {
	case StateSelectSession:
		var updated tea.Model
		updated, cmd = m.sessionView.Update(msg)
		m.sessionView = updated.(session.Model)
	}

	return m, cmd
}

func (m MainModel) View() tea.View {
	if m.quitting {
		return tea.NewView("Goodbye!\n")
	}

	var content string
	switch m.state {
	case StateSelectSession:
		content = m.sessionView.View().Content
	}

	view := tea.NewView(styles.AppContainer.Render(content))
	view.AltScreen = true
	return view
}
