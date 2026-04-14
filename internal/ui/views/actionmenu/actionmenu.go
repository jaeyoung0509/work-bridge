package actionmenu

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/browser"
)

type BackMsg struct{}

type ActionType string

const (
	ActionEdit    ActionType = "edit"
	ActionMigrate ActionType = "migrate"
)

type ActionSelectedMsg struct {
	Entry      browser.Entry
	ActionType ActionType
	Target     domain.Tool // only set if ActionMigrate
}

type Model struct {
	entry   browser.Entry
	options []actionOption
	cursor  int
	width   int
	height  int
	
	status      string
	statusStyle lipgloss.Style
}

type actionOption struct {
	label  string
	action ActionType
	target domain.Tool
}

func New(entry browser.Entry) Model {
	options := []actionOption{
		{label: "Edit in $EDITOR", action: ActionEdit},
	}
	
	for _, tool := range []domain.Tool{domain.ToolCodex, domain.ToolGemini, domain.ToolClaude, domain.ToolOpenCode} {
		if strings.ToLower(entry.Badge) != string(tool) {
			options = append(options, actionOption{
				label:  "Install to " + strings.ToUpper(string(tool)),
				action: ActionMigrate,
				target: tool,
			})
		}
	}

	return Model{
		entry:   entry,
		options: options,
		cursor:  0,
	}
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) EntryTitle() string {
	return m.entry.Title
}

func (m *Model) SetStatus(msg string, isError bool) {
	m.status = msg
	if isError {
		m.statusStyle = styles.ErrorText
	} else {
		m.statusStyle = styles.SuccessText
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "esc", "b":
			return m, func() tea.Msg { return BackMsg{} }
		case "enter":
			opt := m.options[m.cursor]
			return m, func() tea.Msg {
				return ActionSelectedMsg{
					Entry:      m.entry,
					ActionType: opt.action,
					Target:     opt.target,
				}
			}
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("")
	}
	
	panelWidth := m.width - 4
	if panelWidth > 80 {
		panelWidth = 80
	}

	var lines []string
	
	// Details Section
	badge := ""
	if m.entry.Badge != "" {
		badge = styles.ToolBadgeFor(m.entry.Badge) + " "
	}
	lines = append(lines,
		styles.SectionTitle.Render("◈ File Actions"),
		"",
		"  " + badge + styles.Selected.Render(m.entry.Title),
		"  " + styles.Muted.Render(m.entry.Description),
		"  " + styles.Muted.Render("Path: "+m.entry.Key),
		"",
		styles.SectionTitle.Render("◇ Options"),
		"",
	)

	// Options
	for i, opt := range m.options {
		prefix := "    "
		label := styles.Muted.Render(opt.label)
		if i == m.cursor {
			prefix = styles.Highlight.Render("  ▸ ")
			label = styles.Selected.Render(opt.label)
		}
		lines = append(lines, prefix+label)
	}
	
	lines = append(lines, "")
	if m.status != "" {
		lines = append(lines, "  "+m.statusStyle.Render(m.status))
	} else {
		lines = append(lines, styles.Muted.Render("  esc/b: back • enter: select option"))
	}
	
	content := strings.Join(lines, "\n")
	box := styles.Panel.Width(panelWidth).Render(content)
	
	return tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box))
}
