package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
	errorview "github.com/jaeyoung0509/work-bridge/internal/ui/views/error"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/loading"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/result"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/session"
)

type AppState int

const (
	StateLoading AppState = iota
	StateSessionList
	StateSessionPreview
	StateApplying
	StateComplete
	StateError
)

type workspaceLoadedMsg struct {
	workspace switcher.Workspace
	err       error
}

// MainModel is the root TUI model.
type MainModel struct {
	state   AppState
	ctx     context.Context
	backend *switcher.Service

	// Sub-models
	loadingModel loading.Model
	sessionModel session.Model
	previewModel session.PreviewModel
	resultModel  result.Model
	errorModel   errorview.Model

	quitting bool
}

func NewMainModel(ctx context.Context, backend *switcher.Service) MainModel {
	return MainModel{
		state:        StateLoading,
		ctx:          ctx,
		backend:      backend,
		loadingModel: loading.NewModel(),
		sessionModel: session.NewModel(),
		previewModel: session.NewPreviewModel(),
		resultModel:  result.NewModel(),
		errorModel:   errorview.NewModel(),
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
		if msg.err != nil {
			m.state = StateError
			m.errorModel.SetError(msg.err, true)
			m.errorModel.SetTitle("Load Failed")
			return m, nil
		}
		if len(msg.workspace.Sessions) == 0 {
			m.state = StateError
			m.errorModel.SetTitle("No Sessions Found")
			m.errorModel.SetError(nil, false)
			return m, nil
		}
		m.sessionModel.SetSessions(msg.workspace.Sessions)
		m.state = StateSessionList
		return m, nil

	case session.SessionSelectedMsg:
		m.previewModel.SetSession(msg.Session)
		m.state = StateSessionPreview
		return m, nil

	case session.GoBackMsg:
		m.state = StateSessionList
		return m, nil

	case session.StartMigrationMsg:
		m.state = StateApplying
		return m, m.applyCmd(msg.Session)

	case result.SelectAnotherMsg:
		m.state = StateSessionList
		return m, nil

	case result.GoBackMsg:
		m.state = StateSessionList
		return m, nil

	case result.RetryMigrationMsg:
		m.state = StateApplying
		return m, m.applyCmd(m.previewModel.Session())

	case result.MigrationCompleteMsg:
		m.state = StateComplete
		return m, nil

	case result.MigrationFailedMsg:
		m.state = StateError
		m.errorModel.SetError(msg.Err, true)
		m.errorModel.SetTitle("Migration Failed")
		return m, nil

	case errorview.RetryMsg:
		m.state = StateLoading
		return m, m.loadWorkspaceCmd()

	case errorview.GoBackMsg:
		m.state = StateSessionList
		return m, nil
	}

	var cmd tea.Cmd
	switch m.state {
	case StateLoading:
		var updated tea.Model
		updated, cmd = m.loadingModel.Update(msg)
		m.loadingModel = updated.(loading.Model)

	case StateSessionList:
		var updated tea.Model
		updated, cmd = m.sessionModel.Update(msg)
		m.sessionModel = updated.(session.Model)

	case StateSessionPreview:
		var updated tea.Model
		updated, cmd = m.previewModel.Update(msg)
		m.previewModel = updated.(session.PreviewModel)

	case StateApplying, StateComplete:
		var updated tea.Model
		updated, cmd = m.resultModel.Update(msg)
		m.resultModel = updated.(result.Model)

	case StateError:
		var updated tea.Model
		updated, cmd = m.errorModel.Update(msg)
		m.errorModel = updated.(errorview.Model)
	}

	return m, cmd
}

func (m MainModel) applyCmd(s switcher.WorkspaceItem) tea.Cmd {
	return func() tea.Msg {
		// TODO: Replace with actual migration logic
		// Call m.backend.Apply(ctx, Request{...})
		_ = s
		return result.MigrationCompleteMsg{
			Destination: s.ProjectRoot,
			Message:     "Session ready for migration",
		}
	}
}

func (m MainModel) View() tea.View {
	if m.quitting {
		return tea.NewView("Goodbye!\n")
	}

	var content string
	switch m.state {
	case StateLoading:
		content = m.loadingModel.View().Content
	case StateSessionList:
		content = m.sessionModel.View().Content
	case StateSessionPreview:
		content = m.previewModel.View().Content
	case StateApplying, StateComplete:
		content = m.resultModel.View().Content
	case StateError:
		content = m.errorModel.View().Content
	}

	view := tea.NewView(styles.AppContainer.Render(content))
	view.AltScreen = true
	return view
}
