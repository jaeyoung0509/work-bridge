package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"sessionport/internal/capability"
	"sessionport/internal/detect"
	"sessionport/internal/domain"
	"sessionport/internal/inspect"
)

type Backend struct {
	Detect          func(context.Context) (detect.Report, error)
	Inspect         func(context.Context, string) (inspect.Report, error)
	Import          func(context.Context, string, string) (domain.SessionBundle, error)
	Doctor          func(context.Context, domain.SessionBundle, domain.Tool) (domain.CompatibilityReport, error)
	Export          func(context.Context, domain.SessionBundle, domain.Tool, string) (domain.ExportManifest, error)
	ScanSkills      func(context.Context) ([]SkillEntry, error)
	ScanMCP         func(context.Context) ([]MCPEntry, error)
	DefaultExportDir string
}

type SkillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Source      string `json:"source"`
}

type MCPEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Source  string `json:"source"`
	Status  string `json:"status"`
	Details string `json:"details"`
}

type viewKind int

const (
	viewProjects viewKind = iota
	viewSessions
	viewMCP
	viewSkills
	viewLogs
)

type snapshotMsg struct {
	detect detect.Report
	skills []SkillEntry
	mcp    []MCPEntry
}

type inspectMsg struct {
	tool   string
	report inspect.Report
}

type bundleMsg struct {
	bundle domain.SessionBundle
}

type exportMsg struct {
	manifest domain.ExportManifest
}

type errorMsg struct {
	err error
}

type Model struct {
	backend Backend
	ctx     context.Context

	width  int
	height int

	loading bool
	err     error

	activeView viewKind
	activeTool string
	toolIndex  int
	sessionIdx int
	targetIdx  int

	detect detect.Report
	skills []SkillEntry
	mcp    []MCPEntry

	inspectByTool map[string]inspect.Report
	bundle        *domain.SessionBundle
	doctorReport  *domain.CompatibilityReport
	exportManifest *domain.ExportManifest

	logs []string
}

func Run(ctx context.Context, backend Backend, stdout, stderr io.Writer) error {
	model := NewModel(ctx, backend)
	program := tea.NewProgram(model, tea.WithContext(ctx))
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(stderr, err)
		return err
	}
	return nil
}

func NewModel(ctx context.Context, backend Backend) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	return Model{
		backend:       backend,
		ctx:           ctx,
		activeView:    viewProjects,
		activeTool:    "codex",
		inspectByTool: map[string]inspect.Report{},
		logs:          []string{"workspace loaded"},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadSnapshotCmd(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			m.activeView = (m.activeView + 1) % 5
			return m, nil
		case "shift+tab":
			m.activeView = (m.activeView + 4) % 5
			return m, nil
		case "t":
			m.targetIdx = (m.targetIdx + 1) % 4
			return m, nil
		case "T":
			m.targetIdx = (m.targetIdx + 3) % 4
			return m, nil
		case "left":
			return m.prevTool()
		case "right":
			return m.nextTool()
		case "up":
			return m.moveSelection(-1), nil
		case "down":
			return m.moveSelection(1), nil
		case "enter":
			if m.activeView == viewSessions {
				return m, m.importSelectedCmd()
			}
			if m.activeView == viewProjects {
				return m, m.refreshSelectedToolCmd()
			}
		case "e":
			if m.bundle != nil {
				return m, m.exportSelectedCmd()
			}
		case "d":
			if m.bundle != nil {
				return m, m.doctorSelectedCmd()
			}
		case "r":
			return m, m.loadSnapshotCmd()
		}
	case snapshotMsg:
		m.detect = msg.detect
		m.skills = msg.skills
		m.mcp = msg.mcp
		m.loading = false
		if m.activeTool == "" || !m.toolAvailable(m.activeTool) {
			m.activeTool = m.firstAvailableTool()
		}
		if m.activeTool != "" {
			return m, m.inspectToolCmd(m.activeTool)
		}
	case inspectMsg:
		m.inspectByTool[msg.tool] = msg.report
		if m.activeTool == msg.tool {
			m.sessionIdx = clampIndex(m.sessionIdx, len(msg.report.Sessions))
		}
	case bundleMsg:
		b := msg.bundle
		m.bundle = &b
		m.logs = append(m.logs, fmt.Sprintf("imported %s:%s", b.SourceTool, b.SourceSessionID))
		return m, m.doctorSelectedCmd()
	case exportMsg:
		manifest := msg.manifest
		m.exportManifest = &manifest
		m.logs = append(m.logs, fmt.Sprintf("exported to %s", manifest.OutputDir))
	case errorMsg:
		m.err = msg.err
		m.loading = false
		m.logs = append(m.logs, "error: "+msg.err.Error())
	case bundleDoctorMsg:
		report := msg.report
		m.doctorReport = &report
		m.logs = append(m.logs, fmt.Sprintf("doctor target=%s compatible=%d partial=%d unsupported=%d", report.TargetTool, len(report.CompatibleFields), len(report.PartialFields), len(report.UnsupportedFields)))
	}

	return m, nil
}

func (m Model) View() tea.View {
	view := tea.NewView("")
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = "sessionport"
	if m.width == 0 {
		view.SetContent("loading sessionport workspace...")
		return view
	}

	header := m.renderHeader()
	columns := m.renderColumns()
	footer := m.renderFooter()
	view.SetContent(lipgloss.JoinVertical(lipgloss.Left, header, columns, footer))
	return view
}

func (m Model) renderHeader() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Render("sessionport")
	subtitle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("project/session/mcp/skills")
	if m.loading {
		subtitle = lipgloss.NewStyle().Foreground(lipgloss.Color("172")).Render("loading workspace snapshot...")
	}
	if m.err != nil {
		subtitle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.err.Error())
	}
	tool := m.activeTool
	if tool == "" {
		tool = "codex"
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, title, " ", subtitle, " ", lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Render("["+tool+"]"))
}

func (m Model) renderColumns() string {
	left := m.panel("Project", m.renderProjects())
	middle := m.panel("Sessions", m.renderSessions())
	right := m.panel("MCP", m.renderMCP())
	bottom := m.panel("Skills", m.renderSkills()) + "\n" + m.panel("Logs", m.renderLogs())

	top := lipgloss.JoinHorizontal(lipgloss.Top, left, middle, right)
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (m Model) panel(title, body string) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1)
	if m.isActivePane(title) {
		style = style.BorderForeground(lipgloss.Color("33"))
	}
	if body == "" {
		body = " "
	}
	return style.Width(m.panelWidth()).Render(title + "\n" + body)
}

func (m Model) panelWidth() int {
	if m.width <= 0 {
		return 32
	}
	return maxInt(28, (m.width/3)-6)
}

func (m Model) renderProjects() string {
	lines := []string{
		fmt.Sprintf("cwd: %s", shortPath(m.detect.CWD)),
		fmt.Sprintf("project: %s", shortPath(m.detect.ProjectRoot)),
	}
	if len(m.detect.Tools) > 0 {
		lines = append(lines, "")
		for _, tool := range m.detect.Tools {
			status := "missing"
			if tool.Installed {
				status = "ready"
			}
			lines = append(lines, fmt.Sprintf("%s: %s", strings.ToUpper(tool.Tool), status))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderSessions() string {
	report := m.currentInspect()
	if len(report.Sessions) == 0 {
		return "no sessions"
	}
	lines := make([]string, 0, len(report.Sessions)*2)
	for i, session := range report.Sessions {
		prefix := "  "
		if i == m.sessionIdx {
			prefix = "> "
		}
		title := session.Title
		if title == "" {
			title = session.ID
		}
		lines = append(lines, fmt.Sprintf("%s%s", prefix, title))
		lines = append(lines, fmt.Sprintf("   %s", shortPath(session.StoragePath)))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderMCP() string {
	if len(m.mcp) == 0 {
		return "no MCP profiles"
	}
	lines := make([]string, 0, len(m.mcp)*2)
	for _, item := range m.mcp {
		lines = append(lines, fmt.Sprintf("%s [%s]", item.Name, item.Status))
		if item.Path != "" {
			lines = append(lines, "  "+shortPath(item.Path))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderSkills() string {
	if len(m.skills) == 0 {
		return "no skills found"
	}
	lines := make([]string, 0, len(m.skills)*2)
	for _, skill := range m.skills {
		lines = append(lines, skill.Name)
		if skill.Description != "" {
			lines = append(lines, "  "+skill.Description)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderLogs() string {
	if len(m.logs) == 0 {
		return "idle"
	}
	start := 0
	if len(m.logs) > 5 {
		start = len(m.logs) - 5
	}
	return strings.Join(m.logs[start:], "\n")
}

func (m Model) renderFooter() string {
	current := m.currentInspect()
	session := ""
	if len(current.Sessions) > 0 && m.sessionIdx >= 0 && m.sessionIdx < len(current.Sessions) {
		session = current.Sessions[m.sessionIdx].ID
	}
	target := m.targetTool()
	bundle := "none"
	if m.bundle != nil {
		bundle = m.bundle.BundleID
	}
	return fmt.Sprintf("view=%s tool=%s session=%s target=%s bundle=%s", m.activeViewName(), m.activeTool, session, target, bundle)
}

func (m Model) loadSnapshotCmd() tea.Cmd {
	if m.backend.Detect == nil {
		return nil
	}
	return func() tea.Msg {
		detectReport, err := m.backend.Detect(m.ctx)
		if err != nil {
			return errorMsg{err: err}
		}
		skills := []SkillEntry{}
		if m.backend.ScanSkills != nil {
			skills, err = m.backend.ScanSkills(m.ctx)
			if err != nil {
				return errorMsg{err: err}
			}
		}
		mcp := []MCPEntry{}
		if m.backend.ScanMCP != nil {
			mcp, err = m.backend.ScanMCP(m.ctx)
			if err != nil {
				return errorMsg{err: err}
			}
		}
		return snapshotMsg{detect: detectReport, skills: skills, mcp: mcp}
	}
}

func (m Model) inspectToolCmd(tool string) tea.Cmd {
	if m.backend.Inspect == nil || tool == "" {
		return nil
	}
	return func() tea.Msg {
		report, err := m.backend.Inspect(m.ctx, tool)
		if err != nil {
			return errorMsg{err: err}
		}
		return inspectMsg{tool: tool, report: report}
	}
}

func (m Model) refreshSelectedToolCmd() tea.Cmd {
	return m.inspectToolCmd(m.activeTool)
}

func (m Model) importSelectedCmd() tea.Cmd {
	if m.backend.Import == nil {
		return nil
	}
	report := m.currentInspect()
	if len(report.Sessions) == 0 || m.sessionIdx < 0 || m.sessionIdx >= len(report.Sessions) {
		return nil
	}
	session := report.Sessions[m.sessionIdx]
	return func() tea.Msg {
		bundle, err := m.backend.Import(m.ctx, m.activeTool, session.ID)
		if err != nil {
			return errorMsg{err: err}
		}
		return bundleMsg{bundle: bundle}
	}
}

func (m Model) exportSelectedCmd() tea.Cmd {
	if m.backend.Export == nil || m.bundle == nil {
		return nil
	}
	target := m.targetTool()
	if target == "" {
		return nil
	}
	outDir := m.backend.DefaultExportDir
	if outDir == "" {
		outDir = filepath.Join(os.TempDir(), "sessionport-export")
	}
	return func() tea.Msg {
		manifest, err := m.backend.Export(m.ctx, *m.bundle, domain.Tool(target), outDir)
		if err != nil {
			return errorMsg{err: err}
		}
		return exportMsg{manifest: manifest}
	}
}

func (m Model) doctorSelectedCmd() tea.Cmd {
	if m.backend.Doctor == nil || m.bundle == nil {
		return nil
	}
	target := m.targetTool()
	if target == "" {
		return nil
	}
	return func() tea.Msg {
		report, err := m.backend.Doctor(m.ctx, *m.bundle, domain.Tool(target))
		if err != nil {
			return errorMsg{err: err}
		}
		return bundleDoctorMsg{report: report}
	}
}

type bundleDoctorMsg struct {
	report domain.CompatibilityReport
}

func (m Model) currentInspect() inspect.Report {
	if report, ok := m.inspectByTool[m.activeTool]; ok {
		return report
	}
	return inspect.Report{Tool: m.activeTool}
}

func (m Model) toolAvailable(tool string) bool {
	for _, item := range m.detect.Tools {
		if item.Tool == tool && item.Installed {
			return true
		}
	}
	return false
}

func (m Model) firstAvailableTool() string {
	order := []string{"codex", "gemini", "claude", "opencode"}
	for _, tool := range order {
		if m.toolAvailable(tool) {
			return tool
		}
	}
	return "codex"
}

func (m Model) moveSelection(delta int) Model {
	report := m.currentInspect()
	if len(report.Sessions) == 0 {
		return m
	}
	m.sessionIdx = clampIndex(m.sessionIdx+delta, len(report.Sessions))
	return m
}

func (m Model) prevTool() (Model, tea.Cmd) {
	order := []string{"codex", "gemini", "claude", "opencode"}
	if len(order) == 0 {
		return m, nil
	}
	idx := indexOf(order, m.activeTool)
	if idx < 0 {
		idx = 0
	}
	m.activeTool = order[(idx+len(order)-1)%len(order)]
	m.sessionIdx = 0
	return m, m.inspectToolCmd(m.activeTool)
}

func (m Model) nextTool() (Model, tea.Cmd) {
	order := []string{"codex", "gemini", "claude", "opencode"}
	idx := indexOf(order, m.activeTool)
	if idx < 0 {
		idx = 0
	}
	m.activeTool = order[(idx+1)%len(order)]
	m.sessionIdx = 0
	return m, m.inspectToolCmd(m.activeTool)
}

func (m Model) activeViewName() string {
	switch m.activeView {
	case viewProjects:
		return "projects"
	case viewSessions:
		return "sessions"
	case viewMCP:
		return "mcp"
	case viewSkills:
		return "skills"
	default:
		return "logs"
	}
}

func (m Model) targetTool() string {
	order := []string{"codex", "gemini", "claude", "opencode"}
	if len(order) == 0 {
		return "claude"
	}
	if m.targetIdx < 0 || m.targetIdx >= len(order) {
		return order[0]
	}
	return order[m.targetIdx]
}

func (m Model) isActivePane(title string) bool {
	switch title {
	case "Project":
		return m.activeView == viewProjects
	case "Sessions":
		return m.activeView == viewSessions
	case "MCP":
		return m.activeView == viewMCP
	case "Skills":
		return m.activeView == viewSkills
	case "Logs":
		return m.activeView == viewLogs
	default:
		return false
	}
}

func clampIndex(idx int, size int) int {
	if size <= 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= size {
		return size - 1
	}
	return idx
}

func indexOf(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}

func shortPath(path string) string {
	if path == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func BuildMCPProfiles() []MCPEntry {
	profiles := []MCPEntry{}
	for _, tool := range []domain.Tool{domain.ToolCodex, domain.ToolGemini, domain.ToolClaude, domain.ToolOpenCode} {
		profile, err := capability.ProfileFor(tool, domain.AssetKindSession)
		if err != nil {
			continue
		}
		profiles = append(profiles, MCPEntry{
			Name:    string(tool),
			Status:  "profile",
			Details: strings.Join(profile.GeneratedArtifacts, ", "),
		})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles
}

func DefaultScanMCP() []MCPEntry {
	return BuildMCPProfiles()
}

func DefaultExportDir() string {
	return filepath.Join(os.TempDir(), "sessionport-export")
}

func RenderBundleJSON(bundle domain.SessionBundle) string {
	data, _ := json.MarshalIndent(bundle, "", "  ")
	return string(data)
}
