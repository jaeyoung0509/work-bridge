package actionmenu

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jaeyoung0509/work-bridge/internal/catalog"
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
	Target     domain.Tool
}

type Model struct {
	entry       browser.Entry
	options     []actionOption
	cursor      int
	width       int
	height      int
	assetLabel  string
	status      string
	statusStyle lipgloss.Style
}

type actionOption struct {
	label  string
	action ActionType
	target domain.Tool
}

func New(entry browser.Entry) Model {
	return Model{
		entry:      entry,
		options:    buildOptions(entry),
		assetLabel: entryKind(entry),
		cursor:     0,
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
			if len(m.options) == 0 {
				return m, nil
			}
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
	if panelWidth > 88 {
		panelWidth = 88
	}

	var lines []string
	badge := ""
	if m.entry.Badge != "" {
		badge = styles.ToolBadgeFor(m.entry.Badge) + " "
	}

	lines = append(lines,
		styles.SectionTitle.Render("◈ "+m.assetLabel),
		"",
		"  "+badge+styles.Selected.Render(m.entry.Title),
		"  "+styles.Muted.Render(m.entry.Description),
	)

	if len(m.entry.Meta) > 0 {
		lines = append(lines, "  "+renderMeta(m.entry.Meta))
	}
	for _, detail := range compactStrings(m.entry.Details) {
		lines = append(lines, "  "+styles.PathText.Render(detail))
	}

	if note := actionHint(m.entry); note != "" {
		lines = append(lines, "", "  "+styles.InfoText.Render(note))
	}

	lines = append(lines, "", styles.SectionTitle.Render("◇ Actions"), "")
	for i, opt := range m.options {
		prefix := "    "
		label := styles.Muted.Render(opt.label)
		if i == m.cursor {
			prefix = styles.Highlight.Render("  ▸ ")
			label = styles.Selected.Render(opt.label)
		}
		lines = append(lines, prefix+label)
	}

	if len(m.options) == 0 {
		lines = append(lines, "  "+styles.Muted.Render("No actions available for this entry."))
	}

	lines = append(lines, "")
	if m.status != "" {
		lines = append(lines, "  "+m.statusStyle.Render(m.status))
	} else {
		lines = append(lines, styles.Muted.Render("  esc/b: back • enter: run action"))
	}

	content := strings.Join(lines, "\n")
	box := styles.Panel.Width(panelWidth).Render(content)
	return tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box))
}

func buildOptions(entry browser.Entry) []actionOption {
	options := []actionOption{{label: "Edit in $EDITOR", action: ActionEdit}}
	sourceTool := strings.TrimSpace(strings.ToLower(entry.Badge))
	migrateAllowed := canMigrate(entry)

	for _, tool := range []domain.Tool{domain.ToolCodex, domain.ToolGemini, domain.ToolClaude, domain.ToolOpenCode} {
		if sourceTool != "" && sourceTool == string(tool) {
			continue
		}
		if !migrateAllowed {
			continue
		}
		options = append(options, actionOption{
			label:  actionLabel(entry, tool),
			action: ActionMigrate,
			target: tool,
		})
	}
	return options
}

func canMigrate(entry browser.Entry) bool {
	switch raw := entry.Raw.(type) {
	case catalog.MCPEntry:
		return len(raw.Servers) > 0 && raw.Status != "broken"
	case catalog.SkillEntry:
		return true
	default:
		return false
	}
}

func actionLabel(entry browser.Entry, target domain.Tool) string {
	targetLabel := strings.ToUpper(string(target))
	switch raw := entry.Raw.(type) {
	case catalog.MCPEntry:
		serverCount := len(raw.Servers)
		if serverCount == 1 {
			return fmt.Sprintf("Import 1 MCP server to %s", targetLabel)
		}
		return fmt.Sprintf("Import %d MCP servers to %s", serverCount, targetLabel)
	case catalog.SkillEntry:
		return fmt.Sprintf("Install skill to %s", targetLabel)
	default:
		return fmt.Sprintf("Migrate to %s", targetLabel)
	}
}

func entryKind(entry browser.Entry) string {
	switch entry.Raw.(type) {
	case catalog.MCPEntry:
		return "MCP Import"
	case catalog.SkillEntry:
		return "Skill Install"
	default:
		return "Asset Actions"
	}
}

func actionHint(entry browser.Entry) string {
	switch raw := entry.Raw.(type) {
	case catalog.MCPEntry:
		if len(raw.Servers) == 0 {
			return "No importable MCP servers were detected in this config yet."
		}
		return fmt.Sprintf("Detected servers: %s", strings.Join(raw.Servers, ", "))
	case catalog.SkillEntry:
		if len(raw.Files) > 0 {
			return fmt.Sprintf("Bundle includes %d file(s).", len(raw.Files))
		}
	}
	return ""
}

func renderMeta(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range compactStrings(values) {
		parts = append(parts, styles.MetaTag(value))
	}
	return strings.Join(parts, " ")
}

func compactStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}