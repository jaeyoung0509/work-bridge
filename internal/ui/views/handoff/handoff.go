package handoff

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
)

// ─── Messages ───────────────────────────────────────────────

type BackMsg struct{}

type ApplyRequestMsg struct {
	Request switcher.Request
}

type ExportRequestMsg struct {
	Request  switcher.Request
	ExportPath string
}

// ─── Model ──────────────────────────────────────────────────

type optionRow string

const (
	rowTarget   optionRow = "target"
	rowAdvanced optionRow = "advanced"
	rowMode     optionRow = "mode"
	rowScope    optionRow = "scope"
	rowSkills   optionRow = "skills"
	rowMCP      optionRow = "mcp"
	rowApply    optionRow = "apply"
	rowExport   optionRow = "export"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayConfirm
	overlayResult
)

var supportedTools = []domain.Tool{
	domain.ToolCodex,
	domain.ToolGemini,
	domain.ToolClaude,
	domain.ToolOpenCode,
}

type Model struct {
	session       switcher.WorkspaceItem
	projectRoot   string
	defaultExport string

	target        domain.Tool
	mode          domain.SwitchMode
	includeSkills bool
	includeMCP    bool
	sessionOnly   bool
	showAdvanced  bool
	optionCursor  int

	preview       *switcher.Result
	lastResult    *switcher.Result
	lastErr       error
	running       bool

	overlay       overlayKind
	confirmAction string // "apply" or "export"
	confirmInput  string
	confirmCursor int

	width  int
	height int
}

func New(session switcher.WorkspaceItem, projectRoot string, defaultExport string) Model {
	return Model{
		session:       session,
		projectRoot:   projectRoot,
		defaultExport: defaultExport,
		target:        defaultTargetFor(session.Tool),
		mode:          domain.SwitchModeProject,
		includeSkills: true,
		includeMCP:    true,
	}
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *Model) SetPreview(r *switcher.Result) {
	m.preview = r
	m.running = false
}

func (m *Model) SetResult(r *switcher.Result, err error) {
	m.lastResult = r
	m.lastErr = err
	m.running = false
	if err == nil && r != nil {
		m.overlay = overlayResult
	}
}

func (m *Model) SetRunning(v bool) { m.running = v }

func (m Model) BuildRequest() switcher.Request {
	req := switcher.Request{
		From:          m.session.Tool,
		Session:       m.session.ID,
		To:            m.target,
		ProjectRoot:   m.projectRoot,
		Mode:          m.mode,
		IncludeSkills: m.includeSkills,
		IncludeMCP:    m.includeMCP,
	}
	if m.sessionOnly {
		req.IncludeSkills = false
		req.IncludeMCP = false
	}
	return req
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.overlay == overlayConfirm {
			return m.updateConfirm(msg)
		}
		if m.overlay == overlayResult {
			return m.updateResult(msg)
		}
		return m.updateOptions(msg)
	case tea.MouseClickMsg:
		if msg.Mouse().Button == tea.MouseLeft {
			return m.handleClick(msg)
		}
	}
	return m, nil
}

func (m Model) updateOptions(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	rows := m.optionRows()
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return BackMsg{} }
	case "up":
		if m.optionCursor > 0 {
			m.optionCursor--
		}
	case "down":
		if m.optionCursor < len(rows)-1 {
			m.optionCursor++
		}
	case "left":
		m.adjustOption(rows[m.optionCursor], -1)
	case "right", " ":
		m.adjustOption(rows[m.optionCursor], 1)
	case "enter":
		row := rows[m.optionCursor]
		if row == rowApply {
			if m.preview == nil || m.lastErr != nil {
				m.lastErr = fmt.Errorf("preview not ready")
				return m, nil
			}
			m.confirmAction = "apply"
			m.overlay = overlayConfirm
		} else if row == rowExport {
			if m.preview == nil || m.lastErr != nil {
				m.lastErr = fmt.Errorf("preview not ready")
				return m, nil
			}
			m.confirmAction = "export"
			m.confirmInput = m.defaultExportPath()
			m.confirmCursor = len([]rune(m.confirmInput))
			m.overlay = overlayConfirm
		} else {
			m.adjustOption(row, 1)
		}
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.confirmAction == "export" {
		if handled := m.handleConfirmText(msg); handled {
			return m, nil
		}
	}
	switch msg.String() {
	case "esc":
		m.overlay = overlayNone
		m.lastErr = nil
	case "enter":
		req := m.BuildRequest()
		m.overlay = overlayNone
		m.running = true
		if m.confirmAction == "apply" {
			return m, func() tea.Msg { return ApplyRequestMsg{Request: req} }
		}
		if strings.TrimSpace(m.confirmInput) == "" {
			m.lastErr = fmt.Errorf("export path is required")
			m.running = false
			return m, nil
		}
		return m, func() tea.Msg {
			return ExportRequestMsg{Request: req, ExportPath: m.confirmInput}
		}
	}
	return m, nil
}

func (m Model) updateResult(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.overlay = overlayNone
		m.lastErr = nil
	case "b":
		return m, func() tea.Msg { return BackMsg{} }
	}
	return m, nil
}

func (m Model) handleClick(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	// Simple click regions for Apply/Export buttons
	return m, nil
}

func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("")
	}

	leftW := m.width / 2
	rightW := m.width - leftW - 2

	leftContent := m.renderOptions(leftW)
	rightContent := m.renderPreview(rightW)

	leftPane := styles.ActivePane.Width(leftW).Height(m.height - 4).Render(leftContent)
	rightPane := styles.InactivePane.Width(rightW).Height(m.height - 4).Render(rightContent)

	main := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	if m.overlay != overlayNone {
		main = lipgloss.JoinVertical(lipgloss.Left, main, "\n"+m.renderOverlay())
	}

	if m.lastErr != nil && m.overlay == overlayNone {
		main = lipgloss.JoinVertical(lipgloss.Left, main, "\n"+styles.ErrorBox.Width(m.width-4).Render(m.lastErr.Error()))
	}

	return tea.NewView(main)
}

func (m Model) renderOptions(width int) string {
	var lines []string
	lines = append(lines,
		styles.SectionTitle.Render("◈ Handoff Configuration"),
		"",
		styles.Muted.Render(fmt.Sprintf("  Source: %s  %s", styles.ToolBadgeFor(string(m.session.Tool)), m.session.Title)),
		"",
	)

	lines = append(lines,
		m.renderRow(rowTarget, "Target Tool", string(m.target), false),
		m.renderRow(rowAdvanced, "Advanced", onOff(m.showAdvanced), false),
	)
	if m.showAdvanced {
		lines = append(lines,
			m.renderRow(rowMode, "Mode", string(m.mode), false),
			m.renderRow(rowScope, "Scope", scopeLabel(m.sessionOnly), false),
			m.renderRow(rowSkills, "Skills", onOff(m.includeSkills), m.sessionOnly),
			m.renderRow(rowMCP, "MCP", onOff(m.includeMCP), m.sessionOnly),
		)
	}
	lines = append(lines, "")

	applyStyle := styles.ButtonSecondary
	exportStyle := styles.ButtonSecondary
	rows := m.optionRows()
	if rows[m.optionCursor] == rowApply {
		applyStyle = styles.ButtonActive
	}
	if rows[m.optionCursor] == rowExport {
		exportStyle = styles.ButtonActive
	}
	lines = append(lines,
		"  "+applyStyle.Render("  ▶ Apply Handoff  ")+"  "+exportStyle.Render("  ↗ Export  "),
	)

	if m.running {
		lines = append(lines, "", styles.WarningText.Render("  ⟳ Processing..."))
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderPreview(width int) string {
	var lines []string
	lines = append(lines, styles.SectionTitle.Render("◇ Preview"))

	if m.preview == nil {
		lines = append(lines, "", styles.Muted.Render("  Configure options and preview will auto-load."))
		return strings.Join(lines, "\n")
	}

	plan := m.preview.Plan
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  Overall:  %s", styles.Status(plan.Status)))
	lines = append(lines, fmt.Sprintf("  Session:  %s  %s", styles.Status(plan.Session.State), plan.Session.Summary))
	lines = append(lines, fmt.Sprintf("  Skills:   %s  %s", styles.Status(plan.Skills.State), plan.Skills.Summary))
	lines = append(lines, fmt.Sprintf("  MCP:      %s  %s", styles.Status(plan.MCP.State), plan.MCP.Summary))

	if len(plan.PlannedFiles) > 0 {
		lines = append(lines, "")
		lines = append(lines, styles.Muted.Render(fmt.Sprintf("  %d files will be updated", len(plan.PlannedFiles))))
	}

	if len(plan.Warnings) > 0 {
		lines = append(lines, "")
		lines = append(lines, styles.WarningText.Render(fmt.Sprintf("  ⚠ %d warnings", len(plan.Warnings))))
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderOverlay() string {
	switch m.overlay {
	case overlayConfirm:
		lines := []string{
			styles.Title.Render("Confirm " + strings.Title(m.confirmAction)),
			"",
			fmt.Sprintf("  Target:  %s", styles.ToolBadgeFor(string(m.target))),
			fmt.Sprintf("  Mode:    %s", m.mode),
		}
		if m.confirmAction == "export" {
			lines = append(lines, "", "  Export Path:")
			lines = append(lines, "  "+styles.InputBox.Render(m.confirmInputWithCursor()))
		}
		lines = append(lines, "", styles.Muted.Render("  enter: confirm • esc: cancel"))
		return styles.Overlay.Width(m.width - 12).Render(strings.Join(lines, "\n"))

	case overlayResult:
		var lines []string
		if m.lastErr != nil {
			lines = []string{styles.Title.Render("Action Failed"), "", "  " + m.lastErr.Error()}
		} else if m.lastResult != nil && m.lastResult.Report != nil {
			r := m.lastResult.Report
			lines = []string{
				styles.Title.Render("✓ Action Complete"),
				"",
				fmt.Sprintf("  Status:       %s", styles.Status(r.Status)),
				fmt.Sprintf("  Destination:  %s", r.DestinationRoot),
				fmt.Sprintf("  Files:        %d updated", len(r.FilesUpdated)),
			}
			if len(r.Errors) > 0 {
				lines = append(lines, styles.ErrorText.Render(fmt.Sprintf("  %d errors", len(r.Errors))))
			}
		} else {
			lines = []string{"  Unknown result state"}
		}
		lines = append(lines, "", styles.Muted.Render("  esc/enter: close • b: back to sessions"))
		return styles.Overlay.Width(m.width - 12).Render(strings.Join(lines, "\n"))
	}
	return ""
}

// ─── Helpers ────────────────────────────────────────────────

func (m Model) optionRows() []optionRow {
	rows := []optionRow{rowTarget, rowAdvanced}
	if m.showAdvanced {
		rows = append(rows, rowMode, rowScope, rowSkills, rowMCP)
	}
	rows = append(rows, rowApply, rowExport)
	return rows
}

func (m *Model) adjustOption(row optionRow, direction int) {
	switch row {
	case rowTarget:
		m.target = cycleTool(m.target, direction)
	case rowAdvanced:
		m.showAdvanced = !m.showAdvanced
		rows := m.optionRows()
		if m.optionCursor >= len(rows) {
			m.optionCursor = len(rows) - 1
		}
	case rowMode:
		if m.mode == domain.SwitchModeProject {
			m.mode = domain.SwitchModeNative
		} else {
			m.mode = domain.SwitchModeProject
		}
	case rowScope:
		m.sessionOnly = !m.sessionOnly
		if m.sessionOnly {
			m.includeSkills = false
			m.includeMCP = false
		} else {
			m.includeSkills = true
			m.includeMCP = true
		}
	case rowSkills:
		if !m.sessionOnly {
			m.includeSkills = !m.includeSkills
		}
	case rowMCP:
		if !m.sessionOnly {
			m.includeMCP = !m.includeMCP
		}
	}
}

func (m Model) renderRow(row optionRow, label string, value string, disabled bool) string {
	current := m.optionRows()[m.optionCursor] == row
	prefix := "  "
	if current {
		prefix = styles.Highlight.Render("› ")
	}
	var line string
	if value == "" {
		line = fmt.Sprintf("%s%s", prefix, label)
	} else {
		line = fmt.Sprintf("%s%-14s %s", prefix, label, value)
	}
	if disabled {
		return styles.Disabled.Render(line)
	}
	if current {
		return styles.Selected.Render(line)
	}
	return line
}

func (m *Model) handleConfirmText(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "left":
		if m.confirmCursor > 0 {
			m.confirmCursor--
		}
		return true
	case "right":
		if m.confirmCursor < len([]rune(m.confirmInput)) {
			m.confirmCursor++
		}
		return true
	case "backspace":
		if m.confirmCursor == 0 {
			return true
		}
		runes := []rune(m.confirmInput)
		runes = append(runes[:m.confirmCursor-1], runes[m.confirmCursor:]...)
		m.confirmCursor--
		m.confirmInput = string(runes)
		return true
	}
	key := msg.Key()
	if key.Text == "" || key.Mod != 0 {
		return false
	}
	runes := []rune(m.confirmInput)
	insert := []rune(key.Text)
	head := append([]rune{}, runes[:m.confirmCursor]...)
	head = append(head, insert...)
	head = append(head, runes[m.confirmCursor:]...)
	m.confirmInput = string(head)
	m.confirmCursor += len(insert)
	return true
}

func (m Model) confirmInputWithCursor() string {
	runes := []rune(m.confirmInput)
	pos := m.confirmCursor
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	cursor := styles.Cursor.Render(" ")
	return string(runes[:pos]) + cursor + string(runes[pos:])
}

func (m Model) defaultExportPath() string {
	if strings.TrimSpace(m.defaultExport) != "" {
		return m.defaultExport
	}
	if m.projectRoot != "" {
		return m.projectRoot + "/.work-bridge/exports/" + string(m.target)
	}
	return ".work-bridge/exports/" + string(m.target)
}

func defaultTargetFor(source domain.Tool) domain.Tool {
	for _, tool := range supportedTools {
		if tool != source {
			return tool
		}
	}
	return source
}

func cycleTool(current domain.Tool, direction int) domain.Tool {
	index := 0
	for i, tool := range supportedTools {
		if tool == current {
			index = i
			break
		}
	}
	index = (index + direction + len(supportedTools)) % len(supportedTools)
	return supportedTools[index]
}

func onOff(v bool) string {
	if v {
		return styles.BadgeSuccess.Render("ON")
	}
	return styles.BadgeMuted.Render("OFF")
}

func scopeLabel(sessionOnly bool) string {
	if sessionOnly {
		return "session-only"
	}
	return "full"
}
