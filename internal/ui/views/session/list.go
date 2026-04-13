package session

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/bubbles/v2/list"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
	"github.com/jaeyoung0509/work-bridge/internal/ui/common"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
)

// SessionSelectedMsg is emitted when a session is selected.
type SessionSelectedMsg struct {
	Session switcher.WorkspaceItem
}

// GoBackMsg is emitted when user navigates back.
type GoBackMsg struct{}

// StartMigrationMsg is emitted when user confirms migration.
type StartMigrationMsg struct {
	Session switcher.WorkspaceItem
}

// Model wraps the bubbles list for session browsing.
type Model struct {
	List  list.Model
	err   error
	width int
}

type item struct {
	session switcher.WorkspaceItem
}

func (i item) Title() string {
	dot := coloredDot(i.session.Tool)
	return fmt.Sprintf("%s %s", dot, i.session.Tool)
}

func (i item) Description() string {
	parts := []string{i.session.ID, i.session.Title}
	if i.session.ProjectRoot != "" {
		parts = append(parts, shortenPath(i.session.ProjectRoot))
	}
	return strings.Join(parts, "  •  ")
}

func (i item) FilterValue() string {
	return string(i.session.Tool) + " " + i.session.Title + " " + i.session.ID
}

func coloredDot(tool domain.Tool) string {
	color, ok := styles.ToolColors[string(tool)]
	if !ok {
		color = styles.TextSecondary
	}
	return lipgloss.NewStyle().Foreground(color).Render("●")
}

func shortenPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path
	}
	return strings.Join(parts[len(parts)-2:], "/")
}

func NewModel() Model {
	delegate := list.NewDefaultDelegate()
	s := list.NewDefaultItemStyles(true) // dark mode
	s.SelectedTitle = s.SelectedTitle.Foreground(styles.Accent).Bold(true)
	s.SelectedDesc = s.SelectedDesc.Foreground(styles.TextSecondary)
	s.NormalTitle = s.NormalTitle.Foreground(styles.TextPrimary)
	s.NormalDesc = s.NormalDesc.Foreground(styles.TextSecondary)
	delegate.Styles = s

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Select a Session"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false) // We render our own help bar
	l.SetFilteringEnabled(true)

	return Model{List: l}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.List.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ":
			if sel, ok := m.List.SelectedItem().(item); ok {
				return m, func() tea.Msg {
					return SessionSelectedMsg{Session: sel.session}
				}
			}
		}
	}

	var cmd tea.Cmd
	m.List, cmd = m.List.Update(msg)
	return m, cmd
}

func (m Model) View() tea.View {
	view := m.List.View()
	help := common.RenderHelp(map[string]string{
		"enter":  "select",
		"/":      "filter",
		"ctrl+c": "quit",
	})
	return tea.NewView(view + help)
}

func (m *Model) SetSessions(sessions []switcher.WorkspaceItem) {
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = item{session: s}
	}
	m.List.SetItems(items)
}

func (m *Model) SetError(err error) {
	m.err = err
}
