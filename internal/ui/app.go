package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jaeyoung0509/work-bridge/internal/catalog"
	"github.com/jaeyoung0509/work-bridge/internal/detect"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
	"github.com/jaeyoung0509/work-bridge/internal/ui/styles"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/actionmenu"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/browser"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/cmdpalette"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/handoff"
	"github.com/jaeyoung0509/work-bridge/internal/ui/views/session"
)

// ─── Backend Interface ──────────────────────────────────────

type Backend interface {
	LoadWorkspace(ctx context.Context, projectRoot string) (switcher.Workspace, error)
	LoadProjects(ctx context.Context, roots []string) ([]catalog.ProjectEntry, error)
	LoadSkills(ctx context.Context, projectRoot string) ([]catalog.SkillEntry, error)
	LoadMCP(ctx context.Context, projectRoot string) ([]catalog.MCPEntry, error)
	MigrateMCP(ctx context.Context, entry catalog.MCPEntry, target domain.Tool, projectRoot string) error
	MigrateSkill(ctx context.Context, entry catalog.SkillEntry, target domain.Tool, projectRoot string) error
	Preview(ctx context.Context, req switcher.Request) (switcher.Result, error)
	Apply(ctx context.Context, req switcher.Request) (switcher.Result, error)
	Export(ctx context.Context, req switcher.Request, outDir string) (switcher.Result, error)
}

// ─── Options ────────────────────────────────────────────────

type Options struct {
	ProjectRoot      string
	WorkspaceRoots   []string
	DefaultExportDir string
	DetectReport     *detect.Report
}

// ─── Screen Enum ────────────────────────────────────────────

type AppScreen int

const (
	ScreenHub AppScreen = iota
	ScreenProjects
	ScreenSessions
	ScreenHandoff
	ScreenBrowser
	ScreenActionMenu
)

// ─── Action Enum ────────────────────────────────────────────

type actionKind int

const (
	actionNone actionKind = iota
	actionLoadProjects
	actionLoadWorkspace
	actionPreview
	actionApply
	actionExport
	actionLoadSkills
	actionLoadMCP
	actionMigrate
)

// ─── Hub Quick Actions ──────────────────────────────────────

type quickAction struct {
	command     string
	label       string
	description string
}

var hubActions = []quickAction{
	{"/projects", "Projects", "Browse and migrate sessions per project"},
	{"/sessions", "Sessions", "Select and import sessions"},
	{"/mcp", "MCP", "Manage MCP server configurations"},
	{"/skills", "Skills", "Browse available skills"},
}

// ─── Messages ───────────────────────────────────────────────

type projectsLoadedMsg struct {
	entries []catalog.ProjectEntry
	err     error
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
type skillsLoadedMsg struct {
	entries []catalog.SkillEntry
	err     error
}
type mcpLoadedMsg struct {
	entries []catalog.MCPEntry
	err     error
}

// ─── Main Model ─────────────────────────────────────────────

type MainModel struct {
	ctx     context.Context
	backend Backend
	options Options

	screen          AppScreen
	screenStack     []AppScreen
	cmdPalette      cmdpalette.Model
	actionCursor    int

	// Data
	projects        []catalog.ProjectEntry
	projectList     browser.Model
	activeProjectRoot string

	workspace       switcher.Workspace
	sessionList     session.Model
	selectedSession *switcher.WorkspaceItem

	handoffView     handoff.Model
	browserView     browser.Model
	browserTitle    string
	actionMenuView  actionmenu.Model

	running         actionKind
	lastErr         error

	quitting        bool
	width           int
	height          int
}

// ─── Constructor ────────────────────────────────────────────

func NewMainModel(ctx context.Context, backend Backend, opts Options) MainModel {
	return MainModel{
		ctx:             ctx,
		backend:         backend,
		options:         opts,
		screen:          ScreenHub,
		cmdPalette:      cmdpalette.New(cmdpalette.DefaultCommands()),
		projectList:     browser.NewModel("Projects"),
		sessionList:     session.NewModel(),
		browserView:     browser.NewModel("Browser"),
		running:         actionLoadProjects,
		activeProjectRoot: opts.ProjectRoot,
	}
}

// ─── Init ───────────────────────────────────────────────────

func (m MainModel) Init() tea.Cmd {
	return m.loadProjectsCmd()
}

// ─── Update ─────────────────────────────────────────────────

func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()
		return m, nil

	case tea.KeyPressMsg:
		// Command palette intercepts when active
		if m.cmdPalette.Active() {
			return m.updateCmdPalette(msg)
		}
		// Global keys
		if handled, model, cmd := m.handleGlobalKey(msg); handled {
			return model, cmd
		}

	// ── Async result messages ──
	case projectsLoadedMsg:
		return m.handleProjectsLoaded(msg)
	case workspaceLoadedMsg:
		return m.handleWorkspaceLoaded(msg)
	case previewLoadedMsg:
		return m.handlePreviewLoaded(msg)
	case actionFinishedMsg:
		return m.handleActionFinished(msg)
	case skillsLoadedMsg:
		return m.handleSkillsLoaded(msg)
	case mcpLoadedMsg:
		return m.handleMCPLoaded(msg)

	// ── Command palette messages ──
	case cmdpalette.ExecMsg:
		return m.dispatchCommand(msg.Command)
	case cmdpalette.CancelMsg:
		return m, nil

	// ── Handoff messages ──
	case handoff.BackMsg:
		return m.popScreen()
	case handoff.ApplyRequestMsg:
		m.running = actionApply
		return m, m.applyCmd(msg.Request)
	case handoff.ExportRequestMsg:
		m.running = actionExport
		return m, m.exportCmd(msg.Request, msg.ExportPath)

	// ── Action menu messages ──
	case actionmenu.BackMsg:
		return m.popScreen()
	case actionmenu.ActionSelectedMsg:
		if msg.ActionType == actionmenu.ActionEdit {
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vim"
			}
			cmd := exec.Command(editor, msg.Entry.Key)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return actionFinishedMsg{action: actionNone, err: err}
			})
		} else if msg.ActionType == actionmenu.ActionMigrate {
			m.running = actionMigrate
			m.actionMenuView.SetStatus("Migrating...", false)
			return m, m.migrateCmd(msg)
		}
	}

	// Per-screen update
	switch m.screen {
	case ScreenHub:
		return m.updateHub(msg)
	case ScreenProjects:
		return m.updateProjects(msg)
	case ScreenSessions:
		return m.updateSessions(msg)
	case ScreenHandoff:
		return m.updateHandoff(msg)
	case ScreenBrowser:
		return m.updateBrowser(msg)
	case ScreenActionMenu:
		return m.updateActionMenu(msg)
	}

	return m, nil
}

// ─── Global Keys ────────────────────────────────────────────

func (m MainModel) handleGlobalKey(msg tea.KeyPressMsg) (bool, tea.Model, tea.Cmd) {
	key := msg.Key()

	// "/" opens command palette
	if key.Mod == 0 && strings.HasPrefix(key.Text, "/") {
		m.cmdPalette = m.cmdPalette.Open(key.Text)
		return true, m, nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		if m.screen == ScreenHub {
			m.quitting = true
			return true, m, tea.Quit
		}
		// On non-hub screens, go back
		model, cmd := m.popScreen()
		return true, model, cmd
	case "esc":
		if m.screen != ScreenHub {
			model, cmd := m.popScreen()
			return true, model, cmd
		}
	}

	return false, m, nil
}

// ─── Command Palette ────────────────────────────────────────

func (m MainModel) updateCmdPalette(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	updated, cmd := m.cmdPalette.Update(msg)
	m.cmdPalette = updated
	if cmd != nil {
		// Execute command palette messages
		resultMsg := cmd()
		switch resultMsg := resultMsg.(type) {
		case cmdpalette.ExecMsg:
			return m.dispatchCommand(resultMsg.Command)
		case cmdpalette.CancelMsg:
			return m, nil
		}
	}
	return m, nil
}

func (m MainModel) dispatchCommand(command string) (tea.Model, tea.Cmd) {
	m.lastErr = nil
	command = strings.ToLower(strings.TrimSpace(command))
	fields := strings.Fields(command)
	if len(fields) > 0 {
		command = fields[0]
	}

	switch command {
	case "/projects":
		m.pushScreen(ScreenProjects)
		if m.running == actionNone && len(m.projects) == 0 {
			m.running = actionLoadProjects
			return m, m.loadProjectsCmd()
		}
		return m, nil
	case "/sessions":
		m.pushScreen(ScreenSessions)
		if m.activeProjectRoot != "" && len(m.workspace.Sessions) == 0 {
			m.running = actionLoadWorkspace
			return m, m.loadWorkspaceCmd(m.activeProjectRoot)
		}
		return m, nil
	case "/skills":
		m.browserTitle = "Skills"
		m.pushScreen(ScreenBrowser)
		m.running = actionLoadSkills
		return m, m.loadSkillsCmd(m.activeProjectRoot)
	case "/mcp":
		m.browserTitle = "MCP"
		m.pushScreen(ScreenBrowser)
		m.running = actionLoadMCP
		return m, m.loadMCPCmd(m.activeProjectRoot)
	default:
		m.lastErr = fmt.Errorf("unknown command: %s", command)
		return m, nil
	}
}

// ─── Screen Stack ───────────────────────────────────────────

func (m *MainModel) pushScreen(s AppScreen) {
	m.screenStack = append(m.screenStack, m.screen)
	m.screen = s
	m.lastErr = nil
}

func (m MainModel) popScreen() (tea.Model, tea.Cmd) {
	if len(m.screenStack) > 0 {
		m.screen = m.screenStack[len(m.screenStack)-1]
		m.screenStack = m.screenStack[:len(m.screenStack)-1]
	} else {
		m.screen = ScreenHub
	}
	m.lastErr = nil
	return m, nil
}

// ─── Size Management ────────────────────────────────────────

func (m *MainModel) updateSizes() {
	if m.width == 0 || m.height == 0 {
		return
	}
	contentH := m.height - 8
	if contentH < 10 {
		contentH = 10
	}
	m.projectList.SetSize(m.width-6, contentH)
	m.sessionList.List.SetSize(m.width-6, contentH)
	m.browserView.SetSize(m.width-6, contentH)
	m.cmdPalette.SetWidth(m.width)
}

// ─── Hub Screen ─────────────────────────────────────────────

func (m MainModel) updateHub(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up":
			if m.actionCursor > 0 {
				m.actionCursor--
			}
		case "down":
			if m.actionCursor < len(hubActions)-1 {
				m.actionCursor++
			}
		case "enter":
			if m.actionCursor < len(hubActions) {
				return m.dispatchCommand(hubActions[m.actionCursor].command)
			}
		}
	case tea.MouseClickMsg:
		if msg.Mouse().Button == tea.MouseLeft {
			return m.handleHubClick(msg)
		}
	}
	return m, nil
}

func (m MainModel) handleHubClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	y := msg.Mouse().Y
	// Agent cards occupy roughly rows 5-9
	// Quick actions start at roughly row 12+
	actionStartY := 13
	if y >= actionStartY && y < actionStartY+len(hubActions) {
		idx := y - actionStartY
		if idx >= 0 && idx < len(hubActions) {
			return m.dispatchCommand(hubActions[idx].command)
		}
	}
	return m, nil
}

// ─── Projects Screen ────────────────────────────────────────

func (m MainModel) updateProjects(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			m.projectList.List.CursorUp()
			return m, nil
		case "down", "j":
			m.projectList.List.CursorDown()
			return m, nil
		case "enter":
			if entry, ok := m.projectList.SelectedEntry(); ok {
				m.activeProjectRoot = entry.Key
				m.options.ProjectRoot = entry.Key
				m.running = actionLoadWorkspace
				m.pushScreen(ScreenSessions)
				return m, m.loadWorkspaceCmd(m.activeProjectRoot)
			}
		}
	}
	return m, nil
}

// ─── Sessions Screen ────────────────────────────────────────

func (m MainModel) updateSessions(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			m.sessionList.List.CursorUp()
			return m, nil
		case "down", "j":
			m.sessionList.List.CursorDown()
			return m, nil
		case "enter":
			if selected, ok := m.sessionList.List.SelectedItem().(interface {
				Title() string
				Description() string
			}); ok {
				for _, s := range m.workspace.Sessions {
					if string(s.Tool)+" • "+s.ID == selected.Description() {
						m.selectedSession = &s
						m.handoffView = handoff.New(s, m.activeProjectRoot, m.options.DefaultExportDir)
						contentH := m.height - 8
						if contentH < 10 {
							contentH = 10
						}
						m.handoffView.SetSize(m.width-4, contentH)
						m.pushScreen(ScreenHandoff)
						m.running = actionPreview
						return m, m.previewCmd(m.handoffView.BuildRequest())
					}
				}
			}
		}
	}
	return m, nil
}

// ─── Handoff Screen ─────────────────────────────────────────

func (m MainModel) updateHandoff(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.handoffView.Update(msg)
	m.handoffView = updated

	if cmd != nil {
		resultMsg := cmd()
		switch resultMsg := resultMsg.(type) {
		case handoff.BackMsg:
			return m.popScreen()
		case handoff.ApplyRequestMsg:
			m.running = actionApply
			return m, m.applyCmd(resultMsg.Request)
		case handoff.ExportRequestMsg:
			m.running = actionExport
			return m, m.exportCmd(resultMsg.Request, resultMsg.ExportPath)
		}
	}
	return m, nil
}

// ─── Browser Screen ─────────────────────────────────────────

func (m MainModel) updateBrowser(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			m.browserView.List.CursorUp()
			return m, nil
		case "down", "j":
			m.browserView.List.CursorDown()
			return m, nil
		case "enter":
			if entry, ok := m.browserView.SelectedEntry(); ok {
				m.actionMenuView = actionmenu.New(entry)
				m.actionMenuView.SetSize(m.width, m.height)
				m.pushScreen(ScreenActionMenu)
				return m, nil
			}
		}
	}
	return m, nil
}

// ─── Action Menu Screen ──────────────────────────────────────

func (m MainModel) updateActionMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.actionMenuView.Update(msg)
	m.actionMenuView = updated
	if cmd != nil {
		resultMsg := cmd()
		switch resultMsg := resultMsg.(type) {
		case actionmenu.BackMsg:
			return m.popScreen()
		case actionmenu.ActionSelectedMsg:
			if resultMsg.ActionType == actionmenu.ActionEdit {
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vim"
				}
				execCmd := exec.Command(editor, resultMsg.Entry.Key)
				return m, tea.ExecProcess(execCmd, func(err error) tea.Msg {
					return actionFinishedMsg{action: actionNone, err: err}
				})
			} else if resultMsg.ActionType == actionmenu.ActionMigrate {
				m.running = actionMigrate
				m.actionMenuView.SetStatus("Migrating...", false)
				return m, m.migrateCmd(resultMsg)
			}
		}
	}
	return m, nil
}

// ─── Async Handlers ─────────────────────────────────────────

func (m MainModel) handleProjectsLoaded(msg projectsLoadedMsg) (tea.Model, tea.Cmd) {
	m.running = actionNone
	m.projects = msg.entries
	m.lastErr = msg.err
	if msg.err == nil {
		m.projectList.SetEntries(projectEntries(msg.entries))
		if m.activeProjectRoot == "" && len(msg.entries) > 0 {
			m.activeProjectRoot = msg.entries[0].Root
		}
		// Auto-load workspace for active project
		if m.activeProjectRoot != "" && len(m.workspace.Sessions) == 0 {
			m.running = actionLoadWorkspace
			return m, m.loadWorkspaceCmd(m.activeProjectRoot)
		}
	}
	return m, nil
}

func (m MainModel) handleWorkspaceLoaded(msg workspaceLoadedMsg) (tea.Model, tea.Cmd) {
	m.running = actionNone
	m.workspace = msg.workspace
	m.lastErr = msg.err
	if msg.err == nil {
		m.sessionList.SetSessions(msg.workspace.Sessions)
		if len(msg.workspace.Sessions) > 0 {
			m.sessionList.List.Select(0)
		} else {
			m.selectedSession = nil
		}
	}
	return m, nil
}

func (m MainModel) handlePreviewLoaded(msg previewLoadedMsg) (tea.Model, tea.Cmd) {
	m.running = actionNone
	m.lastErr = msg.err
	if msg.err == nil {
		r := msg.result
		m.handoffView.SetPreview(&r)
	}
	return m, nil
}

func (m MainModel) handleActionFinished(msg actionFinishedMsg) (tea.Model, tea.Cmd) {
	m.running = actionNone
	m.lastErr = msg.err
	switch msg.action {
	case actionMigrate:
		if msg.err != nil {
			m.actionMenuView.SetStatus("Error: "+msg.err.Error(), true)
		} else {
			m.actionMenuView.SetStatus("✓ Migration complete!", false)
		}
	default:
		if msg.err == nil {
			r := msg.result
			m.handoffView.SetResult(&r, nil)
		} else {
			m.handoffView.SetResult(nil, msg.err)
		}
	}
	return m, nil
}

func (m MainModel) handleSkillsLoaded(msg skillsLoadedMsg) (tea.Model, tea.Cmd) {
	m.running = actionNone
	m.lastErr = msg.err
	if msg.err == nil {
		m.browserView.SetTitle("Skills")
		m.browserView.SetEntries(skillEntries(msg.entries))
	}
	return m, nil
}

func (m MainModel) handleMCPLoaded(msg mcpLoadedMsg) (tea.Model, tea.Cmd) {
	m.running = actionNone
	m.lastErr = msg.err
	if msg.err == nil {
		m.browserView.SetTitle("MCP")
		m.browserView.SetEntries(mcpEntries(msg.entries))
	}
	return m, nil
}

// ─── Commands ───────────────────────────────────────────────

func (m MainModel) loadProjectsCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := m.backend.LoadProjects(m.ctx, m.options.WorkspaceRoots)
		return projectsLoadedMsg{entries: entries, err: err}
	}
}

func (m MainModel) loadWorkspaceCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		ws, err := m.backend.LoadWorkspace(m.ctx, projectRoot)
		return workspaceLoadedMsg{workspace: ws, err: err}
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

func (m MainModel) migrateCmd(msg actionmenu.ActionSelectedMsg) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch raw := msg.Entry.Raw.(type) {
		case catalog.MCPEntry:
			err = m.backend.MigrateMCP(m.ctx, raw, msg.Target, m.activeProjectRoot)
		case catalog.SkillEntry:
			err = m.backend.MigrateSkill(m.ctx, raw, msg.Target, m.activeProjectRoot)
		default:
			err = fmt.Errorf("unknown entry type for migration")
		}
		return actionFinishedMsg{action: actionMigrate, err: err}
	}
}

// ─── View ───────────────────────────────────────────────────

func (m MainModel) View() tea.View {
	if m.quitting {
		return tea.NewView("Goodbye! 👋\n")
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	var mainContent string
	switch m.screen {
	case ScreenHub:
		mainContent = m.renderHub()
	case ScreenProjects:
		mainContent = m.renderProjectsScreen()
	case ScreenSessions:
		mainContent = m.renderSessionsScreen()
	case ScreenHandoff:
		mainContent = m.handoffView.View().Content
	case ScreenBrowser:
		mainContent = m.renderBrowserScreen()
	case ScreenActionMenu:
		mainContent = m.actionMenuView.View().Content
	}

	content := lipgloss.JoinVertical(lipgloss.Left, header, mainContent, footer)

	// Command palette overlay
	if m.cmdPalette.Active() {
		paletteView := m.cmdPalette.RenderFloating(m.width, m.height)
		content = lipgloss.JoinVertical(lipgloss.Left, content, paletteView)
	}

	view := tea.NewView(styles.AppContainer.Render(content))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeAllMotion
	return view
}

// ─── Hub Render ─────────────────────────────────────────────

func (m MainModel) renderHub() string {
	var b strings.Builder

	// Title section
	b.WriteString(styles.Title.Render("◆ work-bridge"))
	b.WriteString("  ")
	b.WriteString(styles.Muted.Render("Multi-agent session handoff"))
	b.WriteString("\n\n")

	// Agent cards
	b.WriteString(styles.SectionTitle.Render("Agent Connections"))
	b.WriteString("\n\n")
	b.WriteString(m.renderAgentCards())
	b.WriteString("\n\n")

	// Quick actions
	b.WriteString(styles.SectionTitle.Render("Quick Actions"))
	b.WriteString("\n\n")
	for i, action := range hubActions {
		var line string
		if i == m.actionCursor {
			line = styles.QuickActionActive.Render(
				"  ▸ " + styles.QuickActionKey.Render(action.command) + "  " + action.description,
			)
		} else {
			line = styles.QuickAction.Render(
				"    " + styles.CmdSlash.Render(action.command) + "  " + styles.Muted.Render(action.description),
			)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render("Type / to open command palette • ↑↓ to navigate • Enter to select"))

	return b.String()
}

func (m MainModel) renderAgentCards() string {
	tools := []struct {
		name      string
		installed bool
	}{
		{"codex", false},
		{"gemini", false},
		{"claude", false},
		{"opencode", false},
	}

	if m.options.DetectReport != nil {
		for i, t := range m.options.DetectReport.Tools {
			if i < len(tools) {
				tools[i].installed = t.Installed
			}
		}
	}

	var b strings.Builder
	for _, t := range tools {
		icon := styles.AgentIcon(t.name)
		name := strings.ToUpper(t.name)
		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.AgentColor(t.name))

		if t.installed {
			b.WriteString("  " + nameStyle.Render(icon+" "+name) + " " + styles.BadgeSuccess.Render("●") + "  ")
		} else {
			b.WriteString("  " + styles.Muted.Render(icon+" "+name) + " " + styles.BadgeMuted.Render("○") + "  ")
		}
	}

	return b.String()
}

// ─── Projects Screen Render ─────────────────────────────────

func (m MainModel) renderProjectsScreen() string {
	contentH := m.height - 8
	if contentH < 10 {
		contentH = 10
	}
	return styles.ActivePane.Width(m.width - 6).Height(contentH).Render(m.projectList.View().Content)
}

// ─── Sessions Screen Render ─────────────────────────────────

func (m MainModel) renderSessionsScreen() string {
	contentH := m.height - 8
	if contentH < 10 {
		contentH = 10
	}

	header := styles.SectionTitle.Render("◇ Sessions") + "  " +
		styles.Muted.Render(m.activeProjectRoot) + "\n"

	return styles.ActivePane.Width(m.width - 6).Height(contentH).Render(
		header + m.sessionList.View().Content,
	)
}

// ─── Browser Screen Render ──────────────────────────────────

func (m MainModel) renderBrowserScreen() string {
	contentH := m.height - 8
	if contentH < 10 {
		contentH = 10
	}
	return styles.ActivePane.Width(m.width - 6).Height(contentH).Render(m.browserView.View().Content)
}

// ─── Header ─────────────────────────────────────────────────

func (m MainModel) renderHeader() string {
	breadcrumb := m.renderBreadcrumb()
	status := ""
	if m.running != actionNone {
		status = styles.WarningText.Render(fmt.Sprintf(" ⟳ %s", actionLabel(m.running)))
	} else if m.lastErr != nil {
		status = styles.ErrorText.Render(" ✗ Error")
	}
	return styles.Section.Render(breadcrumb + status) + "\n"
}

func (m MainModel) renderBreadcrumb() string {
	sep := styles.BreadcrumbSep.Render(" › ")
	parts := []string{styles.BreadcrumbItem.Render("work-bridge")}

	switch m.screen {
	case ScreenHub:
		parts = append(parts, styles.BreadcrumbActive.Render("Hub"))
	case ScreenProjects:
		parts = append(parts, styles.BreadcrumbActive.Render("Projects"))
	case ScreenSessions:
		if m.activeProjectRoot != "" {
			parts = append(parts, styles.BreadcrumbItem.Render(shortPath(m.activeProjectRoot)))
		}
		parts = append(parts, styles.BreadcrumbActive.Render("Sessions"))
	case ScreenHandoff:
		if m.selectedSession != nil {
			parts = append(parts, styles.BreadcrumbItem.Render("Sessions"))
			parts = append(parts, styles.BreadcrumbActive.Render(m.selectedSession.Title))
		} else {
			parts = append(parts, styles.BreadcrumbActive.Render("Handoff"))
		}
	case ScreenBrowser:
		parts = append(parts, styles.BreadcrumbActive.Render(m.browserTitle))
	case ScreenActionMenu:
		parts = append(parts, styles.BreadcrumbItem.Render(m.browserTitle))
		parts = append(parts, styles.BreadcrumbActive.Render(m.actionMenuView.EntryTitle()))
	}

	return strings.Join(parts, sep)
}

// ─── Footer ─────────────────────────────────────────────────

func (m MainModel) renderFooter() string {
	var help string
	switch m.screen {
	case ScreenHub:
		help = "↑↓: navigate • enter: select • /: commands • q: quit"
	case ScreenProjects:
		help = "↑↓: navigate • enter: select project • esc: back • /: commands"
	case ScreenSessions:
		help = "↑↓: navigate • enter: handoff • esc: back • /: commands"
	case ScreenHandoff:
		help = "↑↓: options • ←→: adjust • enter: confirm • esc: back"
	case ScreenBrowser:
		help = "↑↓: navigate • enter: open • esc: back"
	case ScreenActionMenu:
		help = "↑↓: navigate • enter: execute • esc: back"
	}

	footer := styles.Muted.Render(help)
	if m.lastErr != nil {
		footer = styles.ErrorBox.Width(m.width - 8).Render(m.lastErr.Error()) + "\n" + footer
	}
	return "\n" + footer
}

// ─── Helpers ────────────────────────────────────────────────

func actionLabel(action actionKind) string {
	switch action {
	case actionApply:
		return "applying"
	case actionExport:
		return "exporting"
	case actionPreview:
		return "previewing"
	case actionLoadWorkspace:
		return "loading workspace"
	case actionLoadProjects:
		return "scanning projects"
	case actionLoadSkills:
		return "loading skills"
	case actionLoadMCP:
		return "loading MCP"
	case actionMigrate:
		return "migrating"
	default:
		return "idle"
	}
}

func shortPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		return parts[len(parts)-1]
	}
	return path
}

func projectEntries(entries []catalog.ProjectEntry) []browser.Entry {
	out := make([]browser.Entry, 0, len(entries))
	for _, entry := range entries {
		markers := strings.Join(entry.Markers, ", ")
		desc := entry.Root
		if markers != "" {
			desc = entry.Root + "  [" + markers + "]"
		}
		out = append(out, browser.Entry{
			Key:         entry.Root,
			Title:       entry.Name,
			Description: desc,
			FilterValue: entry.Name,
		})
	}
	return out
}

func skillEntries(entries []catalog.SkillEntry) []browser.Entry {
	out := make([]browser.Entry, 0, len(entries))
	for _, entry := range entries {
		description := strings.TrimSpace(entry.Description)

		// Build a richer description line: scope + source info
		scope := ""
		if entry.Scope != "" {
			scope = "[" + entry.Scope + "] "
		}
		if description != "" {
			description = scope + description
		} else {
			description = scope + entry.Source
		}

		// Determine badge from catalog Tool, then fallback to path heuristics
		badge := entry.Tool
		if badge == "" {
			lower := strings.ToLower(entry.RootPath)
			switch {
			case strings.Contains(lower, "/.codex/") || strings.Contains(lower, "/codex/"):
				badge = "codex"
			case strings.Contains(lower, "/.claude/"):
				badge = "claude"
			case strings.Contains(lower, "/.gemini/"):
				badge = "gemini"
			case strings.Contains(lower, "opencode"):
				badge = "opencode"
			}
		}

		out = append(out, browser.Entry{
			Key:         entry.EntryPath,
			Title:       entry.Name,
			Description: description,
			Badge:       badge,
			FilterValue: entry.Name + " " + badge + " " + entry.Scope,
			Raw:         entry,
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
		
		badge := ""
		lowerName := strings.ToLower(entry.Name)
		if strings.Contains(lowerName, "codex") {
			badge = "codex"
		} else if strings.Contains(lowerName, "claude") {
			badge = "claude"
		} else if strings.Contains(lowerName, "gemini") {
			badge = "gemini"
		} else if strings.Contains(lowerName, "opencode") {
			badge = "opencode"
		}

		out = append(out, browser.Entry{
			Key:         entry.Path,
			Title:       entry.Name,
			Description: description,
			Badge:       badge,
			FilterValue: entry.Name,
			Raw:         entry,
		})
	}
	return out
}
