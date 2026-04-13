package session

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
)

// PreviewModel shows session details before migration.
type PreviewModel struct {
	session switcher.WorkspaceItem
	width   int
	height  int
}

func NewPreviewModel() PreviewModel {
	return PreviewModel{}
}

func (m PreviewModel) Init() tea.Cmd {
	return nil
}

func (m PreviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "escape", "left":
			return m, func() tea.Msg { return GoBackMsg{} }
		case "enter", " ":
			return m, func() tea.Msg { return StartMigrationMsg{Session: m.session} }
		}
	}
	return m, nil
}

func (m PreviewModel) View() tea.View {
	if m.session.ID == "" {
		return tea.NewView("")
	}

	var lines []string
	lines = append(lines, styles.Title.Render("Session Preview"))
	lines = append(lines, "")

	dot := coloredDot(m.session.Tool)
	lines = append(lines, metaRow("Tool", fmt.Sprintf("%s %s", dot, m.session.Tool)))
	lines = append(lines, metaRow("ID", m.session.ID))
	if m.session.Title != "" && m.session.Title != m.session.ID {
		lines = append(lines, metaRow("Title", m.session.Title))
	}

	if m.session.ProjectRoot != "" {
		link := lipgloss.NewStyle().
			Foreground(styles.Accent).
			Render(m.session.ProjectRoot)
		lines = append(lines, metaRow("Project", link))
	} else {
		lines = append(lines, metaRow("Project", "(current directory)"))
	}

	if m.session.UpdatedAt != "" {
		lines = append(lines, metaRow("Updated", m.session.UpdatedAt))
	}

	lines = append(lines, "")
	lines = append(lines, styles.HelpBar.Render("  [Space] migrate  [Esc] back"))

	content := styles.Card.Render(strings.Join(lines, "\n"))
	return tea.NewView(content)
}

func metaRow(label, value string) string {
	labelStyle := lipgloss.NewStyle().
		Foreground(styles.TextSecondary).
		Width(10).
		Align(lipgloss.Right)
	valueStyle := lipgloss.NewStyle().Foreground(styles.TextPrimary)
	return fmt.Sprintf("  %s  %s", labelStyle.Render(label+":"), valueStyle.Render(value))
}

// SetSession populates the preview model.
func (m *PreviewModel) SetSession(s switcher.WorkspaceItem) {
	m.session = s
}

// Session returns the current session being previewed.
func (m *PreviewModel) Session() switcher.WorkspaceItem {
	return m.session
}
