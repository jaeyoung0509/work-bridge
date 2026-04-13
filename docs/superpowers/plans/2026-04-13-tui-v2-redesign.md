# TUI V2 Base Architecture & Session View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish a component-based router architecture for the work-bridge TUI and implement the initial "Select Session" view using Bubble Tea.

**Architecture:** Create `internal/ui` to house the new TUI version. Define centralized styles in `internal/ui/styles/theme.go`. Create a main router (`MainModel`) in `internal/ui/app.go` to handle state delegation. Implement `internal/ui/views/session/session.go` using `github.com/charmbracelet/bubbles/list` for rendering recent sessions.

**Tech Stack:** Go, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/bubbles`

---

### Task 1: Scaffolding Centralized Styles

**Files:**
- Create: `internal/ui/styles/theme.go`

- [ ] **Step 1: Define Lip Gloss styles**

```go
package styles

import (
	"charm.land/lipgloss/v2"
)

var (
	// AppLevel styles
	AppContainer = lipgloss.NewStyle().Padding(1, 2)
	
	// Typography
	Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")). // Purple/Blue
		Bold(true).
		MarginBottom(1)
		
	Subtitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginBottom(1)

	// Status colors
	WarningText = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	SuccessText = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	ErrorText   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)
```

- [ ] **Step 2: Commit**

```bash
git add internal/ui/styles/theme.go
git commit -m "feat(ui): add centralized lipgloss theme styles"
```

### Task 2: Implement the Session List View

**Files:**
- Create: `internal/ui/views/session/session.go`

- [ ] **Step 1: Implement Session List Model using bubbles/list**

```go
package session

import (
	"context"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/bubbles/list"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
)

type SessionSelectedMsg struct {
	Session switcher.WorkspaceItem
}

type Model struct {
	List list.Model
	err  error
}

type item struct {
	session switcher.WorkspaceItem
}

func (i item) Title() string       { return i.session.Title }
func (i item) Description() string { return string(i.session.Tool) + " • " + i.session.ID }
func (i item) FilterValue() string { return i.session.Title }

func NewModel() Model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select a Session"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	return Model{List: l}
}

func (m Model) Init() (tea.Model, tea.Cmd) {
	return m, nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, w := msg.Height, msg.Width
		m.List.SetSize(w, h)
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "enter" {
			if selected, ok := m.List.SelectedItem().(item); ok {
				return m, func() tea.Msg {
					return SessionSelectedMsg{Session: selected.session}
				}
			}
		}
	}

	var cmd tea.Cmd
	m.List, cmd = m.List.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	return m.List.View()
}

func (m *Model) SetSessions(sessions []switcher.WorkspaceItem) {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = item{session: s}
	}
	m.List.SetItems(items)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/ui/views/session/session.go
git commit -m "feat(ui): implement session list view using bubbles/list"
```

### Task 3: Implement the Main App Router

**Files:**
- Create: `internal/ui/app.go`

- [ ] **Step 1: Create MainModel Router**

```go
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

func (m MainModel) Init() (tea.Model, tea.Cmd) {
	return m, m.loadWorkspaceCmd()
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

func (m MainModel) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var content string
	switch m.state {
	case StateSelectSession:
		content = m.sessionView.View()
	}

	return styles.AppContainer.Render(content)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/ui/app.go
git commit -m "feat(ui): add main router model to handle state delegation"
```

### Task 4: Hook up CLI command for TUI V2

**Files:**
- Modify: `internal/cli/app.go`

- [ ] **Step 1: Add a hidden flag to trigger v2 TUI**

Modify `internal/cli/app.go` `BuildRootCommand` to accept a `--v2` hidden flag that kicks off the new TUI instead of the old one.

*(Pseudocode since actual `app.go` implementation depends on current Cobra setup. Focus on injecting `ui.NewMainModel` to `tea.NewProgram` when the flag is true).*

```go
// In root command RunE
// if v2Flag {
//    p := tea.NewProgram(ui.NewMainModel(ctx, switcherSvc), tea.WithAltScreen())
//    _, err := p.Run()
//    return err
// }
```

- [ ] **Step 2: Verify Compilation**

Run: `go build ./cmd/work-bridge`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/cli/app.go
git commit -m "feat(cli): hook up new component-based TUI under hidden v2 flag"
```
