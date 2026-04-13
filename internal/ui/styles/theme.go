package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// TrueColor palette (Tokyo Night inspired)
var (
	BgPrimary     = lipgloss.Color("#1a1b26")
	Accent        = lipgloss.Color("#7aa2f7")
	TextPrimary   = lipgloss.Color("#c0caf5")
	TextSecondary = lipgloss.Color("#565f89")
	Success       = lipgloss.Color("#9ece6a")
	Warning       = lipgloss.Color("#e0af68")
	ErrorColor    = lipgloss.Color("#f7768e") // renamed from Error to avoid conflict

	// Tool brand colors
	ClaudeColor   = lipgloss.Color("#66b366")
	GeminiColor   = lipgloss.Color("#8b5cf6")
	CodexColor    = lipgloss.Color("#f5a623")
	OpenCodeColor = lipgloss.Color("#4fc3f7")
)

// App-level styles
var (
	AppContainer = lipgloss.NewStyle().
			Padding(1, 2).
			Background(BgPrimary).
			Foreground(TextPrimary)

	Title = lipgloss.NewStyle().
		Foreground(Accent).
		Bold(true).
		MarginBottom(1)

	Subtitle = lipgloss.NewStyle().
		Foreground(TextSecondary).
		MarginBottom(1)

	// Status texts
	WarningText = lipgloss.NewStyle().Foreground(Warning)
	SuccessText = lipgloss.NewStyle().Foreground(Success)
	ErrorText   = lipgloss.NewStyle().Foreground(ErrorColor)

	// Card style for preview/result panels
	Card = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(Accent).
		Padding(1, 2).
		MarginBottom(1)

	ErrorCard = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ErrorColor).
		Padding(1, 2).
		MarginBottom(1)

	// Help bar style
	HelpBar = lipgloss.NewStyle().
		Foreground(TextSecondary).
		PaddingTop(1).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(TextSecondary).
		BorderTop(true)

	// Tool dot colors map
	ToolColors = map[string]color.Color{
		"claude":   ClaudeColor,
		"gemini":   GeminiColor,
		"codex":    CodexColor,
		"opencode": OpenCodeColor,
	}
)
