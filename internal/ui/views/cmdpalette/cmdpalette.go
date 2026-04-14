package cmdpalette

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
)

// Command represents a slash command entry.
type Command struct {
	Name        string
	Description string
}

// ExecMsg is emitted when a command is executed.
type ExecMsg struct {
	Command string
}

// CancelMsg is emitted when the palette is dismissed.
type CancelMsg struct{}

// Model is the command palette with autocomplete.
type Model struct {
	commands       []Command
	input          string
	cursor         int
	suggestions    []Command
	selectedIdx    int
	active         bool
	width          int
}

// New creates a new command palette.
func New(commands []Command) Model {
	return Model{
		commands:    commands,
		suggestions: commands,
	}
}

// DefaultCommands returns the standard slash commands.
func DefaultCommands() []Command {
	return []Command{
		{Name: "/projects", Description: "Browse and migrate sessions per project"},
		{Name: "/sessions", Description: "Select and import sessions"},
		{Name: "/mcp", Description: "Manage MCP server configurations"},
		{Name: "/skills", Description: "Browse available skills"},
	}
}

func (m Model) Active() bool  { return m.active }
func (m Model) Input() string { return m.input }

// Open activates the palette with optional initial text.
func (m Model) Open(initial string) Model {
	m.active = true
	m.input = initial
	m.cursor = len([]rune(initial))
	m.selectedIdx = 0
	m.updateSuggestions()
	return m
}

// Close deactivates the palette.
func (m Model) Close() Model {
	m.active = false
	m.input = ""
	m.cursor = 0
	m.selectedIdx = 0
	m.suggestions = m.commands
	return m
}

func (m *Model) SetWidth(w int) { m.width = w }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			m = m.Close()
			return m, func() tea.Msg { return CancelMsg{} }
		case "enter":
			cmd := m.resolveCommand()
			m = m.Close()
			if cmd != "" {
				return m, func() tea.Msg { return ExecMsg{Command: cmd} }
			}
			return m, nil
		case "tab":
			if len(m.suggestions) > 0 {
				m.input = m.suggestions[m.selectedIdx].Name
				m.cursor = len([]rune(m.input))
				m.updateSuggestions()
			}
			return m, nil
		case "up":
			if m.selectedIdx > 0 {
				m.selectedIdx--
			}
			return m, nil
		case "down":
			if m.selectedIdx < len(m.suggestions)-1 {
				m.selectedIdx++
			}
			return m, nil
		case "backspace":
			if m.cursor > 0 {
				runes := []rune(m.input)
				runes = append(runes[:m.cursor-1], runes[m.cursor:]...)
				m.cursor--
				m.input = string(runes)
				m.updateSuggestions()
			}
			return m, nil
		case "left":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "right":
			if m.cursor < len([]rune(m.input)) {
				m.cursor++
			}
			return m, nil
		default:
			key := msg.Key()
			if key.Text != "" && key.Mod == 0 {
				runes := []rune(m.input)
				insert := []rune(key.Text)
				head := append([]rune{}, runes[:m.cursor]...)
				head = append(head, insert...)
				head = append(head, runes[m.cursor:]...)
				m.input = string(head)
				m.cursor += len(insert)
				m.selectedIdx = 0
				m.updateSuggestions()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) updateSuggestions() {
	if m.input == "" || m.input == "/" {
		m.suggestions = m.commands
		return
	}
	query := strings.ToLower(m.input)
	results := make([]Command, 0, len(m.commands))
	for _, cmd := range m.commands {
		if strings.Contains(strings.ToLower(cmd.Name), query) || strings.HasPrefix(strings.ToLower(cmd.Name), query) {
			results = append(results, cmd)
		}
	}
	m.suggestions = results
	if m.selectedIdx >= len(m.suggestions) {
		m.selectedIdx = max(0, len(m.suggestions)-1)
	}
}

func (m Model) resolveCommand() string {
	// Exact match first
	trimmed := strings.TrimSpace(m.input)
	for _, cmd := range m.commands {
		if strings.EqualFold(cmd.Name, trimmed) {
			return cmd.Name
		}
	}
	// Selected suggestion
	if len(m.suggestions) > 0 && m.selectedIdx < len(m.suggestions) {
		return m.suggestions[m.selectedIdx].Name
	}
	// Raw input
	if trimmed != "" {
		return trimmed
	}
	return ""
}

func (m Model) View() tea.View {
	if !m.active {
		return tea.NewView("")
	}
	w := m.width
	if w < 40 {
		w = 60
	}
	innerW := w - 8

	var b strings.Builder

	// Input line
	inputDisplay := m.inputWithCursor()
	prompt := styles.CmdSlash.Render("⌘ ")
	inputLine := prompt + inputDisplay
	b.WriteString(styles.CmdPaletteInput.Width(innerW).Render(inputLine))
	b.WriteString("\n")

	// Suggestions dropdown
	if len(m.suggestions) > 0 {
		for i, cmd := range m.suggestions {
			var line string
			if i == m.selectedIdx {
				line = styles.CmdSuggestionActive.Width(innerW).Render(
					"▸ " + styles.QuickActionKey.Render(cmd.Name) + "  " + cmd.Description,
				)
			} else {
				line = styles.CmdSuggestion.Width(innerW).Render(
					"  " + cmd.Name + "  " + styles.Muted.Render(cmd.Description),
				)
			}
			b.WriteString(line)
			if i < len(m.suggestions)-1 {
				b.WriteString("\n")
			}
		}
	} else if m.input != "" {
		b.WriteString(styles.Muted.Render("  No matching commands"))
	}

	content := b.String()
	styled := styles.Overlay.Width(innerW + 4).Render(content)
	return tea.NewView(styled)
}

func (m Model) inputWithCursor() string {
	runes := []rune(m.input)
	pos := m.cursor
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	cursor := styles.Cursor.Render(" ")
	left := string(runes[:pos])
	right := string(runes[pos:])
	return left + cursor + right
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// RenderFloating renders the palette as a centered floating panel.
func (m Model) RenderFloating(width, height int) string {
	view := m.View().Content
	lines := strings.Split(view, "\n")
	totalLines := len(lines)

	padTop := (height - totalLines) / 2
	if padTop < 2 {
		padTop = 2
	}

	var b strings.Builder
	for i := 0; i < padTop; i++ {
		b.WriteString("\n")
	}
	centerer := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	b.WriteString(centerer.Render(view))
	return b.String()
}
