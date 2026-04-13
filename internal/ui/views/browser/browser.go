package browser

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/bubbles/list"
)

type Entry struct {
	Key         string
	Title       string
	Description string
	FilterValue string
	Details     []string
}

type SelectedMsg struct {
	Entry Entry
}

type Model struct {
	List list.Model
}

type item struct {
	entry Entry
}

func (i item) Title() string       { return i.entry.Title }
func (i item) Description() string { return i.entry.Description }
func (i item) FilterValue() string {
	if i.entry.FilterValue != "" {
		return i.entry.FilterValue
	}
	return i.entry.Title + " " + i.entry.Description
}

func NewModel(title string) Model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	return Model{List: l}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	newList, listCmd := m.List.Update(msg)
	m.List = newList
	if listCmd != nil {
		cmd = func() tea.Msg { return listCmd() }
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.List.SetSize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "enter" {
			if selected, ok := m.List.SelectedItem().(item); ok {
				return m, func() tea.Msg { return SelectedMsg{Entry: selected.entry} }
			}
		}
	case tea.MouseClickMsg:
		if msg.Mouse().Button == tea.MouseLeft {
			if selected, ok := m.List.SelectedItem().(item); ok {
				return m, func() tea.Msg { return SelectedMsg{Entry: selected.entry} }
			}
		}
	}

	return m, cmd
}

func (m Model) View() tea.View {
	return tea.NewView(m.List.View())
}

func (m *Model) SetTitle(title string) {
	m.List.Title = title
}

func (m *Model) SetEntries(entries []Entry) {
	items := make([]list.Item, len(entries))
	for i, entry := range entries {
		items[i] = item{entry: entry}
	}
	m.List.SetItems(items)
}

func (m *Model) SetSize(width, height int) {
	m.List.SetSize(width, height)
}

func (m Model) SelectedEntry() (Entry, bool) {
	selected, ok := m.List.SelectedItem().(item)
	if !ok {
		return Entry{}, false
	}
	return selected.entry, true
}
