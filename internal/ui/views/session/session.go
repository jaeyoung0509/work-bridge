package session

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/browser"
)

type SessionSelectedMsg struct {
	Session switcher.WorkspaceItem
}

type Model struct {
	browser browser.Model
}

func NewModel() Model {
	b := browser.NewModel("Sessions")
	b.SetSubtitle("Search by session title, tool, or session id")
	return Model{browser: b}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.browser.SetSize(size.Width, size.Height)
		return m, nil
	}

	updated, cmd := m.browser.Update(msg)
	m.browser = updated.(browser.Model)
	if cmd != nil {
		result := cmd()
		if selected, ok := result.(browser.SelectedMsg); ok {
			if sessionItem, ok := selected.Entry.Raw.(switcher.WorkspaceItem); ok {
				return m, func() tea.Msg {
					return SessionSelectedMsg{Session: sessionItem}
				}
			}
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	return m.browser.View()
}

func (m *Model) SetSize(width, height int) {
	m.browser.SetSize(width, height)
}

func (m *Model) SetSessions(sessions []switcher.WorkspaceItem) {
	entries := make([]browser.Entry, 0, len(sessions))
	for _, s := range sessions {
		details := []string{}
		if s.ProjectRoot != "" {
			details = append(details, s.ProjectRoot)
		}
		entries = append(entries, browser.Entry{
			Key:         s.ID,
			Title:       s.Title,
			Description: string(s.Tool) + " • " + s.ID,
			Badge:       string(s.Tool),
			Section:     browserSection(string(s.Tool)),
			Details:     details,
			FilterValue: s.Title + " " + s.ID + " " + string(s.Tool) + " " + s.ProjectRoot,
			Raw:         s,
		})
	}
	m.browser.SetEntries(entries)
	m.browser.Select(0)
}

func browserSection(tool string) string {
	switch tool {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	case "gemini":
		return "Gemini"
	case "opencode":
		return "OpenCode"
	default:
		return "Sessions"
	}
}
