package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jaeyoung0509/work-bridge/internal/catalog"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/browser"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/session"
)

type Backend interface {
	LoadWorkspace(ctx context.Context, projectRoot string) (switcher.Workspace, error)
	LoadProjects(ctx context.Context, roots []string) ([]catalog.ProjectEntry, error)
	LoadSkills(ctx context.Context, projectRoot string) ([]catalog.SkillEntry, error)
	LoadMCP(ctx context.Context, projectRoot string) ([]catalog.MCPEntry, error)
	Preview(ctx context.Context, req switcher.Request) (switcher.Result, error)
	Apply(ctx context.Context, req switcher.Request) (switcher.Result, error)
	Export(ctx context.Context, req switcher.Request, outDir string) (switcher.Result, error)
}

type Options struct {
	ProjectRoot      string
	WorkspaceRoots   []string
	DefaultExportDir string
}

type AppState int

const (
	StateSelectSession AppState = iota // Step 1: Select a source session
	StateSelectTarget                  // Step 2: Select the target tool and advanced options
	StatePreview                       // Step 3: Review the planned handoff operations
	StateConfirm                       // Step 4: Confirm action or input export path
	StateResult                        // Step 5: Display the action summary report
	StateProjects                      // Browser: Select an active project from the workspace
	StateSkills                        // Browser: View available skills
	StateMCP                           // Browser: View MCP server configurations
)

type actionKind int

const (
	actionNone actionKind = iota
	actionLoadWorkspace
	actionPreview
	actionApply
	actionExport
	actionLoadProjects
	actionLoadSkills
	actionLoadMCP
)

type optionRow string

const (
	optionRowTarget   optionRow = "target"
	optionRowAdvanced optionRow = "advanced"
	optionRowMode     optionRow = "mode"
	optionRowScope    optionRow = "scope"
	optionRowSkills   optionRow = "skills"
	optionRowMCP      optionRow = "mcp"
	optionRowContinue optionRow = "continue"
)

var supportedTools = []domain.Tool{
	domain.ToolCodex,
	domain.ToolGemini,
	domain.ToolClaude,
	domain.ToolOpenCode,
}

// MainModel serves as the root router for the Bubble Tea application.
// It manages the overall state machine, data fetching (Backend interactions), and delegates rendering to nested views.
type MainModel struct {
	ctx             context.Context
	backend         Backend
	options         Options
	state           AppState // Tracks the current step in the migration wizard or browser view.

	sessionView     session.Model // Renders the list of sessions available for handoff.
	workspace       switcher.Workspace
	selectedSession *switcher.WorkspaceItem

	// Wizard selections
	target          domain.Tool
	mode            domain.SwitchMode
	includeSkills   bool
	includeMCP      bool
	sessionOnly     bool
	showAdvanced    bool
	optionCursor    int

	lastPreview     *switcher.Result // Holds the dry-run handoff plan.
	lastResult      *switcher.Result // Holds the final result report after an apply/export.
	lastErr         error            // Tracks any errors that occurred in the latest action.
	running         actionKind       // Tracks long-running background tasks (e.g., loading previews).

	confirmAction   actionKind
	confirmInput    string
	confirmCursor   int

	showHelp        bool
	commandActive   bool   // True when the user is typing a slash command (e.g., /projects).
	commandInput    string
	commandCursor   int

	browserView     browser.Model // Reusable list view for exploring Projects, Skills, and MCP configs.
	browserReturn   AppState      // State to return to when exiting the browser view.
	projects        []catalog.ProjectEntry
	skills          []catalog.SkillEntry
	mcpEntries      []catalog.MCPEntry

	quitting        bool
	width           int
	height          int
}

type workspaceLoadedMsg struct {
	workspace switcher.Workspace
	err       error
}

type previewLoadedMsg struct {
	result switcher.Result
	err    error
}

type actionFinishedMsg struct {
	action actionKind
	result switcher.Result
	err    error
}

type projectsLoadedMsg struct {
	entries []catalog.ProjectEntry
	err     error
}

type skillsLoadedMsg struct {
	entries []catalog.SkillEntry
	err     error
}

type mcpLoadedMsg struct {
	entries []catalog.MCPEntry
	err     error
}

func NewMainModel(ctx context.Context, backend Backend, opts Options) MainModel {
	return MainModel{
		ctx:           ctx,
		backend:       backend,
		options:       opts,
		state:         StateSelectSession,
		sessionView:   session.NewModel(),
		browserView:   browser.NewModel("Browser"),
		running:       actionLoadWorkspace,
		mode:          domain.SwitchModeProject,
		includeSkills: true,
		includeMCP:    true,
	}
}

func (m MainModel) Init() tea.Cmd {
	return m.loadWorkspaceCmd(m.options.ProjectRoot)
}

func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		updated, cmd := m.sessionView.Update(msg)
		m.sessionView = updated.(session.Model)
		m.browserView.SetSize(msg.Width, msg.Height)
		return m, cmd

	case tea.KeyPressMsg:
		if m.commandActive {
			if handled, model, cmd := m.handleCommandKey(msg); handled {
				return model, cmd
			}
		}
		if handled, model, cmd := m.handleGlobalKey(msg); handled {
			return model, cmd
		}

	case workspaceLoadedMsg:
		m.running = actionNone
		m.workspace = msg.workspace
		m.options.ProjectRoot = msg.workspace.ProjectRoot
		m.lastErr = msg.err
		if msg.err == nil {
			m.sessionView.SetSessions(msg.workspace.Sessions)
		}
		return m, nil

	case previewLoadedMsg:
		m.running = actionNone
		m.state = StatePreview
		m.lastErr = msg.err
		if msg.err == nil {
			m.lastPreview = &msg.result
			m.lastResult = nil
		}
		return m, nil

	case actionFinishedMsg:
		m.running = actionNone
		m.state = StateResult
		m.confirmAction = actionNone
		m.lastErr = msg.err
		if msg.err == nil {
			m.lastResult = &msg.result
		} else {
			m.lastResult = nil
		}
		return m, nil

	case session.SessionSelectedMsg:
		m.selectSession(msg.Session)
		return m, nil

	case projectsLoadedMsg:
		m.running = actionNone
		m.projects = msg.entries
		m.lastErr = msg.err
		if msg.err == nil {
			m.state = StateProjects
			m.browserView.SetTitle("Projects")
			m.browserView.SetEntries(projectEntries(msg.entries))
		}
		return m, nil

	case skillsLoadedMsg:
		m.running = actionNone
		m.skills = msg.entries
		m.lastErr = msg.err
		if msg.err == nil {
			m.state = StateSkills
			m.browserView.SetTitle("Skills")
			m.browserView.SetEntries(skillEntries(msg.entries))
		}
		return m, nil

	case mcpLoadedMsg:
		m.running = actionNone
		m.mcpEntries = msg.entries
		m.lastErr = msg.err
		if msg.err == nil {
			m.state = StateMCP
			m.browserView.SetTitle("MCP")
			m.browserView.SetEntries(mcpEntries(msg.entries))
		}
		return m, nil

	case browser.SelectedMsg:
		return m.handleBrowserSelection(msg.Entry)
	}

	switch m.state {
	case StateSelectSession:
		return m.updateSessionState(msg)
	case StateSelectTarget:
		return m.updateTargetState(msg)
	case StatePreview:
		return m.updatePreviewState(msg)
	case StateConfirm:
		return m.updateConfirmState(msg)
	case StateResult:
		return m.updateResultState(msg)
	case StateProjects, StateSkills, StateMCP:
		return m.updateBrowserState(msg)
	default:
		return m, nil
	}
}

func (m MainModel) View() tea.View {
	if m.quitting {
		return tea.NewView("Goodbye!\n")
	}

	var content string
	switch {
	case m.running != actionNone:
		content = m.renderBusyView()
	default:
		content = strings.Join([]string{
			m.renderHeader(),
			m.renderCurrentState(),
			m.renderFooter(),
		}, "\n\n")
		if m.showHelp {
			content = strings.Join([]string{content, styles.HelpBox.Render(m.renderHelp())}, "\n\n")
		}
		if m.commandActive {
			content = strings.Join([]string{content, m.renderCommandPalette()}, "\n\n")
		}
	}

	view := tea.NewView(styles.AppContainer.Render(content))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeAllMotion
	return view
}

func (m MainModel) handleGlobalKey(msg tea.KeyPressMsg) (bool, tea.Model, tea.Cmd) {
	if key := msg.Key(); key.Mod == 0 && strings.HasPrefix(key.Text, "/") {
		m.commandActive = true
		m.commandInput = key.Text
		m.commandCursor = len([]rune(m.commandInput))
		m.lastErr = nil
		return true, m, nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return true, m, tea.Quit
	case "?":
		if m.running != actionNone {
			return true, m, nil
		}
		m.showHelp = !m.showHelp
		return true, m, nil
	}
	return false, m, nil
}

func (m MainModel) handleCommandKey(msg tea.KeyPressMsg) (bool, tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.commandActive = false
		m.commandInput = ""
		m.commandCursor = 0
		return true, m, nil
	case "enter":
		commandInput := m.commandInput
		m.commandActive = false
		m.commandInput = ""
		m.commandCursor = 0
		fields := strings.Fields(strings.TrimSpace(commandInput))
		if len(fields) == 0 {
			m.lastErr = fmt.Errorf("slash command is required")
			return true, m, nil
		}
		return true, m, m.dispatchCommand(strings.ToLower(fields[0]))
	case "left":
		if m.commandCursor > 0 {
			m.commandCursor--
		}
		return true, m, nil
	case "right":
		if m.commandCursor < len([]rune(m.commandInput)) {
			m.commandCursor++
		}
		return true, m, nil
	case "home":
		m.commandCursor = 0
		return true, m, nil
	case "end":
		m.commandCursor = len([]rune(m.commandInput))
		return true, m, nil
	case "backspace":
		if m.commandCursor == 0 {
			return true, m, nil
		}
		runes := []rune(m.commandInput)
		runes = append(runes[:m.commandCursor-1], runes[m.commandCursor:]...)
		m.commandCursor--
		m.commandInput = string(runes)
		return true, m, nil
	case "delete":
		runes := []rune(m.commandInput)
		if m.commandCursor >= len(runes) {
			return true, m, nil
		}
		runes = append(runes[:m.commandCursor], runes[m.commandCursor+1:]...)
		m.commandInput = string(runes)
		return true, m, nil
	}

	key := msg.Key()
	if key.Text == "" || key.Mod != 0 {
		return true, m, nil
	}
	runes := []rune(m.commandInput)
	insert := []rune(key.Text)
	head := append([]rune{}, runes[:m.commandCursor]...)
	head = append(head, insert...)
	head = append(head, runes[m.commandCursor:]...)
	m.commandInput = string(head)
	m.commandCursor += len(insert)
	return true, m, nil
}

func (m *MainModel) dispatchCommand(command string) tea.Cmd {
	if command == "" {
		m.lastErr = fmt.Errorf("slash command is required")
		return nil
	}

	m.browserReturn = m.state
	m.lastErr = nil
	switch command {
	case "/projects":
		m.running = actionLoadProjects
		return m.loadProjectsCmd()
	case "/skills":
		m.running = actionLoadSkills
		return m.loadSkillsCmd(m.projectRoot())
	case "/mcp":
		m.running = actionLoadMCP
		return m.loadMCPCmd(m.projectRoot())
	default:
		m.lastErr = fmt.Errorf("unknown slash command %q", command)
		return nil
	}
}

func (m MainModel) updateSessionState(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == "r" {
		m.running = actionLoadWorkspace
		m.lastErr = nil
		return m, m.loadWorkspaceCmd(m.projectRoot())
	}
	updated, cmd := m.sessionView.Update(msg)
	m.sessionView = updated.(session.Model)
	return m, cmd
}

func (m MainModel) updateTargetState(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	rows := m.optionRows()
	switch keyMsg.String() {
	case "up", "k":
		if m.optionCursor > 0 {
			m.optionCursor--
		}
		return m, nil
	case "down", "j":
		if m.optionCursor < len(rows)-1 {
			m.optionCursor++
		}
		return m, nil
	case "left", "h":
		m.adjustOption(rows[m.optionCursor], -1)
		return m, nil
	case "right", "l", " ":
		m.adjustOption(rows[m.optionCursor], 1)
		return m, nil
	case "esc":
		m.state = StateSelectSession
		m.lastErr = nil
		return m, nil
	case "enter":
		if rows[m.optionCursor] == optionRowContinue {
			m.lastErr = nil
			m.state = StatePreview
			m.running = actionPreview
			return m, m.previewCmd(m.buildRequest())
		}
		m.adjustOption(rows[m.optionCursor], 1)
		return m, nil
	default:
		return m, nil
	}
}

func (m MainModel) updatePreviewState(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "esc":
		m.state = StateSelectTarget
		m.lastErr = nil
		return m, nil
	case "r":
		m.lastErr = nil
		m.running = actionPreview
		return m, m.previewCmd(m.buildRequest())
	case "a":
		m.confirmAction = actionApply
		m.state = StateConfirm
		m.lastErr = nil
		return m, nil
	case "e":
		m.confirmAction = actionExport
		m.confirmInput = m.defaultExportPath()
		m.confirmCursor = len([]rune(m.confirmInput))
		m.state = StateConfirm
		m.lastErr = nil
		return m, nil
	default:
		return m, nil
	}
}

func (m MainModel) updateConfirmState(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	if m.confirmAction == actionExport {
		if handled := m.handleConfirmTextInput(keyMsg); handled {
			return m, nil
		}
	}

	switch keyMsg.String() {
	case "esc":
		m.state = StatePreview
		m.lastErr = nil
		return m, nil
	case "enter":
		req := m.buildRequest()
		switch m.confirmAction {
		case actionApply:
			m.running = actionApply
			m.lastErr = nil
			return m, m.applyCmd(req)
		case actionExport:
			if strings.TrimSpace(m.confirmInput) == "" {
				m.lastErr = fmt.Errorf("export path is required")
				return m, nil
			}
			m.running = actionExport
			m.lastErr = nil
			return m, m.exportCmd(req, m.confirmInput)
		default:
			return m, nil
		}
	default:
		return m, nil
	}
}

func (m MainModel) updateResultState(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "b":
		m.state = StatePreview
		m.lastErr = nil
		return m, nil
	case "n":
		m.resetSelection()
		m.lastErr = nil
		return m, nil
	default:
		return m, nil
	}
}

func (m MainModel) updateBrowserState(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.state = m.browserReturn
			m.lastErr = nil
			return m, nil
		case "r":
			switch m.state {
			case StateProjects:
				m.running = actionLoadProjects
				return m, m.loadProjectsCmd()
			case StateSkills:
				m.running = actionLoadSkills
				return m, m.loadSkillsCmd(m.projectRoot())
			case StateMCP:
				m.running = actionLoadMCP
				return m, m.loadMCPCmd(m.projectRoot())
			}
		}
	}

	updated, cmd := m.browserView.Update(msg)
	m.browserView = updated.(browser.Model)
	return m, cmd
}

func (m MainModel) handleBrowserSelection(entry browser.Entry) (tea.Model, tea.Cmd) {
	if m.state != StateProjects {
		return m, nil
	}
	if strings.TrimSpace(entry.Key) == "" {
		return m, nil
	}
	m.running = actionLoadWorkspace
	m.lastErr = nil
	m.resetSelection()
	m.options.ProjectRoot = entry.Key
	return m, m.loadWorkspaceCmd(entry.Key)
}

func (m *MainModel) selectSession(item switcher.WorkspaceItem) {
	m.selectedSession = &item
	m.target = defaultTargetFor(item.Tool)
	m.mode = domain.SwitchModeProject
	m.includeSkills = true
	m.includeMCP = true
	m.sessionOnly = false
	m.showAdvanced = false
	m.optionCursor = 0
	m.lastPreview = nil
	m.lastResult = nil
	m.confirmAction = actionNone
	m.confirmInput = ""
	m.confirmCursor = 0
	m.state = StateSelectTarget
}

func (m *MainModel) resetSelection() {
	m.state = StateSelectSession
	m.selectedSession = nil
	m.target = ""
	m.mode = domain.SwitchModeProject
	m.includeSkills = true
	m.includeMCP = true
	m.sessionOnly = false
	m.showAdvanced = false
	m.optionCursor = 0
	m.lastPreview = nil
	m.lastResult = nil
	m.confirmAction = actionNone
	m.confirmInput = ""
	m.confirmCursor = 0
	m.browserReturn = StateSelectSession
}

func (m MainModel) loadWorkspaceCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		ws, err := m.backend.LoadWorkspace(m.ctx, projectRoot)
		return workspaceLoadedMsg{workspace: ws, err: err}
	}
}

func (m MainModel) loadProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := m.backend.LoadProjects(m.ctx, m.options.WorkspaceRoots)
		return projectsLoadedMsg{entries: entries, err: err}
	}
}

func (m MainModel) loadSkillsCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		entries, err := m.backend.LoadSkills(m.ctx, projectRoot)
		return skillsLoadedMsg{entries: entries, err: err}
	}
}

func (m MainModel) loadMCPCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		entries, err := m.backend.LoadMCP(m.ctx, projectRoot)
		return mcpLoadedMsg{entries: entries, err: err}
	}
}

func (m MainModel) previewCmd(req switcher.Request) tea.Cmd {
	return func() tea.Msg {
		result, err := m.backend.Preview(m.ctx, req)
		return previewLoadedMsg{result: result, err: err}
	}
}

func (m MainModel) applyCmd(req switcher.Request) tea.Cmd {
	return func() tea.Msg {
		result, err := m.backend.Apply(m.ctx, req)
		return actionFinishedMsg{action: actionApply, result: result, err: err}
	}
}

func (m MainModel) exportCmd(req switcher.Request, outDir string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.backend.Export(m.ctx, req, outDir)
		return actionFinishedMsg{action: actionExport, result: result, err: err}
	}
}

func (m MainModel) buildRequest() switcher.Request {
	req := switcher.Request{
		From:          m.selectedSession.Tool,
		Session:       m.selectedSession.ID,
		To:            m.target,
		ProjectRoot:   m.projectRoot(),
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

func (m MainModel) projectRoot() string {
	if strings.TrimSpace(m.workspace.ProjectRoot) != "" {
		return m.workspace.ProjectRoot
	}
	return m.options.ProjectRoot
}

func (m MainModel) defaultExportPath() string {
	if strings.TrimSpace(m.options.DefaultExportDir) != "" {
		return m.options.DefaultExportDir
	}
	root := m.projectRoot()
	if root == "" {
		return filepath.Join(".work-bridge", "exports", string(m.target))
	}
	return filepath.Join(root, ".work-bridge", "exports", string(m.target))
}

func (m MainModel) optionRows() []optionRow {
	rows := []optionRow{optionRowTarget, optionRowAdvanced}
	if m.showAdvanced {
		rows = append(rows, optionRowMode, optionRowScope, optionRowSkills, optionRowMCP)
	}
	rows = append(rows, optionRowContinue)
	return rows
}

func (m *MainModel) adjustOption(row optionRow, direction int) {
	switch row {
	case optionRowTarget:
		m.target = cycleTool(m.target, direction)
	case optionRowAdvanced:
		m.showAdvanced = !m.showAdvanced
		rows := m.optionRows()
		if m.optionCursor >= len(rows) {
			m.optionCursor = len(rows) - 1
		}
	case optionRowMode:
		if m.mode == domain.SwitchModeProject {
			m.mode = domain.SwitchModeNative
		} else {
			m.mode = domain.SwitchModeProject
		}
	case optionRowScope:
		m.sessionOnly = !m.sessionOnly
		if m.sessionOnly {
			m.includeSkills = false
			m.includeMCP = false
		} else {
			m.includeSkills = true
			m.includeMCP = true
		}
	case optionRowSkills:
		if !m.sessionOnly {
			m.includeSkills = !m.includeSkills
		}
	case optionRowMCP:
		if !m.sessionOnly {
			m.includeMCP = !m.includeMCP
		}
	}
}

func (m *MainModel) handleConfirmTextInput(msg tea.KeyPressMsg) bool {
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
	case "home":
		m.confirmCursor = 0
		return true
	case "end":
		m.confirmCursor = len([]rune(m.confirmInput))
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
	case "delete":
		runes := []rune(m.confirmInput)
		if m.confirmCursor >= len(runes) {
			return true
		}
		runes = append(runes[:m.confirmCursor], runes[m.confirmCursor+1:]...)
		m.confirmInput = string(runes)
		return true
	case "ctrl+u":
		m.confirmInput = ""
		m.confirmCursor = 0
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

func (m MainModel) renderCurrentState() string {
	switch m.state {
	case StateSelectSession:
		return m.renderSessionState()
	case StateSelectTarget:
		return m.renderTargetState()
	case StatePreview:
		return m.renderPreviewState()
	case StateConfirm:
		return m.renderConfirmState()
	case StateResult:
		return m.renderResultState()
	case StateProjects, StateSkills, StateMCP:
		return m.renderBrowserState()
	default:
		return ""
	}
}

func (m MainModel) renderHeader() string {
	lines := []string{styles.Title.Render("work-bridge")}
	if root := m.projectRoot(); root != "" {
		lines = append(lines, styles.Subtitle.Render(root))
	}
	if m.selectedSession != nil {
		lines = append(lines, styles.Muted.Render(fmt.Sprintf("source %s/%s -> target %s", m.selectedSession.Tool, m.selectedSession.ID, m.target)))
	}
	lines = append(lines, styles.Muted.Render("commands: /projects  /mcp  /skills"))
	return styles.Section.Render(strings.Join(lines, "\n"))
}

func (m MainModel) renderFooter() string {
	if m.lastErr == nil {
		return styles.Muted.Render(m.footerText())
	}
	return styles.ErrorBox.Render(m.lastErr.Error())
}

func (m MainModel) footerText() string {
	switch m.state {
	case StateSelectSession:
		return "enter select  r refresh  ? help  q quit"
	case StateSelectTarget:
		return "up/down move  left/right change  enter preview  esc back  ? help  q quit"
	case StatePreview:
		return "a apply  e export  r refresh  esc back  ? help  q quit"
	case StateConfirm:
		if m.confirmAction == actionExport {
			return "type path  enter confirm  esc cancel  left/right move cursor  backspace delete"
		}
		return "enter confirm  esc cancel"
	case StateResult:
		return "b back to preview  n new session  ? help  q quit"
	case StateProjects:
		return "up/down move  enter switch project  r refresh  esc back  / slash commands"
	case StateSkills, StateMCP:
		return "up/down move  enter inspect  r refresh  esc back  / slash commands"
	default:
		return ""
	}
}

func (m MainModel) renderSessionState() string {
	if m.lastErr != nil {
		return styles.ErrorBox.Render("Failed to load workspace sessions.\n\n" + m.lastErr.Error())
	}
	if len(m.workspace.Sessions) == 0 {
		return styles.Panel.Render("No sessions found for this project.\n\nUse `work-bridge inspect <tool>` to verify the local session stores.")
	}
	return styles.Panel.Render(m.sessionView.View().Content)
}

func (m MainModel) renderTargetState() string {
	lines := []string{
		styles.SectionTitle.Render("Target & Options"),
		m.renderOptionLine(optionRowTarget, "Target tool", string(m.target), false),
		m.renderOptionLine(optionRowAdvanced, "Advanced", onOff(m.showAdvanced), false),
	}
	if m.showAdvanced {
		lines = append(lines,
			m.renderOptionLine(optionRowMode, "Mode", string(m.mode), false),
			m.renderOptionLine(optionRowScope, "Scope", scopeLabel(m.sessionOnly), false),
			m.renderOptionLine(optionRowSkills, "Skills", onOff(m.includeSkills), m.sessionOnly),
			m.renderOptionLine(optionRowMCP, "MCP", onOff(m.includeMCP), m.sessionOnly),
		)
	}
	lines = append(lines, m.renderOptionLine(optionRowContinue, "Continue", "Build preview", false))
	return styles.Panel.Render(strings.Join(lines, "\n"))
}

func (m MainModel) renderOptionLine(row optionRow, label string, value string, disabled bool) string {
	current := m.optionRows()[m.optionCursor] == row
	prefix := "  "
	if current {
		prefix = styles.Highlight.Render("› ")
	}
	line := fmt.Sprintf("%s%s: %s", prefix, label, value)
	if disabled {
		return styles.Disabled.Render(line + " (disabled by session-only)")
	}
	if current {
		return styles.Selected.Render(line)
	}
	return line
}

func (m MainModel) renderPreviewState() string {
	if m.lastErr != nil {
		return styles.ErrorBox.Render("Preview failed.\n\n" + m.lastErr.Error())
	}
	if m.lastPreview == nil {
		return styles.Panel.Render("No preview available yet.")
	}

	plan := m.lastPreview.Plan
	lines := []string{
		styles.SectionTitle.Render("Preview"),
		fmt.Sprintf("overall: %s", styles.Status(plan.Status)),
		fmt.Sprintf("destination: %s", plan.DestinationRoot),
		fmt.Sprintf("session: %s  %s", styles.Status(plan.Session.State), plan.Session.Summary),
		fmt.Sprintf("skills: %s  %s", styles.Status(plan.Skills.State), plan.Skills.Summary),
		fmt.Sprintf("mcp: %s  %s", styles.Status(plan.MCP.State), plan.MCP.Summary),
	}

	if len(plan.PlannedFiles) > 0 {
		lines = append(lines, "", styles.SectionTitle.Render("Planned Files"))
		for _, file := range plan.PlannedFiles {
			lines = append(lines, fmt.Sprintf("- [%s] %s (%s)", file.Section, file.Path, file.Action))
		}
	}
	if warnings := collectWarnings(plan.Warnings, nil); len(warnings) > 0 {
		lines = append(lines, "", styles.SectionTitle.Render("Warnings"))
		for _, warning := range warnings {
			lines = append(lines, styles.WarningText.Render("- "+warning))
		}
	}
	if len(plan.Errors) > 0 {
		lines = append(lines, "", styles.SectionTitle.Render("Errors"))
		for _, errText := range plan.Errors {
			lines = append(lines, styles.ErrorText.Render("- "+errText))
		}
	}
	return styles.Panel.Render(strings.Join(lines, "\n"))
}

func (m MainModel) renderConfirmState() string {
	lines := []string{
		styles.SectionTitle.Render("Confirm"),
		fmt.Sprintf("action: %s", actionLabel(m.confirmAction)),
		fmt.Sprintf("target: %s", m.target),
		fmt.Sprintf("mode: %s", m.mode),
	}
	if m.confirmAction == actionExport {
		lines = append(lines, "", styles.SectionTitle.Render("Export Path"), styles.InputBox.Render(m.confirmInputWithCursor()))
	}
	return styles.Panel.Render(strings.Join(lines, "\n"))
}

func (m MainModel) renderResultState() string {
	if m.lastErr != nil {
		return styles.ErrorBox.Render("Action failed.\n\n" + m.lastErr.Error())
	}
	if m.lastResult == nil || m.lastResult.Report == nil {
		return styles.Panel.Render("No action result available.")
	}

	report := m.lastResult.Report
	lines := []string{
		styles.SectionTitle.Render("Result"),
		fmt.Sprintf("overall: %s", styles.Status(report.Status)),
		fmt.Sprintf("destination: %s", report.DestinationRoot),
		fmt.Sprintf("updated files: %d", len(report.FilesUpdated)),
		fmt.Sprintf("backups: %d", len(report.BackupsCreated)),
		fmt.Sprintf("session: %s  %s", styles.Status(report.Session.State), report.Session.Summary),
		fmt.Sprintf("skills: %s  %s", styles.Status(report.Skills.State), report.Skills.Summary),
		fmt.Sprintf("mcp: %s  %s", styles.Status(report.MCP.State), report.MCP.Summary),
	}
	if len(report.FilesUpdated) > 0 {
		lines = append(lines, "", styles.SectionTitle.Render("Files Updated"))
		for _, file := range report.FilesUpdated {
			lines = append(lines, "- "+file)
		}
	}
	if warnings := collectWarnings(report.Warnings, report.Errors); len(warnings) > 0 {
		lines = append(lines, "", styles.SectionTitle.Render("Warnings"))
		for _, warning := range warnings {
			lines = append(lines, styles.WarningText.Render("- "+warning))
		}
	}
	if len(report.Errors) > 0 {
		lines = append(lines, "", styles.SectionTitle.Render("Errors"))
		for _, errText := range report.Errors {
			lines = append(lines, styles.ErrorText.Render("- "+errText))
		}
	}
	return styles.Panel.Render(strings.Join(lines, "\n"))
}

func (m MainModel) renderBrowserState() string {
	title := "Browser"
	empty := "No entries found."
	switch m.state {
	case StateProjects:
		title = "Projects"
		empty = "No projects found. Configure --workspace-roots or WORK_BRIDGE_WORKSPACE_ROOTS to widen project discovery."
	case StateSkills:
		title = "Skills"
		empty = "No skills found for this project or user scope."
	case StateMCP:
		title = "MCP"
		empty = "No MCP configs found for this project or user scope."
	}
	if m.lastErr != nil {
		return styles.ErrorBox.Render(title + " failed.\n\n" + m.lastErr.Error())
	}
	selected, hasSelected := m.browserView.SelectedEntry()
	if !hasSelected {
		return styles.Panel.Render(empty)
	}
	lines := []string{
		styles.SectionTitle.Render(title),
		m.browserView.View().Content,
		"",
		styles.SectionTitle.Render("Details"),
	}
	lines = append(lines, selected.Details...)
	return styles.Panel.Render(strings.Join(lines, "\n"))
}

func (m MainModel) renderBusyView() string {
	label := "Working..."
	switch m.running {
	case actionLoadWorkspace:
		label = "Loading workspace..."
	case actionPreview:
		label = "Building preview..."
	case actionApply:
		label = "Applying handoff..."
	case actionExport:
		label = "Exporting handoff..."
	case actionLoadProjects:
		label = "Scanning projects..."
	case actionLoadSkills:
		label = "Scanning skills..."
	case actionLoadMCP:
		label = "Scanning MCP..."
	}
	return styles.Panel.Render(label)
}

func (m MainModel) renderHelp() string {
	var lines []string
	switch m.state {
	case StateSelectSession:
		lines = []string{
			"Session step",
			"- Use the list to choose a source session from this workspace.",
			"- Press `enter` to continue to target selection.",
			"- Press `r` to reload sessions from disk.",
		}
	case StateSelectTarget:
		lines = []string{
			"Target step",
			"- Choose the target tool first.",
			"- Open Advanced to enable native mode or trim skills/MCP.",
			"- `session-only` disables skills and MCP automatically.",
		}
	case StatePreview:
		lines = []string{
			"Preview step",
			"- `a` applies the handoff into the project or native target.",
			"- `e` exports to a destination directory.",
			"- `r` rebuilds the preview using the current options.",
		}
	case StateConfirm:
		lines = []string{
			"Confirm step",
			"- Confirm file-changing actions here.",
			"- Export lets you edit the destination path before running.",
		}
	case StateResult:
		lines = []string{
			"Result step",
			"- `b` returns to preview using the same selections.",
			"- `n` starts over from the session list.",
		}
	case StateProjects, StateSkills, StateMCP:
		lines = []string{
			"Browser step",
			"- Type `/projects`, `/skills`, or `/mcp` from anywhere to jump here.",
			"- Arrow keys move through the list and mouse interactions are forwarded to the active list.",
			"- `esc` returns to the previous migration screen.",
		}
	}
	return strings.Join(lines, "\n")
}

func (m MainModel) renderCommandPalette() string {
	lines := []string{
		styles.SectionTitle.Render("Slash Commands"),
		styles.InputBox.Render(m.commandInputWithCursor()),
		styles.Muted.Render("/projects  /mcp  /skills"),
	}
	return styles.HelpBox.Render(strings.Join(lines, "\n"))
}

func (m MainModel) confirmInputWithCursor() string {
	runes := []rune(m.confirmInput)
	if m.confirmCursor < 0 {
		m.confirmCursor = 0
	}
	if m.confirmCursor > len(runes) {
		m.confirmCursor = len(runes)
	}
	cursor := styles.Cursor.Render(" ")
	rendered := append([]rune{}, runes[:m.confirmCursor]...)
	rendered = append(rendered, []rune(cursor)...)
	rendered = append(rendered, runes[m.confirmCursor:]...)
	return string(rendered)
}

func (m MainModel) commandInputWithCursor() string {
	runes := []rune(m.commandInput)
	if m.commandCursor < 0 {
		m.commandCursor = 0
	}
	if m.commandCursor > len(runes) {
		m.commandCursor = len(runes)
	}
	cursor := styles.Cursor.Render(" ")
	rendered := append([]rune{}, runes[:m.commandCursor]...)
	rendered = append(rendered, []rune(cursor)...)
	rendered = append(rendered, runes[m.commandCursor:]...)
	return string(rendered)
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

func actionLabel(action actionKind) string {
	switch action {
	case actionApply:
		return "apply"
	case actionExport:
		return "export"
	case actionPreview:
		return "preview"
	case actionLoadWorkspace:
		return "load"
	default:
		return "idle"
	}
}

func scopeLabel(sessionOnly bool) string {
	if sessionOnly {
		return "session-only"
	}
	return "full"
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func collectWarnings(values []string, extras []string) []string {
	combined := append([]string{}, values...)
	combined = append(combined, extras...)
	seen := make(map[string]struct{}, len(combined))
	result := make([]string, 0, len(combined))
	for _, value := range combined {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func projectEntries(entries []catalog.ProjectEntry) []browser.Entry {
	out := make([]browser.Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, browser.Entry{
			Key:         entry.Root,
			Title:       entry.Name,
			Description: strings.Join([]string{entry.WorkspaceRoot, strings.Join(entry.Markers, ", ")}, " • "),
			FilterValue: strings.Join([]string{entry.Name, entry.Root, strings.Join(entry.Markers, " ")}, " "),
			Details: []string{
				fmt.Sprintf("root: %s", entry.Root),
				fmt.Sprintf("workspace: %s", entry.WorkspaceRoot),
				fmt.Sprintf("markers: %s", strings.Join(entry.Markers, ", ")),
			},
		})
	}
	return out
}

func skillEntries(entries []catalog.SkillEntry) []browser.Entry {
	out := make([]browser.Entry, 0, len(entries))
	for _, entry := range entries {
		description := strings.TrimSpace(entry.Description)
		if description == "" {
			description = entry.Source
		}
		details := []string{
			fmt.Sprintf("scope: %s", firstNonEmpty(entry.Scope, "unknown")),
			fmt.Sprintf("source: %s", firstNonEmpty(entry.Source, "unknown")),
			fmt.Sprintf("tool: %s", firstNonEmpty(entry.Tool, "shared")),
			fmt.Sprintf("entry: %s", entry.EntryPath),
		}
		if len(entry.Files) > 0 {
			details = append(details, fmt.Sprintf("files: %d", len(entry.Files)))
		}
		out = append(out, browser.Entry{
			Key:         entry.EntryPath,
			Title:       entry.Name,
			Description: description,
			FilterValue: strings.Join([]string{entry.Name, entry.Description, entry.EntryPath, entry.Source}, " "),
			Details:     details,
		})
	}
	return out
}

func mcpEntries(entries []catalog.MCPEntry) []browser.Entry {
	out := make([]browser.Entry, 0, len(entries))
	for _, entry := range entries {
		description := strings.TrimSpace(entry.Details)
		if description == "" {
			description = entry.Source
		}
		out = append(out, browser.Entry{
			Key:         entry.Path,
			Title:       entry.Name,
			Description: description,
			FilterValue: strings.Join([]string{entry.Name, entry.Path, entry.Source, entry.Status}, " "),
			Details: []string{
				fmt.Sprintf("path: %s", entry.Path),
				fmt.Sprintf("scope: %s", entry.Source),
				fmt.Sprintf("status: %s", entry.Status),
				fmt.Sprintf("details: %s", firstNonEmpty(entry.Details, "-")),
			},
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
