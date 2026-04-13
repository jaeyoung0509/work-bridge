package result

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/bubbles/v2/spinner"
	"github.com/jaeyoung0509/work-bridge/internal/ui/common"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
)

type State int

const (
	StateApplying State = iota
	StateComplete
	StateFailed
)

// MigrationCompleteMsg is emitted when migration succeeds.
type MigrationCompleteMsg struct {
	Destination string
	Message     string
}

// MigrationFailedMsg is emitted when migration fails.
type MigrationFailedMsg struct {
	Err error
}

// SelectAnotherMsg is emitted when user wants to pick another session.
type SelectAnotherMsg struct{}

// GoBackMsg is emitted when user navigates back.
type GoBackMsg struct{}

// RetryMigrationMsg is emitted when user retries a failed migration.
type RetryMigrationMsg struct{}

// Model handles applying, success, and failure states.
type Model struct {
	spinner     spinner.Model
	state       State
	message     string
	destination string
	err         error
	width       int
	height      int
}

func NewModel() Model {
	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(styles.Accent)),
	)
	return Model{
		spinner: s,
		state:   StateApplying,
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
		case "enter":
			if m.state == StateComplete {
				return m, func() tea.Msg { return SelectAnotherMsg{} }
			}
		case "r":
			if m.state == StateFailed {
				return m, func() tea.Msg { return RetryMigrationMsg{} }
			}
		case "escape":
			if m.state == StateFailed {
				return m, func() tea.Msg { return GoBackMsg{} }
			}
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

	case MigrationCompleteMsg:
		m.state = StateComplete
		m.message = msg.Message
		m.destination = msg.Destination
		return m, nil

	case MigrationFailedMsg:
		m.state = StateFailed
		m.err = msg.Err
		return m, nil
	}

	return m, nil
}

func (m Model) View() tea.View {
	var content string

	switch m.state {
	case StateApplying:
		line := m.spinner.View() + " Migrating session..."
		content = styles.AppContainer.Render(styles.Subtitle.Render(line))

	case StateComplete:
		var lines []string
		lines = append(lines, styles.SuccessText.Bold(true).Render("✓  Migration Complete"))
		lines = append(lines, "")
		if m.message != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(styles.TextPrimary).Render(m.message))
			lines = append(lines, "")
		}
		if m.destination != "" {
			link := lipgloss.NewStyle().
				Foreground(styles.Accent).
				Render(m.destination)
			lines = append(lines, "  "+link)
		}
		lines = append(lines, "")
		lines = append(lines, common.RenderHelp(map[string]string{
			"enter":  "select another",
			"ctrl+c": "quit",
		}))
		content = styles.AppContainer.Render(strings.Join(lines, "\n"))

	case StateFailed:
		var lines []string
		lines = append(lines, styles.ErrorText.Bold(true).Render("✗  Migration Failed"))
		lines = append(lines, "")
		if m.err != nil {
			lines = append(lines, styles.ErrorText.Render(m.err.Error()))
		}
		lines = append(lines, "")
		lines = append(lines, common.RenderHelp(map[string]string{
			"r":      "retry",
			"escape": "back",
			"ctrl+c": "quit",
		}))
		content = styles.AppContainer.Render(strings.Join(lines, "\n"))
	}

	return tea.NewView(content)
}
