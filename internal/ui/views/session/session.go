package session

import (
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

func (m Model) Init() tea.Cmd {
	return nil
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
	newList, listCmd := m.List.Update(msg)
	m.List = newList
	if listCmd != nil {
		cmd = func() tea.Msg { return listCmd() }
	}
	return m, cmd
}

func (m Model) View() tea.View {
	return tea.NewView(m.List.View())
}

func (m *Model) SetSessions(sessions []switcher.WorkspaceItem) {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = item{session: s}
	}
	m.List.SetItems(items)
}
