package browser

import (
	"fmt"
	"io"

	teav1 "github.com/charmbracelet/bubbletea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/bubbles/list"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
)

type Entry struct {
	Key         string
	Title       string
	Description string
	Badge       string
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

type delegate struct {
	height  int
	spacing int
}

func (d delegate) Height() int                               { return d.height }
func (d delegate) Spacing() int                              { return d.spacing }
func (d delegate) Update(_ teav1.Msg, _ *list.Model) teav1.Cmd { return nil }

func (d delegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(item)
	if !ok {
		return
	}

	title := it.entry.Title
	desc := it.entry.Description
	badge := it.entry.Badge
	badgeStr := ""

	if badge != "" {
		badgeStr = styles.ToolBadgeFor(badge) + "  "
	}

	var titleStyle, descStyle lipgloss.Style

	if index == m.Index() {
		titleStyle = styles.Highlight
		descStyle = lipgloss.NewStyle().Foreground(styles.ColorTextDim)
		fmt.Fprint(w, "  ▸ "+badgeStr+titleStyle.Render(title)+"\n    "+descStyle.Render(desc))
	} else {
		titleStyle = styles.Selected
		descStyle = styles.Muted
		fmt.Fprint(w, "    "+badgeStr+titleStyle.Render(title)+"\n    "+descStyle.Render(desc))
	}
}

func NewModel(title string) Model {
	l := list.New([]list.Item{}, delegate{height: 2, spacing: 1}, 0, 0)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
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
