package styles

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

// ─── Color Palette ──────────────────────────────────────────

var (
	// Base surfaces
	ColorBg        = lipgloss.Color("#0F172A")
	ColorSurface   = lipgloss.Color("#111827")
	ColorSurfaceHL = lipgloss.Color("#1E293B")
	ColorBorder    = lipgloss.Color("#334155")
	ColorBorderHL  = lipgloss.Color("#22D3EE")

	// Primary / Accent
	ColorPrimary   = lipgloss.Color("#22D3EE")
	ColorAccent    = lipgloss.Color("#F59E0B")
	ColorSecondary = lipgloss.Color("#38BDF8")

	// Text
	ColorText     = lipgloss.Color("#E5EEF8")
	ColorTextDim  = lipgloss.Color("#94A3B8")
	ColorTextMute = lipgloss.Color("#64748B")

	// Semantic
	ColorSuccess = lipgloss.Color("#10B981")
	ColorWarning = lipgloss.Color("#F59E0B")
	ColorError   = lipgloss.Color("#EF4444")
	ColorInfo    = lipgloss.Color("#3B82F6")

	// Agent-specific
	ColorCodex    = lipgloss.Color("#F97316") // orange
	ColorGemini   = lipgloss.Color("#3B82F6") // blue
	ColorClaude   = lipgloss.Color("#A855F7") // purple
	ColorOpenCode = lipgloss.Color("#22C55E") // green
)

// AgentColor returns the brand color for a given tool.
func AgentColor(tool string) color.Color {
	switch strings.ToLower(tool) {
	case "codex":
		return ColorCodex
	case "gemini":
		return ColorGemini
	case "claude":
		return ColorClaude
	case "opencode":
		return ColorOpenCode
	default:
		return ColorTextDim
	}
}

// AgentIcon returns a unicode icon for a given tool.
func AgentIcon(tool string) string {
	switch strings.ToLower(tool) {
	case "codex":
		return "◆"
	case "gemini":
		return "✦"
	case "claude":
		return "◈"
	case "opencode":
		return "⬡"
	default:
		return "○"
	}
}

// ─── Layout Styles ──────────────────────────────────────────

var (
	AppContainer = lipgloss.NewStyle().
			Padding(1, 2)

	ActivePane = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderHL).
			Padding(0, 1)

	InactivePane = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	Overlay = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Background(ColorSurface).
		Padding(1, 2)

	Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2)
)

// ─── Typography ─────────────────────────────────────────────

var (
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary)

	Subtitle = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	SectionTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent)

	Selected = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorText)

	Highlight = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	Muted = lipgloss.NewStyle().
		Foreground(ColorTextMute)

	Disabled = lipgloss.NewStyle().
			Foreground(ColorTextMute)
)

// ─── Semantic Styles ────────────────────────────────────────

var (
	SuccessText = lipgloss.NewStyle().Foreground(ColorSuccess)
	WarningText = lipgloss.NewStyle().Foreground(ColorWarning)
	ErrorText   = lipgloss.NewStyle().Foreground(ColorError)
	InfoText    = lipgloss.NewStyle().Foreground(ColorInfo)

	ErrorBox = Panel.
			BorderForeground(ColorError).
			Foreground(ColorError)

	HelpBox = Panel.
		BorderForeground(ColorAccent)

	InputBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1)

	SearchBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorderHL).
			Background(ColorSurfaceHL).
			Foreground(ColorText).
			Padding(0, 1)

	SearchPlaceholder = lipgloss.NewStyle().
				Foreground(ColorTextMute)

	Cursor = lipgloss.NewStyle().
		Background(ColorAccent).
		Foreground(ColorBg)

	Section = lipgloss.NewStyle()

	PathText = lipgloss.NewStyle().
			Foreground(ColorTextMute)
)

// ─── Agent Card Styles ──────────────────────────────────────

// AgentCardActive returns a card style for an active (connected) agent.
func AgentCardActive(tool string) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(AgentColor(tool)).
		Foreground(ColorText).
		Padding(1, 2).
		Bold(true)
}

// AgentCardInactive returns a card style for an inactive agent.
func AgentCardInactive() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Foreground(ColorTextMute).
		Padding(1, 2)
}

// ─── Command Palette ────────────────────────────────────────

var (
	CmdPaletteInput = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Foreground(ColorText).
			Padding(0, 1)

	CmdSuggestion = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Padding(0, 1)

	CmdSuggestionActive = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAccent).
				Padding(0, 1)

	CmdSlash = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)
)

// ─── Quick Action ───────────────────────────────────────────

var (
	QuickAction = lipgloss.NewStyle().
			Foreground(ColorText).
			Padding(0, 1)

	QuickActionActive = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAccent).
				Padding(0, 1)

	QuickActionKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)
)

// ─── Button Styles ──────────────────────────────────────────

var (
	ButtonPrimary = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBg).
			Background(ColorPrimary).
			Padding(0, 2)

	ButtonSecondary = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorText).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 2)

	ButtonActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBg).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Background(ColorAccent).
			Padding(0, 2)
)

// ─── Tag / Badge Styles ────────────────────────────────────

var (
	BadgeSuccess = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSuccess)

	BadgeWarning = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWarning)

	BadgeError = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorError)

	BadgeMuted = lipgloss.NewStyle().
			Foreground(ColorTextMute)

	ToolBadge = lipgloss.NewStyle().
			Bold(true)

	MetaBadge = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Background(ColorSurfaceHL).
			Padding(0, 1)

	MetaBadgeActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBg).
			Background(ColorAccent).
			Padding(0, 1)
)

// ToolBadgeFor returns a tool badge styled with the agent color.
func ToolBadgeFor(tool string) string {
	return ToolBadge.Foreground(AgentColor(tool)).Render(strings.ToUpper(tool))
}

func MetaTag(text string) string {
	return MetaBadge.Render(strings.ToUpper(strings.TrimSpace(text)))
}

func MetaTagActive(text string) string {
	return MetaBadgeActive.Render(strings.ToUpper(strings.TrimSpace(text)))
}

func SectionPill(label string, tool string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "Items"
	}
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBg).
		Background(ColorAccent).
		Padding(0, 1)
	if strings.TrimSpace(tool) != "" {
		style = style.Background(AgentColor(tool))
	}
	return style.Render(strings.ToUpper(label))
}

// ─── Breadcrumb ─────────────────────────────────────────────

var (
	BreadcrumbSep = lipgloss.NewStyle().
			Foreground(ColorTextMute)

	BreadcrumbItem = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	BreadcrumbActive = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAccent)
)

// ─── Helpers ────────────────────────────────────────────────

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

// ConnBadge returns a connection status badge.
func ConnBadge(connected bool) string {
	if connected {
		return BadgeSuccess.Render("● Active")
	}
	return BadgeMuted.Render("○ Not Found")
}
