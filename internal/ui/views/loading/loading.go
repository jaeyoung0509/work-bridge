package loading

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/bubbles/v2/spinner"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
)

type Model struct {
	spinner spinner.Model
	message string
	err     error
	width   int
	height  int
}

func NewModel(msg ...string) Model {
	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(styles.Accent)),
	)
	message := "Discovering sessions..."
	if len(msg) > 0 && msg[0] != "" {
		message = msg[0]
	}
	return Model{
		spinner: s,
		message: message,
	}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg { return m.spinner.Tick() }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		var updated spinner.Model
		updated, cmd = m.spinner.Update(msg)
		m.spinner = updated
		return m, cmd
	}

	return m, nil
}

func (m Model) View() tea.View {
	var content string
	if m.err != nil {
		content = styles.ErrorCard.Render(
			styles.ErrorText.Render("✗ "+m.err.Error()),
		)
	} else {
		line := m.spinner.View() + " " + m.message
		view := styles.AppContainer.Render(
			styles.Subtitle.Render(line),
		)
		content = centerVertical(view, m.height)
	}
	return tea.NewView(content)
}

func centerVertical(content string, height int) string {
	if height <= 0 {
		return content
	}
	lines := len(strings.Split(content, "\n"))
	padding := (height - lines) / 2
	if padding > 0 {
		return strings.Repeat("\n", padding) + content
	}
	return content
}
