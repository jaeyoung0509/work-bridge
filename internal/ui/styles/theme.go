package styles

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

var (
	AppContainer = lipgloss.NewStyle().Padding(1, 2)

	Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("244")).
		Padding(1, 2)

	Section = lipgloss.NewStyle()

	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("81"))

	Subtitle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	SectionTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("117"))

	Selected  = lipgloss.NewStyle().Bold(true)
	Highlight = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("81"))

	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	Disabled = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	WarningText = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	SuccessText = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	ErrorText   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))

	ErrorBox = Panel.Copy().
			BorderForeground(lipgloss.Color("203")).
			Foreground(lipgloss.Color("203"))

	HelpBox = Panel.Copy().
		BorderForeground(lipgloss.Color("81"))

	InputBox = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("81")).
			Padding(0, 1)

	Cursor = lipgloss.NewStyle().
		Background(lipgloss.Color("81")).
		Foreground(lipgloss.Color("16"))
)

func Status(state domain.SwitchState) string {
	var style lipgloss.Style
	switch state {
	case domain.SwitchStateApplied:
		style = SuccessText
	case domain.SwitchStatePartial:
		style = WarningText
	case domain.SwitchStateError:
		style = ErrorText
	default:
		style = Highlight
	}
	return style.Render(strings.ToUpper(string(state)))
}
