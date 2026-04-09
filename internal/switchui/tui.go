package switchui

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
)

type Backend struct {
	LoadWorkspace func(context.Context) (switcher.Workspace, error)
	Preview       func(context.Context, switcher.Request) (switcher.Result, error)
	Apply         func(context.Context, switcher.Request) (switcher.Result, error)
	Export        func(context.Context, switcher.Request, string) (switcher.Result, error)
}

type Model struct {
	ctx      context.Context
	backend  Backend
	workspace switcher.Workspace

	sessionIdx int
	targetIdx  int
	busy       string
	err        string
	help       bool

	preview *switcher.Result
	result  *switcher.Result
	activity []string
}

type workspaceLoadedMsg struct {
	workspace switcher.Workspace
	err       error
}

type previewReadyMsg struct {
	result switcher.Result
	err    error
}

type applyReadyMsg struct {
	result switcher.Result
	err    error
}

type exportReadyMsg struct {
	result switcher.Result
	err    error
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
		ctx:       ctx,
		backend:   backend,
		targetIdx: 0,
		activity:  []string{"boot requested"},
	}
}

func (m Model) Init() tea.Cmd {
	m.busy = "loading workspace"
	return m.loadCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case workspaceLoadedMsg:
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.workspace = msg.workspace
		m.err = ""
		m.preview = nil
		m.result = nil
		m.activity = append(m.activity, fmt.Sprintf("loaded %d sessions", len(msg.workspace.Sessions)))
		m.ensureSessionIndex()
		return m, nil
	case previewReadyMsg:
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.preview = &msg.result
		m.result = nil
		m.err = ""
		m.activity = append(m.activity, fmt.Sprintf("preview %s -> %s", msg.result.Payload.Bundle.SourceTool, msg.result.Plan.TargetTool))
		return m, nil
	case applyReadyMsg:
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.preview = &msg.result
		m.result = &msg.result
		m.err = ""
		m.activity = append(m.activity, fmt.Sprintf("applied %s -> %s", msg.result.Payload.Bundle.SourceTool, msg.result.Plan.TargetTool))
		return m, nil
	case exportReadyMsg:
		m.busy = ""
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.preview = &msg.result
		m.result = &msg.result
		m.err = ""
		m.activity = append(m.activity, fmt.Sprintf("exported %s -> %s", msg.result.Payload.Bundle.SourceTool, msg.result.Plan.TargetTool))
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		return m, nil
	}
}

func (m Model) View() tea.View {
	var b strings.Builder
	fmt.Fprintln(&b, "work-bridge")
	fmt.Fprintf(&b, "project: %s\n", fallback(m.workspace.ProjectRoot, "n/a"))
	fmt.Fprintf(&b, "scope: current-project | target: %s", m.targetTool())
	if m.busy != "" {
		fmt.Fprintf(&b, " | %s", m.busy)
	}
	if m.err != "" {
		fmt.Fprintf(&b, " | error: %s", m.err)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b)

	b.WriteString(m.renderSessions())
	fmt.Fprintln(&b)
	fmt.Fprintln(&b)
	b.WriteString(m.renderPreview())
	fmt.Fprintln(&b)
	fmt.Fprintln(&b)
	b.WriteString(m.renderResult())
	fmt.Fprintln(&b)
	fmt.Fprintln(&b)
	if m.help {
		fmt.Fprintln(&b, "keys: ↑/↓ or j/k move | ←/→ or h/l target | enter preview | a apply | e export | r refresh | ? help | q quit")
	} else {
		fmt.Fprintln(&b, "keys: enter preview | a apply | e export | r refresh | ? help | q quit")
	}
	return tea.NewView(b.String())
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.help = !m.help
		return m, nil
	case "r":
		m.busy = "refreshing workspace"
		return m, m.loadCmd()
	case "up", "k":
		if m.sessionIdx > 0 {
			m.sessionIdx--
		}
		return m, nil
	case "down", "j":
		if m.sessionIdx < len(m.workspace.Sessions)-1 {
			m.sessionIdx++
		}
		return m, nil
	case "left", "h":
		if m.targetIdx > 0 {
			m.targetIdx--
		} else {
			m.targetIdx = len(targetOrder) - 1
		}
		m.preview = nil
		m.result = nil
		return m, nil
	case "right", "l", "tab":
		if len(targetOrder) > 0 {
		m.targetIdx = (m.targetIdx + 1) % len(targetOrder)
		}
		m.preview = nil
		m.result = nil
		return m, nil
	case "enter":
		if m.selectedSession() == nil {
			return m, nil
		}
		m.busy = "building preview"
		return m, m.previewCmd()
	case "a":
		if m.selectedSession() == nil {
			return m, nil
		}
		m.busy = "applying switch"
		return m, m.applyCmd()
	case "e":
		if m.selectedSession() == nil {
			return m, nil
		}
		m.busy = "exporting handoff"
		return m, m.exportCmd()
	default:
		return m, nil
	}
}

func (m Model) loadCmd() tea.Cmd {
	return func() tea.Msg {
		if m.backend.LoadWorkspace == nil {
			return workspaceLoadedMsg{err: fmt.Errorf("load workspace backend is required")}
		}
		workspace, err := m.backend.LoadWorkspace(m.ctx)
		return workspaceLoadedMsg{workspace: workspace, err: err}
	}
}

func (m Model) previewCmd() tea.Cmd {
	return func() tea.Msg {
		if m.backend.Preview == nil {
			return previewReadyMsg{err: fmt.Errorf("preview backend is required")}
		}
		result, err := m.backend.Preview(m.ctx, m.currentRequest())
		return previewReadyMsg{result: result, err: err}
	}
}

func (m Model) applyCmd() tea.Cmd {
	return func() tea.Msg {
	if m.backend.Apply == nil {
			return applyReadyMsg{err: fmt.Errorf("apply backend is required")}
		}
		result, err := m.backend.Apply(m.ctx, m.currentRequest())
		return applyReadyMsg{result: result, err: err}
	}
}

func (m Model) exportCmd() tea.Cmd {
	return func() tea.Msg {
		if m.backend.Export == nil {
			return exportReadyMsg{err: fmt.Errorf("export backend is required")}
		}
		result, err := m.backend.Export(m.ctx, m.currentRequest(), m.exportRoot())
		return exportReadyMsg{result: result, err: err}
	}
}

func (m Model) currentRequest() switcher.Request {
	session := m.selectedSession()
	req := switcher.Request{
		From:          session.Tool,
		Session:       session.ID,
		To:            m.targetTool(),
		ProjectRoot:   m.workspace.ProjectRoot,
		IncludeSkills: true,
		IncludeMCP:    true,
	}
	return req
}

func (m Model) selectedSession() *switcher.WorkspaceItem {
	if len(m.workspace.Sessions) == 0 {
		return nil
	}
	if m.sessionIdx < 0 || m.sessionIdx >= len(m.workspace.Sessions) {
		return nil
	}
	return &m.workspace.Sessions[m.sessionIdx]
}

func (m *Model) ensureSessionIndex() {
	if len(m.workspace.Sessions) == 0 {
		m.sessionIdx = 0
		return
	}
	if m.sessionIdx >= len(m.workspace.Sessions) {
		m.sessionIdx = len(m.workspace.Sessions) - 1
	}
	if m.sessionIdx < 0 {
		m.sessionIdx = 0
	}
}

func (m Model) targetTool() domain.Tool {
	if len(targetOrder) == 0 {
		return domain.ToolCodex
	}
	return targetOrder[m.targetIdx%len(targetOrder)]
}

func (m Model) renderSessions() string {
	var b strings.Builder
	fmt.Fprintln(&b, "Sessions")
	if len(m.workspace.Sessions) == 0 {
		fmt.Fprintln(&b, "  no sessions for current project")
		return b.String()
	}
	for i, session := range m.workspace.Sessions {
		cursor := " "
		if i == m.sessionIdx {
			cursor = ">"
		}
		fmt.Fprintf(&b, "%s [%s] %s\n", cursor, strings.ToUpper(string(session.Tool)), fallback(session.Title, session.ID))
		if session.ProjectRoot != "" {
			fmt.Fprintf(&b, "    %s\n", session.ProjectRoot)
		}
	}
	return b.String()
}

func (m Model) renderPreview() string {
	var b strings.Builder
	fmt.Fprintln(&b, "Preview")
	if m.preview == nil {
		fmt.Fprintln(&b, "  select a session and press Enter")
		return b.String()
	}
	fmt.Fprintf(&b, "  status: %s\n", m.preview.Plan.Status)
	fmt.Fprintf(&b, "  session: %s\n", m.preview.Plan.Session.State)
	fmt.Fprintf(&b, "  skills: %s\n", m.preview.Plan.Skills.State)
	fmt.Fprintf(&b, "  mcp: %s\n", m.preview.Plan.MCP.State)
	fmt.Fprintf(&b, "  managed root: %s\n", m.preview.Plan.ManagedRoot)
	if len(m.preview.Plan.Warnings) > 0 {
		fmt.Fprintln(&b, "  warnings:")
		for _, warning := range m.preview.Plan.Warnings {
			fmt.Fprintf(&b, "    - %s\n", warning)
		}
	}
	fmt.Fprintln(&b, "  files:")
	for _, file := range m.preview.Plan.PlannedFiles {
		fmt.Fprintf(&b, "    - [%s] %s (%s)\n", file.Section, file.Path, file.Action)
	}
	return b.String()
}

func (m Model) renderResult() string {
	var b strings.Builder
	fmt.Fprintln(&b, "Result")
	if m.result != nil && m.result.Report != nil {
		fmt.Fprintf(&b, "  mode: %s\n", m.result.Report.AppliedMode)
		fmt.Fprintf(&b, "  status: %s\n", m.result.Report.Status)
		for _, file := range m.result.Report.FilesUpdated {
			fmt.Fprintf(&b, "    - updated %s\n", file)
		}
		for _, file := range m.result.Report.BackupsCreated {
			fmt.Fprintf(&b, "    - backup %s\n", file)
		}
		if len(m.result.Report.Warnings) > 0 {
			fmt.Fprintln(&b, "  warnings:")
			for _, warning := range m.result.Report.Warnings {
				fmt.Fprintf(&b, "    - %s\n", warning)
			}
		}
		return b.String()
	}
	fmt.Fprintln(&b, "  no apply yet")
	if len(m.activity) > 0 {
		fmt.Fprintln(&b, "  recent:")
		start := 0
		if len(m.activity) > 5 {
			start = len(m.activity) - 5
		}
		for _, item := range m.activity[start:] {
			fmt.Fprintf(&b, "    - %s\n", item)
		}
	}
	return b.String()
}

func fallback(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var targetOrder = []domain.Tool{
	domain.ToolCodex,
	domain.ToolGemini,
	domain.ToolClaude,
	domain.ToolOpenCode,
}

func (m Model) exportRoot() string {
	return filepath.Join(m.workspace.ProjectRoot, ".work-bridge", "exports", string(m.targetTool()))
}
