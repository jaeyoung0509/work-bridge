package browser

import (
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/bubbles/list"
	teav1 "github.com/charmbracelet/bubbletea"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
)

type Entry struct {
	Key         string
	Title       string
	Description string
	Badge       string
	Section     string
	FilterValue string
	Meta        []string
	Details     []string
	Raw         any
}

type SelectedMsg struct {
	Entry Entry
}

type Model struct {
	List            list.Model
	title           string
	subtitle        string
	allEntries      []Entry
	filteredEntries []Entry
	query           string
	width           int
	height          int
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
	parts := []string{i.entry.Title, i.entry.Description, i.entry.Section}
	parts = append(parts, i.entry.Meta...)
	parts = append(parts, i.entry.Details...)
	return strings.Join(parts, " ")
}

type delegate struct {
	height  int
	spacing int
}

func (d delegate) Height() int                                 { return d.height }
func (d delegate) Spacing() int                                { return d.spacing }
func (d delegate) Update(_ teav1.Msg, _ *list.Model) teav1.Cmd { return nil }

func (d delegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(item)
	if !ok {
		return
	}

	selected := index == m.Index()
	sectionLine := ""
	if shouldRenderSectionHeader(m, index, it.entry.Section) {
		sectionLine = "  " + styles.SectionPill(it.entry.Section, it.entry.Badge)
	}

	badge := ""
	if it.entry.Badge != "" {
		badge = styles.ToolBadgeFor(it.entry.Badge) + "  "
	}

	prefix := "    "
	titleStyle := styles.Selected
	descStyle := lipgloss.NewStyle().Foreground(styles.ColorTextDim)
	if selected {
		prefix = styles.Highlight.Render("  ▸ ")
		titleStyle = styles.Highlight
		descStyle = lipgloss.NewStyle().Foreground(styles.ColorText)
	}

	meta := renderMeta(it.entry.Meta, selected)
	titleLine := prefix + badge + titleStyle.Render(it.entry.Title)
	if meta != "" {
		titleLine += "  " + meta
	}

	detail := strings.Join(compactStrings(it.entry.Details), "  •  ")
	if detail == "" {
		detail = " "
	} else {
		detail = "    " + styles.PathText.Render(detail)
	}

	if sectionLine == "" {
		sectionLine = " "
	}

	fmt.Fprint(w,
		sectionLine+"\n"+
			titleLine+"\n"+
			"    "+descStyle.Render(it.entry.Description)+"\n"+
			detail,
	)
}

func NewModel(title string) Model {
	l := list.New([]list.Item{}, delegate{height: 4, spacing: 0}, 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	return Model{List: l, title: title}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if selected, ok := m.List.SelectedItem().(item); ok {
				return m, func() tea.Msg { return SelectedMsg{Entry: selected.entry} }
			}
			return m, nil
		case "backspace":
			if m.query != "" {
				m.query = trimLastRune(m.query)
				m.applyFilter()
			}
			return m, nil
		case "ctrl+u":
			if m.query != "" {
				m.query = ""
				m.applyFilter()
			}
			return m, nil
		case "up", "down", "pgup", "pgdown", "home", "end":
			// Let the list handle navigation.
		default:
			key := msg.Key()
			if key.Mod == 0 && key.Text != "" {
				m.query += key.Text
				m.applyFilter()
				return m, nil
			}
		}
	case tea.MouseClickMsg:
		if msg.Mouse().Button == tea.MouseLeft {
			if selected, ok := m.List.SelectedItem().(item); ok {
				return m, func() tea.Msg { return SelectedMsg{Entry: selected.entry} }
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
	searchValue := m.query
	if searchValue == "" {
		searchValue = styles.SearchPlaceholder.Render("type to filter")
	} else {
		searchValue = styles.Selected.Render(searchValue)
	}

	meta := []string{fmt.Sprintf("%d results", len(m.filteredEntries))}
	if sections := countSections(m.filteredEntries); sections > 1 {
		meta = append(meta, fmt.Sprintf("%d groups", sections))
	}
	if m.subtitle != "" {
		meta = append(meta, m.subtitle)
	}

	header := styles.SectionTitle.Render(m.title)
	search := styles.SearchBox.Width(max(28, m.width-4)).Render("⌕  " + searchValue)
	status := styles.Muted.Render(strings.Join(meta, "  •  "))
	listView := m.List.View()

	content := lipgloss.JoinVertical(lipgloss.Left, header, search, status, "", listView)
	return tea.NewView(content)
}

func (m *Model) SetTitle(title string) {
	m.title = title
}

func (m *Model) SetSubtitle(subtitle string) {
	m.subtitle = strings.TrimSpace(subtitle)
}

func (m *Model) SetEntries(entries []Entry) {
	m.allEntries = append([]Entry{}, entries...)
	m.applyFilter()
}

func (m *Model) ResetQuery() {
	m.query = ""
	m.applyFilter()
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.List.SetSize(width, max(6, height-5))
}

func (m Model) SelectedEntry() (Entry, bool) {
	selected, ok := m.List.SelectedItem().(item)
	if !ok {
		return Entry{}, false
	}
	return selected.entry, true
}

func (m Model) EntryCount() int {
	return len(m.filteredEntries)
}

func (m Model) Query() string {
	return m.query
}

func (m *Model) applyFilter() {
	selectedKey := ""
	selectedIndex := m.List.Index()
	if selected, ok := m.SelectedEntry(); ok {
		selectedKey = selected.Key
	}

	query := strings.ToLower(strings.TrimSpace(m.query))
	filtered := make([]Entry, 0, len(m.allEntries))
	for _, entry := range m.allEntries {
		if matchesQuery(entry, query) {
			filtered = append(filtered, entry)
		}
	}
	m.filteredEntries = filtered

	items := make([]list.Item, len(filtered))
	for i, entry := range filtered {
		items[i] = item{entry: entry}
	}
	m.List.SetItems(items)
	if len(items) == 0 {
		return
	}

	for i, entry := range filtered {
		if entry.Key == selectedKey && selectedKey != "" {
			m.List.Select(i)
			return
		}
	}
	if selectedIndex >= len(items) {
		selectedIndex = len(items) - 1
	}
	if selectedIndex < 0 {
		selectedIndex = 0
	}
	m.List.Select(selectedIndex)
}

func matchesQuery(entry Entry, query string) bool {
	if query == "" {
		return true
	}
	value := entry.FilterValue
	if value == "" {
		parts := []string{entry.Title, entry.Description, entry.Section}
		parts = append(parts, entry.Meta...)
		parts = append(parts, entry.Details...)
		value = strings.Join(parts, " ")
	}
	return strings.Contains(strings.ToLower(value), query)
}

func countSections(entries []Entry) int {
	seen := map[string]struct{}{}
	for _, entry := range entries {
		key := strings.TrimSpace(entry.Section)
		if key == "" {
			key = "Items"
		}
		seen[key] = struct{}{}
	}
	return len(seen)
}

func shouldRenderSectionHeader(m list.Model, index int, section string) bool {
	section = strings.TrimSpace(section)
	if section == "" {
		section = "Items"
	}
	if index == 0 {
		return true
	}
	previous, ok := m.Items()[index-1].(item)
	if !ok {
		return true
	}
	prevSection := strings.TrimSpace(previous.entry.Section)
	if prevSection == "" {
		prevSection = "Items"
	}
	return prevSection != section
}

func renderMeta(values []string, selected bool) string {
	parts := make([]string, 0, len(values))
	for _, value := range compactStrings(values) {
		if selected {
			parts = append(parts, styles.MetaTagActive(value))
			continue
		}
		parts = append(parts, styles.MetaTag(value))
	}
	return strings.Join(parts, " ")
}

func compactStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
