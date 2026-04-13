package styles

import (
	"charm.land/lipgloss/v2"
)

var (
	// AppLevel styles
	AppContainer = lipgloss.NewStyle().Padding(1, 2)
	
	// Typography
	Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")). // Purple/Blue
		Bold(true).
		MarginBottom(1)
		
	Subtitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		MarginBottom(1)

	// Status colors
	WarningText = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	SuccessText = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	ErrorText   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)
