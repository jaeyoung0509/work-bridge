package errorview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jaeyoung0509/work-bridge/internal/ui/common"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
)

// RetryMsg is emitted when user retries an operation.
type RetryMsg struct{}

// GoBackMsg is emitted when user navigates back.
type GoBackMsg struct{}

// Model displays an error with optional retry.
type Model struct {
	err        error
	title      string
	canRetry   bool
	retryLabel string
}

func NewModel() Model {
	return Model{
		title:      "Error",
		canRetry:   false,
		retryLabel: "r",
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r":
			if m.canRetry {
				return m, func() tea.Msg { return RetryMsg{} }
			}
		case "escape":
			return m, func() tea.Msg { return GoBackMsg{} }
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	errMsg := "Unknown error"
	if m.err != nil {
		errMsg = m.err.Error()
	}

	lines := []string{
		styles.ErrorText.Bold(true).Render("✗ " + m.title),
		"",
		styles.ErrorText.Render(errMsg),
	}

	helpKeys := map[string]string{}
	if m.canRetry {
		helpKeys["r"] = m.retryLabel
	}
	helpKeys["escape"] = "back"
	helpKeys["ctrl+c"] = "quit"

	lines = append(lines, "")
	lines = append(lines, common.RenderHelp(helpKeys))

	content := styles.ErrorCard.Render(strings.Join(lines, "\n"))
	return tea.NewView(content)
}

func (m *Model) SetError(err error, canRetry bool) {
	m.err = err
	m.canRetry = canRetry
}

func (m *Model) SetTitle(title string) {
	m.title = title
}
