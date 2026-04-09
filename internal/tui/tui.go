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
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jaeyoung0509/work-bridge/internal/detect"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/inspect"
)

const (
	appName           = "work-bridge"
	defaultExportRoot = "work-bridge-export"
)

type Backend struct {
	LoadWorkspaceSnapshot func(context.Context) (WorkspaceSnapshot, error)
	ImportSession         func(context.Context, domain.Tool, string) (domain.SessionBundle, error)
	DoctorBundle          func(context.Context, domain.SessionBundle, domain.Tool) (domain.CompatibilityReport, error)
	ExportBundle          func(context.Context, domain.SessionBundle, domain.Tool, string) (domain.ExportManifest, error)
	InstallSkill          func(context.Context, SkillEntry, SkillTarget) (SkillInstallResult, error)
	ProbeMCP              func(context.Context, MCPEntry) (MCPProbeResult, error)
	DefaultExportDir      string
}

type WorkspaceSnapshot struct {
	Detect        detect.Report                  `json:"detect"`
	HomeDir       string                         `json:"home_dir,omitempty"`
	InspectByTool map[domain.Tool]inspect.Report `json:"inspect_by_tool"`
	Projects      []ProjectEntry                 `json:"projects"`
	Skills        []SkillEntry                   `json:"skills"`
	MCPProfiles   []MCPEntry                     `json:"mcp_profiles"`
	HealthSummary WorkspaceHealthSummary         `json:"health_summary"`
}

type WorkspaceHealthSummary struct {
	InstalledTools int `json:"installed_tools"`
	ProjectCount   int `json:"project_count"`
	SessionCount   int `json:"session_count"`
	SkillCount     int `json:"skill_count"`
	MCPCount       int `json:"mcp_count"`
	BrokenMCP      int `json:"broken_mcp"`
}

type ProjectEntry struct {
	Name          string         `json:"name"`
	Root          string         `json:"root"`
	WorkspaceRoot string         `json:"workspace_root"`
	Markers       []string       `json:"markers,omitempty"`
	SessionCount  int            `json:"session_count,omitempty"`
	SkillCount    int            `json:"skill_count,omitempty"`
	MCPCount      int            `json:"mcp_count,omitempty"`
	SessionByTool map[string]int `json:"session_by_tool,omitempty"`
}

type SkillEntry struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Path          string         `json:"path"`
	Source        string         `json:"source"`
	Scope         string         `json:"scope,omitempty"`
	Tool          domain.Tool    `json:"tool,omitempty"`
	GroupKey      string         `json:"group_key,omitempty"`
	ConflictState string         `json:"conflict_state,omitempty"`
	VariantCount  int            `json:"variant_count,omitempty"`
	ContentHash   string         `json:"content_hash,omitempty"`
	Content       string         `json:"content,omitempty"`
	Variants      []SkillVariant `json:"variants,omitempty"`
}

type SkillVariant struct {
	Path   string      `json:"path"`
	Scope  string      `json:"scope,omitempty"`
	Tool   domain.Tool `json:"tool,omitempty"`
	Source string      `json:"source,omitempty"`
}

type SkillTarget struct {
	ID         string      `json:"id"`
	Label      string      `json:"label"`
	Scope      string      `json:"scope"`
	Tool       domain.Tool `json:"tool,omitempty"`
	Path       string      `json:"path"`
	Exists     bool        `json:"exists,omitempty"`
	SameSource bool        `json:"same_source,omitempty"`
}

type MCPEntry struct {
	ID              string            `json:"id,omitempty"`
	Kind            string            `json:"kind,omitempty"`
	Name            string            `json:"name"`
	Path            string            `json:"path"`
	Source          string            `json:"source"`
	Scope           string            `json:"scope,omitempty"`
	Status          string            `json:"status"`
	Details         string            `json:"details"`
	Tool            domain.Tool       `json:"tool,omitempty"`
	Transport       string            `json:"transport,omitempty"`
	Format          string            `json:"format,omitempty"`
	DeclaredServers int               `json:"declared_servers,omitempty"`
	ServerNames     []string          `json:"server_names,omitempty"`
	ParseSource     string            `json:"parse_source,omitempty"`
	RawConfig       string            `json:"raw_config,omitempty"`
	ParseWarnings   []string          `json:"parse_warnings,omitempty"`
	BinaryFound     bool              `json:"binary_found"`
	BinaryPath      string            `json:"binary_path,omitempty"`
	HiddenScopes    []string          `json:"hidden_scopes,omitempty"`
	Servers         []MCPServerConfig `json:"servers,omitempty"`
	Declarations    []MCPDeclaration  `json:"declarations,omitempty"`
}

type MCPDeclaration struct {
	Label         string          `json:"label,omitempty"`
	Path          string          `json:"path"`
	Source        string          `json:"source"`
	Scope         string          `json:"scope,omitempty"`
	Status        string          `json:"status,omitempty"`
	Details       string          `json:"details,omitempty"`
	Format        string          `json:"format,omitempty"`
	ParseSource   string          `json:"parse_source,omitempty"`
	RawConfig     string          `json:"raw_config,omitempty"`
	ParseWarnings []string        `json:"parse_warnings,omitempty"`
	BinaryFound   bool            `json:"binary_found"`
	BinaryPath    string          `json:"binary_path,omitempty"`
	Server        MCPServerConfig `json:"server,omitempty"`
}

type MCPServerConfig struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Cwd       string            `json:"cwd,omitempty"`
	URL       string            `json:"url,omitempty"`
}

type SkillInstallResult struct {
	InstalledPath string   `json:"installed_path"`
	TargetID      string   `json:"target_id,omitempty"`
	TargetLabel   string   `json:"target_label,omitempty"`
	TargetScope   string   `json:"target_scope,omitempty"`
	Overwrote     bool     `json:"overwrote"`
	Warnings      []string `json:"warnings,omitempty"`
}

type MCPProbeResult struct {
	Reachable        bool                   `json:"reachable"`
	Latency          string                 `json:"latency,omitempty"`
	ResourceCount    int                    `json:"resource_count,omitempty"`
	TemplateCount    int                    `json:"template_count,omitempty"`
	ToolCount        int                    `json:"tool_count,omitempty"`
	PromptCount      int                    `json:"prompt_count,omitempty"`
	ConnectedServers int                    `json:"connected_servers,omitempty"`
	Warnings         []string               `json:"warnings,omitempty"`
	ProbedAt         string                 `json:"probed_at,omitempty"`
	Mode             string                 `json:"mode,omitempty"`
	ServerResults    []MCPServerProbeResult `json:"server_results,omitempty"`
}

type MCPServerProbeResult struct {
	Name            string   `json:"name"`
	Reachable       bool     `json:"reachable"`
	Latency         string   `json:"latency,omitempty"`
	Transport       string   `json:"transport,omitempty"`
	Command         string   `json:"command,omitempty"`
	ProtocolVersion string   `json:"protocol_version,omitempty"`
	ServerName      string   `json:"server_name,omitempty"`
	ServerVersion   string   `json:"server_version,omitempty"`
	ResourceCount   int      `json:"resource_count,omitempty"`
	TemplateCount   int      `json:"template_count,omitempty"`
	ToolCount       int      `json:"tool_count,omitempty"`
	PromptCount     int      `json:"prompt_count,omitempty"`
	Error           string   `json:"error,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

type workspaceSection int

const (
	sectionSessions workspaceSection = iota
	sectionProjects
	sectionSkills
	sectionMCP
	sectionLogs
)

type focusArea int

const (
	focusNav focusArea = iota
	focusList
	focusPreview
)

type layoutMode int

const (
	layoutWide layoutMode = iota
	layoutMedium
	layoutNarrow
)

type PreviewTab string

const (
	previewSummary  PreviewTab = "Summary"
	previewRaw      PreviewTab = "Raw"
	previewDoctor   PreviewTab = "Doctor"
	previewMetadata PreviewTab = "Metadata"
	previewContent  PreviewTab = "Content"
	previewLive     PreviewTab = "Live"
)

type taskState struct {
	kind  string
	label string
}

type snapshotLoadedMsg struct {
	snapshot WorkspaceSnapshot
}

type bundleImportedMsg struct {
	sessionKey string
	bundle     domain.SessionBundle
	autoDoctor bool
}

type doctorReadyMsg struct {
	sessionKey string
	target     domain.Tool
	bundle     *domain.SessionBundle
	report     domain.CompatibilityReport
}

type exportReadyMsg struct {
	sessionKey string
	target     domain.Tool
	bundle     domain.SessionBundle
	report     domain.CompatibilityReport
	manifest   domain.ExportManifest
}

type skillInstalledMsg struct {
	skill  SkillEntry
	result SkillInstallResult
}

type mcpProbedMsg struct {
	entry  MCPEntry
	result MCPProbeResult
}

type tickMsg struct{}

type errorMsg struct {
	err error
}

type sessionItem struct {
	Tool    domain.Tool
	Report  inspect.Report
	Session inspect.Session
}

type badge struct {
	label string
	tone  string
}

type listItem struct {
	title    string
	subtitle string
	badges   []badge
}

type rect struct {
	X int
	Y int
	W int
	H int
}

func (r rect) contains(x int, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

type previewTabHit struct {
	Tab   PreviewTab
	Start int
	End   int
}

type navHit struct {
	Section workspaceSection
	Start   int
	End     int
}

type workspaceLayout struct {
	mode    layoutMode
	nav     rect
	list    rect
	preview rect
}

type Model struct {
	backend Backend
	ctx     context.Context

	width  int
	height int

	activeSection workspaceSection
	focus         focusArea

	searchMode  bool
	searchQuery string
	showHelp    bool

	sessionIdx int
	projectIdx int
	skillIdx   int
	mcpIdx     int
	logIdx     int

	sessionListOffset int
	projectListOffset int
	skillListOffset   int
	mcpListOffset     int
	logListOffset     int

	sessionTabIdx int
	skillTabIdx   int
	mcpTabIdx     int

	sessionPreviewOffset int
	projectPreviewOffset int
	skillPreviewOffset   int
	mcpPreviewOffset     int
	logPreviewOffset     int

	targetIdx  int
	spinnerIdx int

	snapshot WorkspaceSnapshot
	task     taskState
	lastErr  error

	activeProjectRoot string

	bundleBySession   map[string]domain.SessionBundle
	doctorByKey       map[string]domain.CompatibilityReport
	exportByKey       map[string]domain.ExportManifest
	installByPath     map[string]SkillInstallResult
	probeByID         map[string]MCPProbeResult
	skillTargetByPath map[string]string

	logs []string
}

var (
	toolOrder          = []domain.Tool{domain.ToolCodex, domain.ToolGemini, domain.ToolClaude, domain.ToolOpenCode}
	sectionOrder       = []workspaceSection{sectionSessions, sectionProjects, sectionSkills, sectionMCP, sectionLogs}
	sessionPreviewTabs = []PreviewTab{previewSummary, previewRaw, previewDoctor}
	skillPreviewTabs   = []PreviewTab{previewMetadata, previewContent}
	mcpPreviewTabs     = []PreviewTab{previewSummary, previewRaw, previewLive}
)

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
		backend:           backend,
		ctx:               ctx,
		activeSection:     sectionSessions,
		focus:             focusList,
		bundleBySession:   map[string]domain.SessionBundle{},
		doctorByKey:       map[string]domain.CompatibilityReport{},
		exportByKey:       map[string]domain.ExportManifest{},
		installByPath:     map[string]SkillInstallResult{},
		probeByID:         map[string]MCPProbeResult{},
		skillTargetByPath: map[string]string{},
		logs:              []string{"workspace boot requested"},
	}
}

func (m Model) Init() tea.Cmd {
	m.task = taskState{kind: "load", label: "Loading workspace"}
	return tea.Batch(m.loadWorkspaceCmd(), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureVisibleSelection()
		return m, nil
	case tickMsg:
		if m.task.kind == "" {
			return m, nil
		}
		m.spinnerIdx = (m.spinnerIdx + 1) % 4
		return m, tickCmd()
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case snapshotLoadedMsg:
		m.snapshot = msg.snapshot
		m.reconcileActiveProject()
		m.task = taskState{}
		m.lastErr = nil
		m.logs = append(m.logs, fmt.Sprintf("workspace loaded: %d sessions, %d skills, %d mcp", m.snapshot.HealthSummary.SessionCount, m.snapshot.HealthSummary.SkillCount, m.snapshot.HealthSummary.MCPCount))
		m.ensureValidSelection()
		m.ensureSectionWithContent()
		m.ensureVisibleSelection()
		return m, nil
	case bundleImportedMsg:
		m.bundleBySession[msg.sessionKey] = msg.bundle
		m.task = taskState{}
		m.lastErr = nil
		m.logs = append(m.logs, fmt.Sprintf("imported %s", msg.sessionKey))
		if msg.autoDoctor {
			if item, ok := m.selectedSession(); ok && sessionKeyFor(item.Tool, item.Session.ID) == msg.sessionKey {
				target := m.targetTool()
				m.task = taskState{kind: "doctor", label: fmt.Sprintf("Doctor %s -> %s", item.Session.ID, target)}
				return m, tea.Batch(m.doctorSelectedCmd(), tickCmd())
			}
		}
		return m, nil
	case doctorReadyMsg:
		if msg.bundle != nil {
			m.bundleBySession[msg.sessionKey] = *msg.bundle
		}
		m.doctorByKey[doctorKey(msg.sessionKey, msg.target)] = msg.report
		m.task = taskState{}
		m.lastErr = nil
		m.logs = append(m.logs, fmt.Sprintf("doctor %s -> %s: compatible=%d partial=%d unsupported=%d", msg.sessionKey, msg.target, len(msg.report.CompatibleFields), len(msg.report.PartialFields), len(msg.report.UnsupportedFields)))
		return m, nil
	case exportReadyMsg:
		m.bundleBySession[msg.sessionKey] = msg.bundle
		m.doctorByKey[doctorKey(msg.sessionKey, msg.target)] = msg.report
		m.exportByKey[doctorKey(msg.sessionKey, msg.target)] = msg.manifest
		m.task = taskState{}
		m.lastErr = nil
		m.logs = append(m.logs, fmt.Sprintf("exported %s -> %s at %s", msg.sessionKey, msg.target, msg.manifest.OutputDir))
		return m, nil
	case skillInstalledMsg:
		m.installByPath[msg.skill.Path] = msg.result
		m.task = taskState{kind: "load", label: "Refreshing workspace"}
		m.lastErr = nil
		m.logs = append(m.logs, fmt.Sprintf("synced skill %s -> %s", msg.skill.Name, shortPath(msg.result.InstalledPath)))
		return m, tea.Batch(m.loadWorkspaceCmd(), tickCmd())
	case mcpProbedMsg:
		m.probeByID[mcpEntryKey(msg.entry)] = msg.result
		m.task = taskState{}
		m.lastErr = nil
		state := "unreachable"
		if msg.result.Reachable {
			state = "reachable"
		}
		m.logs = append(m.logs, fmt.Sprintf("probed %s: %s", msg.entry.Name, state))
		return m, nil
	case errorMsg:
		m.task = taskState{}
		m.lastErr = msg.err
		m.logs = append(m.logs, "error: "+msg.err.Error())
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.searchMode {
		switch key {
		case "esc":
			m.searchMode = false
			return m, nil
		case "enter":
			m.searchMode = false
			m.ensureValidSelection()
			m.ensureVisibleSelection()
			return m, nil
		case "backspace", "ctrl+h":
			if len(m.searchQuery) > 0 {
				m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				m.ensureValidSelection()
				m.ensureVisibleSelection()
			}
			return m, nil
		default:
			if (len(key) == 1 && key != " ") || key == "space" {
				if key == "space" {
					m.searchQuery += " "
				} else {
					m.searchQuery += key
				}
				m.ensureValidSelection()
				m.ensureVisibleSelection()
			}
			return m, nil
		}
	}

	if m.showHelp {
		switch key {
		case "?", "esc", "q":
			m.showHelp = false
			return m, nil
		}
	}

	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		return m, tea.Quit
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "/":
		m.searchMode = true
		return m, nil
	case "esc":
		if m.searchQuery != "" {
			m.searchQuery = ""
			m.ensureValidSelection()
			m.ensureVisibleSelection()
			return m, nil
		}
		if m.activeProjectRoot != "" {
			m.clearActiveProject()
		}
		return m, nil
	case "c":
		if m.activeProjectRoot != "" {
			m.clearActiveProject()
		}
		return m, nil
	case "tab":
		m.focus = (m.focus + 1) % 3
		return m, nil
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
		return m, nil
	case "r":
		m.task = taskState{kind: "load", label: "Refreshing workspace"}
		return m, tea.Batch(m.loadWorkspaceCmd(), tickCmd())
	case "t":
		if m.activeSection == sectionSkills {
			m.moveSkillTarget(1)
			return m, nil
		}
		m.targetIdx = (m.targetIdx + 1) % len(toolOrder)
		return m, nil
	case "T":
		if m.activeSection == sectionSkills {
			m.moveSkillTarget(-1)
			return m, nil
		}
		m.targetIdx = (m.targetIdx + len(toolOrder) - 1) % len(toolOrder)
		return m, nil
	case "[":
		m.movePreviewTab(-1)
		return m, nil
	case "]":
		m.movePreviewTab(1)
		return m, nil
	case "left", "h":
		if m.focus == focusPreview {
			m.movePreviewTab(-1)
			return m, nil
		}
	case "right", "l":
		if m.focus == focusPreview {
			m.movePreviewTab(1)
			return m, nil
		}
	case "up", "k":
		m.moveSelection(-1)
		return m, nil
	case "down", "j":
		m.moveSelection(1)
		return m, nil
	case "enter":
		if m.layoutMode() == layoutNarrow {
			if m.focus == focusList {
				m.focus = focusPreview
			} else {
				m.focus = focusList
			}
			return m, nil
		}
		if m.focus == focusNav {
			m.focus = focusList
		} else if m.focus == focusList {
			m.focus = focusPreview
		} else {
			m.focus = focusList
		}
		return m, nil
	case "i":
		if m.activeSection == sectionSessions {
			if _, ok := m.selectedSession(); ok {
				m.task = taskState{kind: "import", label: "Importing session"}
				return m, tea.Batch(m.importSelectedCmd(), tickCmd())
			}
		}
	case "d":
		if m.activeSection == sectionSessions {
			if _, ok := m.selectedSession(); ok {
				m.task = taskState{kind: "doctor", label: fmt.Sprintf("Doctor -> %s", m.targetTool())}
				return m, tea.Batch(m.doctorSelectedCmd(), tickCmd())
			}
		}
	case "e":
		if m.activeSection == sectionSessions {
			if _, ok := m.selectedSession(); ok {
				m.task = taskState{kind: "export", label: fmt.Sprintf("Export -> %s", m.targetTool())}
				return m, tea.Batch(m.exportSelectedCmd(), tickCmd())
			}
		}
	case "I":
		if m.activeSection == sectionSkills {
			if _, ok := m.selectedSkill(); ok {
				m.task = taskState{kind: "skill-sync", label: "Syncing skill"}
				return m, tea.Batch(m.installSelectedSkillCmd(), tickCmd())
			}
		}
	case "p":
		if m.activeSection == sectionMCP {
			if _, ok := m.selectedMCP(); ok {
				m.task = taskState{kind: "mcp-probe", label: "Validating MCP config"}
				return m, tea.Batch(m.probeSelectedMCP(), tickCmd())
			}
		}
	}

	return m, nil
}

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	mouse := tea.Mouse(msg)
	layout := m.currentLayout()

	if layout.mode == layoutNarrow && m.focus == focusPreview && layout.preview.contains(mouse.X, mouse.Y) {
		if tab, ok := m.hitPreviewTab(layout, mouse.X, mouse.Y); ok {
			m.setPreviewTab(tab)
		}
		return m, nil
	}

	if layout.nav.contains(mouse.X, mouse.Y) {
		m.focus = focusNav
		if section, ok := m.hitNav(layout, mouse.X, mouse.Y); ok {
			m.setActiveSection(section)
		}
		return m, nil
	}

	if layout.list.contains(mouse.X, mouse.Y) {
		m.focus = focusList
		if index, ok := m.hitList(layout, mouse.X, mouse.Y); ok {
			m.selectVisibleIndex(index)
		}
		return m, nil
	}

	if layout.preview.contains(mouse.X, mouse.Y) {
		m.focus = focusPreview
		if tab, ok := m.hitPreviewTab(layout, mouse.X, mouse.Y); ok {
			m.setPreviewTab(tab)
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	mouse := tea.Mouse(msg)
	layout := m.currentLayout()
	delta := 0
	switch mouse.Button {
	case tea.MouseWheelUp:
		delta = -1
	case tea.MouseWheelDown:
		delta = 1
	default:
		return m, nil
	}

	if layout.mode == layoutNarrow && m.focus == focusPreview && layout.preview.contains(mouse.X, mouse.Y) {
		m.focus = focusPreview
		m.scrollPreview(delta * 3)
		return m, nil
	}

	if layout.list.contains(mouse.X, mouse.Y) {
		m.focus = focusList
		m.moveSelection(delta)
		return m, nil
	}

	if layout.preview.contains(mouse.X, mouse.Y) {
		m.focus = focusPreview
		m.scrollPreview(delta * 3)
		return m, nil
	}

	return m, nil
}

func (m Model) View() tea.View {
	view := tea.NewView("")
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = appName
	if m.width == 0 || m.height == 0 {
		view.SetContent("loading " + appName + " workspace...")
		return view
	}

	m.ensureVisibleSelection()

	body := lipgloss.JoinVertical(lipgloss.Left,
		m.renderHeader(),
		m.renderBody(),
		m.renderStatusBar(),
	)
	if m.showHelp {
		body = lipgloss.JoinVertical(lipgloss.Left, body, m.renderHelp())
	}
	view.SetContent(body)
	return view
}

func (m Model) renderHeader() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45")).Render(appName)
	project := "project: n/a"
	if m.snapshot.Detect.ProjectRoot != "" {
		project = "project: " + shortPath(m.snapshot.Detect.ProjectRoot)
	}
	subtitle := lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Render(project)
	pills := []string{
		badgeStyle("section "+m.activeSectionName(), "accent"),
		badgeStyle("focus "+m.focusName(), "muted"),
		badgeStyle("target "+string(m.targetTool()), "warning"),
	}
	if m.searchQuery != "" {
		pills = append(pills, badgeStyle("search "+m.searchQuery, "muted"))
	}
	scopeLabel := "scope all-projects"
	if item, ok := m.activeProject(); ok {
		scopeLabel = "scope " + firstNonEmpty(item.Name, shortPath(item.Root))
	}
	pills = append(pills, badgeStyle(scopeLabel, "muted"))
	if m.activeSection == sectionSkills {
		if item, ok := m.selectedSkill(); ok {
			if target, ok := m.selectedSkillTarget(item); ok {
				pills = append(pills, badgeStyle("copy "+target.Label, "warning"))
			}
		}
	}
	if m.task.kind != "" {
		pills = append(pills, badgeStyle(m.spinnerFrame()+" "+m.task.label, "success"))
	}
	if m.lastErr != nil {
		pills = append(pills, badgeStyle(m.lastErr.Error(), "error"))
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", subtitle),
		lipgloss.JoinHorizontal(lipgloss.Left, pills...),
	)
}

func (m Model) renderBody() string {
	layout := m.currentLayout()

	switch layout.mode {
	case layoutWide:
		return lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderPanel("Workspace", m.renderNav(layout.nav), layout.nav.W, layout.nav.H, m.focus == focusNav),
			m.renderPanel(m.listPanelTitle(), m.renderList(layout.list), layout.list.W, layout.list.H, m.focus == focusList),
			m.renderPanel(m.previewPanelTitle(), m.renderPreview(layout.preview), layout.preview.W, layout.preview.H, m.focus == focusPreview),
		)
	case layoutMedium:
		right := lipgloss.JoinVertical(lipgloss.Left,
			m.renderPanel(m.listPanelTitle(), m.renderList(layout.list), layout.list.W, layout.list.H, m.focus == focusList),
			m.renderPanel(m.previewPanelTitle(), m.renderPreview(layout.preview), layout.preview.W, layout.preview.H, m.focus == focusPreview),
		)
		return lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderPanel("Workspace", m.renderNav(layout.nav), layout.nav.W, layout.nav.H, m.focus == focusNav),
			right,
		)
	default:
		nav := m.renderPanel("Workspace", m.renderNav(layout.nav), layout.nav.W, layout.nav.H, m.focus == focusNav)
		title := m.listPanelTitle()
		content := m.renderList(layout.list)
		active := m.focus != focusPreview
		if m.focus == focusPreview {
			title = m.previewPanelTitle()
			content = m.renderPreview(layout.preview)
			active = true
		}
		panel := m.renderPanel(title, content, layout.list.W, layout.list.H, active)
		return lipgloss.JoinVertical(lipgloss.Left, nav, panel)
	}
}

func (m Model) renderStatusBar() string {
	scope := "all projects"
	if item, ok := m.activeProject(); ok {
		scope = firstNonEmpty(item.Name, shortPath(item.Root))
	}
	left := []string{
		fmt.Sprintf("%d sessions", len(m.filteredSessions())),
		fmt.Sprintf("%d projects", len(m.filteredProjects())),
		fmt.Sprintf("%d skills", len(m.filteredSkills())),
		fmt.Sprintf("%d mcp", len(m.filteredMCP())),
		"scope: " + scope,
	}
	if m.searchMode {
		left = append(left, "search typing...")
	}
	right := m.contextActions()
	bar := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)
	return bar.Width(maxInt(0, m.width)).Render(strings.Join(left, " | ") + "    " + right)
}

func (m Model) renderHelp() string {
	lines := []string{
		"Keys",
		"tab / shift+tab  move focus",
		"j/k or up/down   move selection",
		"/                filter current section",
		"[ / ]            switch preview tab",
		"t / T            cycle target",
		"r                refresh workspace",
		"mouse click      focus pane / choose item / switch tab",
		"mouse wheel      scroll list selection or preview content",
		"c                clear active project scope",
		"i                import session",
		"d                doctor selected session",
		"e                export selected session",
		"I                sync selected skill",
		"p                validate selected mcp config",
		"? or esc         close help",
		"q                quit",
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("45")).
		Padding(0, 1)
	return style.Width(maxInt(48, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m Model) renderPanel(title string, body string, width int, height int, focused bool) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(0, 1).
		Width(maxInt(12, width)).
		Height(maxInt(4, height))
	if focused {
		style = style.BorderForeground(lipgloss.Color("45"))
	}
	bodyLines := maxInt(1, height-3)
	return style.Render(title + "\n" + fitLines(body, bodyLines))
}

func (m Model) renderNav(panel rect) string {
	if m.layoutMode() == layoutNarrow {
		return m.renderNavCompact()
	}
	lines := make([]string, 0, len(sectionOrder)+4)
	lines = append(lines,
		fmt.Sprintf("cwd: %s", shortPath(m.snapshot.Detect.CWD)),
		fmt.Sprintf("root: %s", shortPath(m.snapshot.Detect.ProjectRoot)),
		"",
	)
	for _, section := range sectionOrder {
		prefix := "  "
		if section == m.activeSection {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s", prefix, m.navLabel(section)))
	}
	if m.snapshot.HealthSummary.BrokenMCP > 0 {
		lines = append(lines, "", fmt.Sprintf("degraded mcp: %d", m.snapshot.HealthSummary.BrokenMCP))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderNavCompact() string {
	segments := make([]string, 0, len(sectionOrder))
	for _, section := range sectionOrder {
		label := m.navLabel(section)
		tone := "muted"
		if section == m.activeSection {
			tone = "accent"
		}
		segments = append(segments, badgeStyle(label, tone))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, segments...)
}

func (m Model) navLabel(section workspaceSection) string {
	switch section {
	case sectionSessions:
		return fmt.Sprintf("Sessions %d", len(m.filteredSessions()))
	case sectionProjects:
		return fmt.Sprintf("Projects %d", len(m.filteredProjects()))
	case sectionSkills:
		return fmt.Sprintf("Skills %d", len(m.filteredSkills()))
	case sectionMCP:
		return fmt.Sprintf("MCP %d", len(m.filteredMCP()))
	default:
		return fmt.Sprintf("Logs %d", len(m.filteredLogs()))
	}
}

func (m Model) listPanelTitle() string {
	switch m.activeSection {
	case sectionSessions:
		return "Sessions"
	case sectionProjects:
		return "Projects"
	case sectionSkills:
		return "Skills"
	case sectionMCP:
		return "MCP"
	default:
		return "Logs"
	}
}

func (m Model) previewPanelTitle() string {
	switch m.activeSection {
	case sectionSessions:
		return "Preview " + m.previewTabLabel()
	case sectionProjects:
		return "Project"
	case sectionSkills:
		return "Skill " + m.previewTabLabel()
	case sectionMCP:
		return "MCP " + m.previewTabLabel()
	default:
		return "Log Detail"
	}
}

func (m Model) renderList(panel rect) string {
	bodyHeight := panelBodyHeight(panel)
	switch m.activeSection {
	case sectionSessions:
		return m.renderSessionList(bodyHeight)
	case sectionProjects:
		return m.renderProjectList(bodyHeight)
	case sectionSkills:
		return m.renderSkillList(bodyHeight)
	case sectionMCP:
		return m.renderMCPList(bodyHeight)
	default:
		return m.renderLogList(bodyHeight)
	}
}

func (m Model) renderSessionList(height int) string {
	items := m.filteredSessions()
	if len(items) == 0 {
		return "no sessions"
	}
	offset, visible := m.listWindow(len(items), 2, height, m.sessionIdx, m.sessionListOffset)
	m.sessionListOffset = offset
	lines := make([]string, 0, visible*2)
	for i := 0; i < visible; i++ {
		item := items[offset+i]
		entry := m.sessionListItem(item)
		prefix := "  "
		if offset+i == clampIndex(m.sessionIdx, len(items)) {
			prefix = "> "
		}
		lines = append(lines, prefix+entry.title+"  "+renderBadges(entry.badges))
		lines = append(lines, "  "+firstNonEmpty(entry.subtitle, " "))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderProjectList(height int) string {
	items := m.filteredProjects()
	if len(items) == 0 {
		return "no projects"
	}
	offset, visible := m.listWindow(len(items), 2, height, m.projectIdx, m.projectListOffset)
	m.projectListOffset = offset
	lines := make([]string, 0, visible*2)
	for i := 0; i < visible; i++ {
		item := items[offset+i]
		entry := m.projectListItem(item)
		prefix := "  "
		if offset+i == clampIndex(m.projectIdx, len(items)) {
			prefix = "> "
		}
		lines = append(lines, prefix+entry.title+"  "+renderBadges(entry.badges))
		lines = append(lines, "  "+firstNonEmpty(entry.subtitle, " "))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderSkillList(height int) string {
	items := m.filteredSkills()
	if len(items) == 0 {
		return "no skills"
	}
	offset, visible := m.listWindow(len(items), 2, height, m.skillIdx, m.skillListOffset)
	m.skillListOffset = offset
	lines := make([]string, 0, visible*2)
	for i := 0; i < visible; i++ {
		item := items[offset+i]
		entry := m.skillListItem(item)
		prefix := "  "
		if offset+i == clampIndex(m.skillIdx, len(items)) {
			prefix = "> "
		}
		lines = append(lines, prefix+entry.title+"  "+renderBadges(entry.badges))
		lines = append(lines, "  "+firstNonEmpty(entry.subtitle, " "))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderMCPList(height int) string {
	items := m.filteredMCP()
	if len(items) == 0 {
		return "no mcp profiles"
	}
	offset, visible := m.listWindow(len(items), 2, height, m.mcpIdx, m.mcpListOffset)
	m.mcpListOffset = offset
	lines := make([]string, 0, visible*2)
	for i := 0; i < visible; i++ {
		item := items[offset+i]
		entry := m.mcpListItem(item)
		prefix := "  "
		if offset+i == clampIndex(m.mcpIdx, len(items)) {
			prefix = "> "
		}
		lines = append(lines, prefix+entry.title+"  "+renderBadges(entry.badges))
		lines = append(lines, "  "+firstNonEmpty(entry.subtitle, " "))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderLogList(height int) string {
	items := m.filteredLogs()
	if len(items) == 0 {
		return "idle"
	}
	offset, visible := m.listWindow(len(items), 1, height, m.logIdx, m.logListOffset)
	m.logListOffset = offset
	lines := make([]string, 0, visible)
	for i := 0; i < visible; i++ {
		prefix := "  "
		if offset+i == clampIndex(m.logIdx, len(items)) {
			prefix = "> "
		}
		lines = append(lines, prefix+items[offset+i])
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderPreview(panel rect) string {
	bodyHeight := panelBodyHeight(panel)
	if m.activeSection == sectionLogs || m.activeSection == sectionProjects {
		content := m.renderLogPreview()
		if m.activeSection == sectionProjects {
			content = m.renderProjectPreview()
		}
		lines, offset := m.previewLines(bodyHeight, content)
		m.setPreviewOffset(offset)
		return strings.Join(lines, "\n")
	}

	tabLine, _ := m.renderPreviewTabs()
	contentHeight := maxInt(0, bodyHeight-1)
	lines, offset := m.previewLines(contentHeight, m.previewContent())
	m.setPreviewOffset(offset)
	if contentHeight == 0 {
		return tabLine
	}
	return tabLine + "\n" + strings.Join(lines, "\n")
}

func (m Model) renderSessionPreview() string {
	item, ok := m.selectedSession()
	if !ok {
		return "Select a session."
	}
	key := sessionKeyFor(item.Tool, item.Session.ID)
	bundle, hasBundle := m.bundleBySession[key]
	report, hasDoctor := m.doctorByKey[doctorKey(key, m.targetTool())]
	manifest, hasExport := m.exportByKey[doctorKey(key, m.targetTool())]

	switch m.currentPreviewTab() {
	case previewRaw:
		payload := map[string]any{
			"tool":    item.Tool,
			"session": item.Session,
			"notes":   item.Report.Notes,
		}
		if hasBundle {
			payload["bundle"] = bundle
		}
		return mustJSON(payload)
	case previewDoctor:
		if !hasDoctor {
			return "Doctor report not generated yet. Press d to analyze the selected session."
		}
		return mustJSON(report)
	default:
		lines := []string{
			fmt.Sprintf("Title: %s", firstNonEmpty(item.Session.Title, item.Session.ID)),
			fmt.Sprintf("Tool: %s", item.Tool),
			fmt.Sprintf("Session ID: %s", item.Session.ID),
			fmt.Sprintf("Project: %s", shortPath(item.Session.ProjectRoot)),
			fmt.Sprintf("Updated: %s", firstNonEmpty(item.Session.UpdatedAt, "unknown")),
			fmt.Sprintf("Started: %s", firstNonEmpty(item.Session.StartedAt, "unknown")),
			fmt.Sprintf("Storage: %s", shortPath(item.Session.StoragePath)),
			fmt.Sprintf("Messages: %d", item.Session.MessageCount),
		}
		if hasBundle {
			lines = append(lines, "",
				"Imported Bundle",
				fmt.Sprintf("Bundle ID: %s", firstNonEmpty(bundle.BundleID, "(not assigned)")),
				fmt.Sprintf("Goal: %s", firstNonEmpty(bundle.CurrentGoal, "(none)")),
				fmt.Sprintf("Touched files: %d", len(bundle.TouchedFiles)),
				fmt.Sprintf("Decisions: %d", len(bundle.Decisions)),
				fmt.Sprintf("Warnings: %d", len(bundle.Warnings)),
			)
		}
		if hasDoctor {
			lines = append(lines, "",
				"Doctor",
				fmt.Sprintf("Compatible: %d", len(report.CompatibleFields)),
				fmt.Sprintf("Partial: %d", len(report.PartialFields)),
				fmt.Sprintf("Unsupported: %d", len(report.UnsupportedFields)),
			)
		}
		if hasExport {
			lines = append(lines, "",
				"Last Export",
				fmt.Sprintf("Output: %s", shortPath(manifest.OutputDir)),
				fmt.Sprintf("Files: %d", len(manifest.Files)),
			)
		}
		if len(item.Report.Notes) > 0 {
			lines = append(lines, "", "Inspect Notes")
			for _, note := range item.Report.Notes {
				lines = append(lines, "- "+note)
			}
		}
		return strings.Join(lines, "\n")
	}
}

func (m Model) renderSkillPreview() string {
	item, ok := m.selectedSkill()
	if !ok {
		return "Select a skill."
	}
	install, hasInstall := m.installByPath[item.Path]
	target, hasTarget := m.selectedSkillTarget(item)
	switch m.currentPreviewTab() {
	case previewContent:
		if strings.TrimSpace(item.Content) == "" {
			return "Skill content unavailable."
		}
		return item.Content
	default:
		lines := []string{
			fmt.Sprintf("Name: %s", item.Name),
			fmt.Sprintf("Scope: %s", firstNonEmpty(item.Scope, "project")),
			fmt.Sprintf("Tool: %s", firstNonEmpty(string(item.Tool), "shared")),
			fmt.Sprintf("Source: %s", item.Source),
			fmt.Sprintf("Path: %s", shortPath(item.Path)),
			fmt.Sprintf("Conflict: %s", firstNonEmpty(item.ConflictState, "none")),
			fmt.Sprintf("Variants: %d", maxInt(1, item.VariantCount)),
			fmt.Sprintf("Description: %s", firstNonEmpty(item.Description, "(none)")),
		}
		if hasTarget {
			action := "create"
			switch {
			case target.SameSource:
				action = "noop"
			case target.Exists:
				action = "overwrite"
			}
			lines = append(lines, "",
				"Selected Target",
				fmt.Sprintf("Label: %s", target.Label),
				fmt.Sprintf("Scope: %s", target.Scope),
				fmt.Sprintf("Path: %s", shortPath(target.Path)),
				fmt.Sprintf("Action: %s", action),
			)
		}
		if len(item.Variants) > 0 {
			lines = append(lines, "", "Variants")
			for _, variant := range item.Variants {
				lines = append(lines, fmt.Sprintf("- %s | %s | %s", firstNonEmpty(variant.Scope, "project"), firstNonEmpty(string(variant.Tool), "shared"), shortPath(variant.Path)))
			}
		}
		if hasInstall {
			lines = append(lines, "",
				"Last Sync",
				fmt.Sprintf("Target: %s", firstNonEmpty(install.TargetLabel, install.TargetScope)),
				fmt.Sprintf("Installed path: %s", shortPath(install.InstalledPath)),
				fmt.Sprintf("Overwrote: %t", install.Overwrote),
			)
			for _, warning := range install.Warnings {
				lines = append(lines, "- "+warning)
			}
		}
		return strings.Join(lines, "\n")
	}
}

func (m Model) renderProjectPreview() string {
	item, ok := m.selectedProject()
	if !ok {
		return "Select a project."
	}
	scopeState := "inactive"
	if samePath(item.Root, m.activeProjectRoot) {
		scopeState = "active"
	}
	lines := []string{
		fmt.Sprintf("Name: %s", item.Name),
		fmt.Sprintf("Root: %s", shortPath(item.Root)),
		fmt.Sprintf("Workspace root: %s", shortPath(item.WorkspaceRoot)),
		fmt.Sprintf("Scope: %s", scopeState),
		fmt.Sprintf("Sessions: %d", item.SessionCount),
		fmt.Sprintf("Skills: %d", item.SkillCount),
		fmt.Sprintf("MCP configs: %d", item.MCPCount),
	}
	if len(item.SessionByTool) > 0 {
		lines = append(lines, "", "Sessions by Tool")
		for _, tool := range toolOrder {
			if count := item.SessionByTool[string(tool)]; count > 0 {
				lines = append(lines, fmt.Sprintf("- %s: %d", tool, count))
			}
		}
	}
	if len(item.Markers) > 0 {
		lines = append(lines, "", "Markers")
		for _, marker := range item.Markers {
			lines = append(lines, "- "+marker)
		}
	}
	lines = append(lines, "", "Controls", "- select in Projects to scope workspace", "- click active project again or press c to clear scope")
	return strings.Join(lines, "\n")
}

func (m Model) renderMCPPreview() string {
	item, ok := m.selectedMCP()
	if !ok {
		return "Select an MCP server."
	}
	probe, hasProbe := m.probeByID[mcpEntryKey(item)]
	switch m.currentPreviewTab() {
	case previewRaw:
		blocks := []string{}
		for _, decl := range item.Declarations {
			if strings.TrimSpace(decl.RawConfig) == "" {
				continue
			}
			header := fmt.Sprintf("# %s | %s | %s", firstNonEmpty(decl.Scope, decl.Source), firstNonEmpty(decl.Label, filepath.Base(decl.Path)), shortPath(decl.Path))
			blocks = append(blocks, header, decl.RawConfig)
		}
		if len(blocks) > 0 {
			return strings.Join(blocks, "\n\n")
		}
		if strings.TrimSpace(item.RawConfig) == "" {
			return "Config content unavailable."
		}
		return item.RawConfig
	case previewLive:
		if !hasProbe {
			return "No validation result yet. Press p to validate the selected MCP server."
		}
		return mustJSON(probe)
	default:
		kind := firstNonEmpty(item.Kind, "server")
		lines := []string{
			fmt.Sprintf("Name: %s", item.Name),
			fmt.Sprintf("Kind: %s", kind),
			fmt.Sprintf("Tool: %s", firstNonEmpty(string(item.Tool), "(shared)")),
			fmt.Sprintf("Effective scope: %s", firstNonEmpty(item.Scope, "(none)")),
			fmt.Sprintf("Source: %s", item.Source),
			fmt.Sprintf("Path: %s", shortPath(item.Path)),
			fmt.Sprintf("Status: %s", item.Status),
			fmt.Sprintf("Transport: %s", firstNonEmpty(item.Transport, "(unknown)")),
			fmt.Sprintf("Format: %s", firstNonEmpty(item.Format, "(unknown)")),
			fmt.Sprintf("Parse source: %s", firstNonEmpty(item.ParseSource, "(none)")),
			fmt.Sprintf("Declarations: %d", maxInt(len(item.Declarations), item.DeclaredServers)),
			fmt.Sprintf("Tool binary: %s", boolLabel(item.BinaryFound, firstNonEmpty(item.BinaryPath, "found"), "missing")),
			fmt.Sprintf("Details: %s", firstNonEmpty(item.Details, "(none)")),
		}
		if len(item.HiddenScopes) > 0 {
			lines = append(lines, "", "Hidden / Overridden")
			for _, hidden := range item.HiddenScopes {
				lines = append(lines, "- "+hidden)
			}
		}
		if len(item.ParseWarnings) > 0 {
			lines = append(lines, "", "Parse Warnings")
			for _, warning := range item.ParseWarnings {
				lines = append(lines, "- "+warning)
			}
		}
		if len(item.Declarations) > 0 {
			lines = append(lines, "", "Declarations")
			for i, decl := range item.Declarations {
				prefix := "hidden"
				if i == 0 {
					prefix = "effective"
				}
				summary := fmt.Sprintf("- %s | %s | %s | %s", prefix, firstNonEmpty(decl.Scope, decl.Source), firstNonEmpty(decl.Label, "(config)"), shortPath(decl.Path))
				if decl.Server.Name != "" {
					summary += fmt.Sprintf(" | transport=%s", firstNonEmpty(decl.Server.Transport, "stdio"))
				}
				lines = append(lines, summary)
				for _, warning := range decl.ParseWarnings {
					lines = append(lines, "  warning: "+warning)
				}
			}
		}
		if hasProbe {
			lines = append(lines, "",
				"Validation",
				fmt.Sprintf("Mode: %s", firstNonEmpty(probe.Mode, "config-only")),
				fmt.Sprintf("Reachable: %t", probe.Reachable),
				fmt.Sprintf("Connected servers: %d", probe.ConnectedServers),
				fmt.Sprintf("Latency: %s", firstNonEmpty(probe.Latency, "n/a")),
				fmt.Sprintf("Resources: %d", probe.ResourceCount),
				fmt.Sprintf("Templates: %d", probe.TemplateCount),
				fmt.Sprintf("Tools: %d", probe.ToolCount),
				fmt.Sprintf("Prompts: %d", probe.PromptCount),
			)
			for _, warning := range probe.Warnings {
				lines = append(lines, "- "+warning)
			}
		}
		return strings.Join(lines, "\n")
	}
}

func (m Model) renderLogPreview() string {
	items := m.filteredLogs()
	if len(items) == 0 {
		return "idle"
	}
	idx := clampIndex(m.logIdx, len(items))
	return items[idx]
}

func (m Model) previewContent() string {
	switch m.activeSection {
	case sectionSessions:
		return m.renderSessionPreview()
	case sectionProjects:
		return m.renderProjectPreview()
	case sectionSkills:
		return m.renderSkillPreview()
	case sectionMCP:
		return m.renderMCPPreview()
	default:
		return m.renderLogPreview()
	}
}

func (m Model) renderPreviewTabs() (string, []previewTabHit) {
	if m.activeSection == sectionLogs || m.activeSection == sectionProjects {
		return "", nil
	}
	tabs := m.currentSectionTabs()
	active := m.currentPreviewTab()
	parts := make([]string, 0, len(tabs))
	hits := make([]previewTabHit, 0, len(tabs))
	x := 0
	for _, tab := range tabs {
		label := "[" + string(tab) + "]"
		tone := "muted"
		if tab == active {
			tone = "accent"
		}
		rendered := badgeStyle(label, tone)
		parts = append(parts, rendered)
		width := lipgloss.Width(rendered)
		hits = append(hits, previewTabHit{Tab: tab, Start: x, End: x + width})
		x += width
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...), hits
}

func (m Model) loadWorkspaceCmd() tea.Cmd {
	if m.backend.LoadWorkspaceSnapshot == nil {
		return nil
	}
	return func() tea.Msg {
		snapshot, err := m.backend.LoadWorkspaceSnapshot(m.ctx)
		if err != nil {
			return errorMsg{err: err}
		}
		return snapshotLoadedMsg{snapshot: snapshot}
	}
}

func (m Model) importSelectedCmd() tea.Cmd {
	if m.backend.ImportSession == nil {
		return nil
	}
	item, ok := m.selectedSession()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		bundle, err := m.backend.ImportSession(m.ctx, item.Tool, item.Session.ID)
		if err != nil {
			return errorMsg{err: err}
		}
		return bundleImportedMsg{
			sessionKey: sessionKeyFor(item.Tool, item.Session.ID),
			bundle:     bundle,
			autoDoctor: true,
		}
	}
}

func (m Model) doctorSelectedCmd() tea.Cmd {
	if m.backend.DoctorBundle == nil {
		return nil
	}
	item, ok := m.selectedSession()
	if !ok {
		return nil
	}
	sessionKey := sessionKeyFor(item.Tool, item.Session.ID)
	target := m.targetTool()
	cachedBundle, hasBundle := m.bundleBySession[sessionKey]
	return func() tea.Msg {
		bundle := cachedBundle
		var bundlePtr *domain.SessionBundle
		if !hasBundle {
			if m.backend.ImportSession == nil {
				return errorMsg{err: fmt.Errorf("import backend is required to run doctor on %s", item.Session.ID)}
			}
			imported, err := m.backend.ImportSession(m.ctx, item.Tool, item.Session.ID)
			if err != nil {
				return errorMsg{err: err}
			}
			bundle = imported
			bundlePtr = &imported
		}
		report, err := m.backend.DoctorBundle(m.ctx, bundle, target)
		if err != nil {
			return errorMsg{err: err}
		}
		return doctorReadyMsg{
			sessionKey: sessionKey,
			target:     target,
			bundle:     bundlePtr,
			report:     report,
		}
	}
}

func (m Model) exportSelectedCmd() tea.Cmd {
	if m.backend.ExportBundle == nil || m.backend.DoctorBundle == nil {
		return nil
	}
	item, ok := m.selectedSession()
	if !ok {
		return nil
	}
	sessionKey := sessionKeyFor(item.Tool, item.Session.ID)
	target := m.targetTool()
	cachedBundle, hasBundle := m.bundleBySession[sessionKey]
	cachedDoctor, hasDoctor := m.doctorByKey[doctorKey(sessionKey, target)]
	outDir := exportOutputDir(m.backend.DefaultExportDir, item.Session.ID, target)

	return func() tea.Msg {
		bundle := cachedBundle
		if !hasBundle {
			if m.backend.ImportSession == nil {
				return errorMsg{err: fmt.Errorf("import backend is required to export %s", item.Session.ID)}
			}
			imported, err := m.backend.ImportSession(m.ctx, item.Tool, item.Session.ID)
			if err != nil {
				return errorMsg{err: err}
			}
			bundle = imported
		}
		report := cachedDoctor
		if !hasDoctor {
			generated, err := m.backend.DoctorBundle(m.ctx, bundle, target)
			if err != nil {
				return errorMsg{err: err}
			}
			report = generated
		}
		manifest, err := m.backend.ExportBundle(m.ctx, bundle, target, outDir)
		if err != nil {
			return errorMsg{err: err}
		}
		return exportReadyMsg{
			sessionKey: sessionKey,
			target:     target,
			bundle:     bundle,
			report:     report,
			manifest:   manifest,
		}
	}
}

func (m Model) installSelectedSkillCmd() tea.Cmd {
	if m.backend.InstallSkill == nil {
		return nil
	}
	item, ok := m.selectedSkill()
	if !ok {
		return nil
	}
	target, ok := m.selectedSkillTarget(item)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		result, err := m.backend.InstallSkill(m.ctx, item, target)
		if err != nil {
			return errorMsg{err: err}
		}
		return skillInstalledMsg{skill: item, result: result}
	}
}

func (m Model) probeSelectedMCP() tea.Cmd {
	if m.backend.ProbeMCP == nil {
		return nil
	}
	item, ok := m.selectedMCP()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		result, err := m.backend.ProbeMCP(m.ctx, item)
		if err != nil {
			return errorMsg{err: err}
		}
		return mcpProbedMsg{entry: item, result: result}
	}
}

func (m *Model) moveSelection(delta int) {
	switch m.focus {
	case focusNav:
		idx := indexOfSection(sectionOrder, m.activeSection)
		m.setActiveSection(sectionOrder[wrapIndex(idx+delta, len(sectionOrder))])
		return
	case focusPreview:
		m.scrollPreview(delta * 2)
		return
	}

	switch m.activeSection {
	case sectionSessions:
		m.setSessionIndex(clampIndex(m.sessionIdx+delta, len(m.filteredSessions())))
	case sectionProjects:
		m.setProjectIndex(clampIndex(m.projectIdx+delta, len(m.filteredProjects())))
	case sectionSkills:
		m.setSkillIndex(clampIndex(m.skillIdx+delta, len(m.filteredSkills())))
	case sectionMCP:
		m.setMCPIndex(clampIndex(m.mcpIdx+delta, len(m.filteredMCP())))
	case sectionLogs:
		m.setLogIndex(clampIndex(m.logIdx+delta, len(m.filteredLogs())))
	}
	m.ensureVisibleSelection()
}

func (m *Model) movePreviewTab(delta int) {
	switch m.activeSection {
	case sectionSessions:
		m.sessionTabIdx = wrapIndex(m.sessionTabIdx+delta, len(sessionPreviewTabs))
	case sectionSkills:
		m.skillTabIdx = wrapIndex(m.skillTabIdx+delta, len(skillPreviewTabs))
	case sectionMCP:
		m.mcpTabIdx = wrapIndex(m.mcpTabIdx+delta, len(mcpPreviewTabs))
	}
	m.resetPreviewScroll()
}

func (m *Model) scrollPreview(delta int) {
	lines := strings.Split(m.previewContent(), "\n")
	layout := m.currentLayout()
	visible := panelBodyHeight(layout.preview)
	if m.activeSection != sectionLogs {
		visible = maxInt(0, visible-1)
	}
	maxOffset := maxInt(0, len(lines)-visible)
	offset := clampIndex(m.currentPreviewOffset()+delta, maxOffset+1)
	if offset > maxOffset {
		offset = maxOffset
	}
	m.setPreviewOffset(offset)
}

func (m *Model) ensureValidSelection() {
	m.sessionIdx = clampIndex(m.sessionIdx, len(m.filteredSessions()))
	m.projectIdx = clampIndex(m.projectIdx, len(m.filteredProjects()))
	m.skillIdx = clampIndex(m.skillIdx, len(m.filteredSkills()))
	m.mcpIdx = clampIndex(m.mcpIdx, len(m.filteredMCP()))
	m.logIdx = clampIndex(m.logIdx, len(m.filteredLogs()))
}

func (m *Model) ensureSectionWithContent() {
	if m.sectionCount(m.activeSection) > 0 {
		return
	}
	for _, section := range sectionOrder {
		if m.sectionCount(section) > 0 {
			m.setActiveSection(section)
			return
		}
	}
}

func (m *Model) ensureVisibleSelection() {
	layout := m.currentLayout()
	switch m.activeSection {
	case sectionSessions:
		m.sessionListOffset = ensureWindowOffset(m.sessionListOffset, len(m.filteredSessions()), 2, panelBodyHeight(layout.list), m.sessionIdx)
	case sectionProjects:
		m.projectListOffset = ensureWindowOffset(m.projectListOffset, len(m.filteredProjects()), 2, panelBodyHeight(layout.list), m.projectIdx)
	case sectionSkills:
		m.skillListOffset = ensureWindowOffset(m.skillListOffset, len(m.filteredSkills()), 2, panelBodyHeight(layout.list), m.skillIdx)
	case sectionMCP:
		m.mcpListOffset = ensureWindowOffset(m.mcpListOffset, len(m.filteredMCP()), 2, panelBodyHeight(layout.list), m.mcpIdx)
	case sectionLogs:
		m.logListOffset = ensureWindowOffset(m.logListOffset, len(m.filteredLogs()), 1, panelBodyHeight(layout.list), m.logIdx)
	}
}

func (m Model) filteredSessions() []sessionItem {
	items := []sessionItem{}
	for _, tool := range toolOrder {
		report, ok := m.snapshot.InspectByTool[tool]
		if !ok {
			continue
		}
		for _, session := range report.Sessions {
			item := sessionItem{Tool: tool, Report: report, Session: session}
			if !m.matchesActiveProjectSession(item) {
				continue
			}
			if !matchesQuery(m.searchQuery, string(tool), session.Title, session.ID, session.ProjectRoot, session.StoragePath) {
				continue
			}
			items = append(items, item)
		}
	}
	return items
}

func (m Model) filteredProjects() []ProjectEntry {
	items := make([]ProjectEntry, 0, len(m.snapshot.Projects))
	for _, item := range m.snapshot.Projects {
		if matchesQuery(m.searchQuery, item.Name, item.Root, item.WorkspaceRoot, strings.Join(item.Markers, " ")) {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].Root < items[j].Root
		}
		return items[i].Name < items[j].Name
	})
	return items
}

func (m Model) filteredSkills() []SkillEntry {
	items := make([]SkillEntry, 0, len(m.snapshot.Skills))
	for _, item := range m.snapshot.Skills {
		if !m.matchesActiveProjectSkill(item) {
			continue
		}
		if !matchesQuery(m.searchQuery, item.Name, item.Description, item.Path, item.Source) {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if strings.EqualFold(items[i].Name, items[j].Name) {
			if skillScopeRank(items[i].Scope) == skillScopeRank(items[j].Scope) {
				if items[i].Tool == items[j].Tool {
					return items[i].Path < items[j].Path
				}
				return items[i].Tool < items[j].Tool
			}
			return skillScopeRank(items[i].Scope) < skillScopeRank(items[j].Scope)
		}
		if items[i].GroupKey == items[j].GroupKey {
			return items[i].Path < items[j].Path
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items
}

func (m Model) filteredMCP() []MCPEntry {
	items := make([]MCPEntry, 0, len(m.snapshot.MCPProfiles))
	for _, item := range m.snapshot.MCPProfiles {
		if len(item.Declarations) == 0 && !m.matchesActiveProjectPath(item.Path) {
			continue
		}
		resolved, ok := m.resolveMCPEntry(item)
		if !ok {
			continue
		}
		if !matchesQuery(
			m.searchQuery,
			resolved.Name,
			resolved.Path,
			resolved.Source,
			resolved.Scope,
			resolved.Status,
			resolved.Details,
			resolved.Transport,
			string(resolved.Tool),
			strings.Join(resolved.HiddenScopes, " "),
			strings.Join(resolved.ServerNames, " "),
			strings.Join(m.mcpDeclarationSearchTerms(resolved.Declarations), " "),
		) {
			continue
		}
		items = append(items, resolved)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Tool == items[j].Tool {
			if items[i].Name == items[j].Name {
				return items[i].Path < items[j].Path
			}
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].Tool < items[j].Tool
	})
	return items
}

func (m Model) filteredLogs() []string {
	if m.searchQuery == "" {
		return append([]string{}, m.logs...)
	}
	out := []string{}
	for _, line := range m.logs {
		if matchesQuery(m.searchQuery, line) {
			out = append(out, line)
		}
	}
	return out
}

func (m Model) selectedSession() (sessionItem, bool) {
	items := m.filteredSessions()
	if len(items) == 0 {
		return sessionItem{}, false
	}
	return items[clampIndex(m.sessionIdx, len(items))], true
}

func (m Model) selectedSkill() (SkillEntry, bool) {
	items := m.filteredSkills()
	if len(items) == 0 {
		return SkillEntry{}, false
	}
	return items[clampIndex(m.skillIdx, len(items))], true
}

func (m Model) selectedProject() (ProjectEntry, bool) {
	items := m.filteredProjects()
	if len(items) == 0 {
		return ProjectEntry{}, false
	}
	return items[clampIndex(m.projectIdx, len(items))], true
}

func (m Model) activeProject() (ProjectEntry, bool) {
	if strings.TrimSpace(m.activeProjectRoot) == "" {
		return ProjectEntry{}, false
	}
	for _, item := range m.snapshot.Projects {
		if samePath(item.Root, m.activeProjectRoot) {
			return item, true
		}
	}
	return ProjectEntry{}, false
}

func (m Model) selectedMCP() (MCPEntry, bool) {
	items := m.filteredMCP()
	if len(items) == 0 {
		return MCPEntry{}, false
	}
	return items[clampIndex(m.mcpIdx, len(items))], true
}

func (m Model) projectListItem(item ProjectEntry) listItem {
	badges := []badge{}
	if samePath(item.Root, m.activeProjectRoot) {
		badges = append(badges, badge{label: "ACTIVE", tone: "accent"})
	}
	for _, marker := range item.Markers {
		tone := "muted"
		switch marker {
		case "git":
			tone = "accent"
		case "skills":
			tone = "warning"
		case "claude", "gemini", "codex", "opencode":
			tone = "success"
		}
		badges = append(badges, badge{label: strings.ToUpper(marker), tone: tone})
	}
	if item.SessionCount > 0 {
		badges = append(badges, badge{label: fmt.Sprintf("%d SES", item.SessionCount), tone: "muted"})
	}
	return listItem{
		title:    item.Name,
		subtitle: shortPath(item.Root),
		badges:   badges,
	}
}

func (m Model) sessionListItem(item sessionItem) listItem {
	key := sessionKeyFor(item.Tool, item.Session.ID)
	badges := []badge{{label: strings.ToUpper(string(item.Tool)), tone: "accent"}}
	if _, ok := m.bundleBySession[key]; ok {
		badges = append(badges, badge{label: "IMPORTED", tone: "success"})
	}
	if _, ok := m.doctorByKey[doctorKey(key, m.targetTool())]; ok {
		badges = append(badges, badge{label: "DOCTOR", tone: "warning"})
	}
	if _, ok := m.exportByKey[doctorKey(key, m.targetTool())]; ok {
		badges = append(badges, badge{label: "EXPORTED", tone: "muted"})
	}
	subtitle := shortPath(firstNonEmpty(item.Session.ProjectRoot, item.Session.StoragePath))
	if subtitle == "" {
		subtitle = firstNonEmpty(item.Session.UpdatedAt, "session metadata only")
	}
	return listItem{
		title:    firstNonEmpty(item.Session.Title, item.Session.ID),
		subtitle: subtitle,
		badges:   badges,
	}
}

func (m Model) skillListItem(item SkillEntry) listItem {
	scopeTone := "muted"
	switch item.Scope {
	case "project":
		scopeTone = "accent"
	case "global":
		scopeTone = "warning"
	}
	badges := []badge{{label: strings.ToUpper(firstNonEmpty(item.Scope, "project")), tone: scopeTone}}
	if item.Tool != "" {
		badges = append(badges, badge{label: strings.ToUpper(string(item.Tool)), tone: "success"})
	}
	switch item.ConflictState {
	case "only-in-project":
		badges = append(badges, badge{label: "ONLY PROJECT", tone: "accent"})
	case "only-in-user/global":
		badges = append(badges, badge{label: "ONLY EXTERNAL", tone: "muted"})
	case "both-present":
		badges = append(badges, badge{label: "BOTH", tone: "success"})
	case "content-diverged":
		badges = append(badges, badge{label: "DIVERGED", tone: "warning"})
	}
	if item.VariantCount > 1 {
		badges = append(badges, badge{label: fmt.Sprintf("%d VAR", item.VariantCount), tone: "muted"})
	}
	if install, ok := m.installByPath[item.Path]; ok {
		tone := "success"
		label := "SYNCED"
		if install.Overwrote {
			tone = "warning"
			label = "UPDATED"
		}
		badges = append(badges, badge{label: label, tone: tone})
	}
	return listItem{
		title:    item.Name,
		subtitle: firstNonEmpty(item.Description, shortPath(item.Path)),
		badges:   badges,
	}
}

func (m Model) mcpListItem(item MCPEntry) listItem {
	tone := "muted"
	switch item.Status {
	case "parsed", "probed":
		tone = "success"
	case "configured", "degraded":
		tone = "warning"
	case "broken":
		tone = "error"
	}
	badges := []badge{{label: strings.ToUpper(item.Status), tone: tone}}
	if item.Tool != "" {
		badges = append(badges, badge{label: strings.ToUpper(string(item.Tool)), tone: "accent"})
	}
	if item.Scope != "" {
		badges = append(badges, badge{label: strings.ToUpper(item.Scope), tone: "muted"})
	}
	if item.Transport != "" {
		badges = append(badges, badge{label: strings.ToUpper(item.Transport), tone: "muted"})
	}
	if len(item.Declarations) > 1 {
		badges = append(badges, badge{label: fmt.Sprintf("%d DEF", len(item.Declarations)), tone: "warning"})
	}
	if probe, ok := m.probeByID[mcpEntryKey(item)]; ok {
		label := "PROBED"
		tone = "warning"
		if probe.Reachable {
			tone = "success"
			label = "VALID"
		}
		badges = append(badges, badge{label: label, tone: tone})
	}
	return listItem{
		title:    item.Name,
		subtitle: firstNonEmpty(item.Details, shortPath(item.Path)),
		badges:   badges,
	}
}

func (m Model) activeSectionName() string {
	switch m.activeSection {
	case sectionSessions:
		return "sessions"
	case sectionProjects:
		return "projects"
	case sectionSkills:
		return "skills"
	case sectionMCP:
		return "mcp"
	default:
		return "logs"
	}
}

func (m Model) focusName() string {
	switch m.focus {
	case focusNav:
		return "nav"
	case focusList:
		return "list"
	default:
		return "preview"
	}
}

func (m Model) targetTool() domain.Tool {
	if len(toolOrder) == 0 {
		return domain.ToolClaude
	}
	if m.targetIdx < 0 || m.targetIdx >= len(toolOrder) {
		return toolOrder[0]
	}
	return toolOrder[m.targetIdx]
}

func (m Model) projectRootForPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	cleaned := filepath.Clean(path)
	bestRoot := ""
	bestLen := 0
	for _, item := range m.snapshot.Projects {
		root := filepath.Clean(item.Root)
		if cleaned != root && !strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
			continue
		}
		if len(root) > bestLen {
			bestRoot = root
			bestLen = len(root)
		}
	}
	return bestRoot
}

func (m Model) skillProjectRoot() string {
	if m.activeProjectRoot != "" {
		return filepath.Clean(m.activeProjectRoot)
	}
	if strings.TrimSpace(m.snapshot.Detect.ProjectRoot) == "" {
		return ""
	}
	return filepath.Clean(m.snapshot.Detect.ProjectRoot)
}

func (m Model) matchesActiveProjectPath(path string) bool {
	if strings.TrimSpace(m.activeProjectRoot) == "" {
		return true
	}
	return samePath(m.projectRootForPath(path), m.activeProjectRoot)
}

func (m Model) matchesActiveProjectSkill(item SkillEntry) bool {
	if strings.TrimSpace(m.activeProjectRoot) == "" {
		return true
	}
	if item.Scope == "" {
		return samePath(m.projectRootForPath(item.Path), m.activeProjectRoot)
	}
	if item.Scope != "project" {
		return true
	}
	return samePath(m.projectRootForPath(item.Path), m.activeProjectRoot)
}

func (m Model) matchesActiveProjectSession(item sessionItem) bool {
	if strings.TrimSpace(m.activeProjectRoot) == "" {
		return true
	}
	if root := m.projectRootForPath(item.Session.ProjectRoot); root != "" {
		return samePath(root, m.activeProjectRoot)
	}
	return samePath(m.projectRootForPath(item.Session.StoragePath), m.activeProjectRoot)
}

func (m Model) resolveMCPEntry(item MCPEntry) (MCPEntry, bool) {
	resolved := item
	resolved.ID = mcpEntryKey(item)
	if len(item.Declarations) == 0 {
		if resolved.Kind == "" {
			if len(resolved.Servers) > 0 {
				resolved.Kind = "server"
			} else {
				resolved.Kind = "config"
			}
		}
		if resolved.Transport == "" && len(resolved.Servers) > 0 {
			resolved.Transport = firstNonEmpty(resolved.Servers[0].Transport, "stdio")
		}
		return resolved, true
	}

	declarations := m.relevantMCPDeclarations(item)
	if len(declarations) == 0 {
		return MCPEntry{}, false
	}
	sort.SliceStable(declarations, func(i, j int) bool {
		if mcpScopeRank(declarations[i].Scope) == mcpScopeRank(declarations[j].Scope) {
			if declarations[i].Path == declarations[j].Path {
				return declarations[i].Label < declarations[j].Label
			}
			return declarations[i].Path < declarations[j].Path
		}
		return mcpScopeRank(declarations[i].Scope) < mcpScopeRank(declarations[j].Scope)
	})

	effective := declarations[0]
	resolved.Declarations = declarations
	resolved.Path = effective.Path
	resolved.Source = effective.Source
	resolved.Scope = effective.Scope
	resolved.Status = firstNonEmpty(effective.Status, resolved.Status)
	resolved.Details = firstNonEmpty(effective.Details, resolved.Details)
	resolved.Format = firstNonEmpty(effective.Format, resolved.Format)
	resolved.ParseSource = firstNonEmpty(effective.ParseSource, resolved.ParseSource)
	resolved.RawConfig = firstNonEmpty(effective.RawConfig, resolved.RawConfig)
	resolved.ParseWarnings = dedupeText(append(collectMCPDeclarationWarnings(declarations), resolved.ParseWarnings...))
	resolved.BinaryFound = effective.BinaryFound
	resolved.BinaryPath = firstNonEmpty(effective.BinaryPath, resolved.BinaryPath)
	resolved.Transport = firstNonEmpty(effective.Server.Transport, resolved.Transport)
	if resolved.Kind == "" {
		if effective.Server.Name != "" {
			resolved.Kind = "server"
		} else {
			resolved.Kind = "config"
		}
	}
	if resolved.Name == "" {
		resolved.Name = firstNonEmpty(effective.Server.Name, filepath.Base(effective.Path))
	}
	if resolved.Kind == "server" && effective.Server.Name != "" {
		resolved.Servers = []MCPServerConfig{effective.Server}
		resolved.ServerNames = []string{effective.Server.Name}
		resolved.DeclaredServers = len(declarations)
		if resolved.Details == "" {
			resolved.Details = fmt.Sprintf("%s declaration in %s", firstNonEmpty(effective.Scope, effective.Source), shortPath(effective.Path))
		}
	} else {
		resolved.Servers = nil
		resolved.ServerNames = nil
		resolved.DeclaredServers = len(declarations)
	}

	hidden := []string{}
	for i, decl := range declarations {
		if i == 0 {
			continue
		}
		hidden = append(hidden, fmt.Sprintf("%s %s", firstNonEmpty(decl.Scope, decl.Source), shortPath(decl.Path)))
	}
	resolved.HiddenScopes = dedupeText(hidden)
	if !resolved.BinaryFound {
		for _, decl := range declarations {
			if decl.BinaryFound {
				resolved.BinaryFound = true
				resolved.BinaryPath = decl.BinaryPath
				break
			}
		}
	}
	if strings.TrimSpace(m.activeProjectRoot) == "" && mcpHasMultipleProjectScopes(declarations, m.projectRootForPath) {
		resolved.ParseWarnings = dedupeText(append(resolved.ParseWarnings, "effective declaration varies by project scope"))
	}
	if resolved.Status == "" {
		resolved.Status = "configured"
	}
	return resolved, true
}

func (m Model) relevantMCPDeclarations(item MCPEntry) []MCPDeclaration {
	if len(item.Declarations) == 0 {
		return nil
	}
	out := make([]MCPDeclaration, 0, len(item.Declarations))
	for _, decl := range item.Declarations {
		if strings.TrimSpace(m.activeProjectRoot) == "" {
			out = append(out, decl)
			continue
		}
		switch decl.Scope {
		case "project", "local":
			if samePath(m.projectRootForPath(decl.Path), m.activeProjectRoot) {
				out = append(out, decl)
			}
		default:
			out = append(out, decl)
		}
	}
	return out
}

func (m Model) mcpDeclarationSearchTerms(declarations []MCPDeclaration) []string {
	terms := make([]string, 0, len(declarations)*6)
	for _, decl := range declarations {
		terms = append(
			terms,
			decl.Path,
			decl.Source,
			decl.Scope,
			decl.Label,
			decl.Server.Transport,
			decl.Server.Command,
			decl.Server.URL,
		)
		terms = append(terms, decl.ParseWarnings...)
	}
	return terms
}

func mcpScopeRank(scope string) int {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "local":
		return 0
	case "project":
		return 1
	case "user":
		return 2
	case "global":
		return 3
	case "legacy":
		return 4
	default:
		return 5
	}
}

func mcpHasMultipleProjectScopes(declarations []MCPDeclaration, projectRootForPath func(string) string) bool {
	roots := map[string]struct{}{}
	for _, decl := range declarations {
		switch decl.Scope {
		case "project", "local":
			if root := projectRootForPath(decl.Path); root != "" {
				roots[root] = struct{}{}
			}
		}
	}
	return len(roots) > 1
}

func collectMCPDeclarationWarnings(declarations []MCPDeclaration) []string {
	warnings := []string{}
	for _, decl := range declarations {
		for _, warning := range decl.ParseWarnings {
			prefix := firstNonEmpty(decl.Scope, decl.Source)
			warnings = append(warnings, strings.TrimSpace(prefix+": "+warning))
		}
	}
	return warnings
}

func mcpEntryKey(item MCPEntry) string {
	if strings.TrimSpace(item.ID) != "" {
		return item.ID
	}
	return strings.Join([]string{
		firstNonEmpty(string(item.Tool), "shared"),
		firstNonEmpty(item.Kind, "config"),
		firstNonEmpty(item.Name, filepath.Base(item.Path)),
		filepath.Clean(firstNonEmpty(item.Path, item.Name)),
	}, "|")
}

func (m *Model) moveSkillTarget(delta int) {
	item, ok := m.selectedSkill()
	if !ok {
		return
	}
	targets := m.availableSkillTargets(item)
	if len(targets) == 0 {
		return
	}
	current, ok := m.selectedSkillTarget(item)
	index := 0
	if ok {
		for i, target := range targets {
			if target.ID == current.ID {
				index = i
				break
			}
		}
	}
	index = wrapIndex(index+delta, len(targets))
	m.skillTargetByPath[item.Path] = targets[index].ID
}

func (m Model) selectedSkillTarget(item SkillEntry) (SkillTarget, bool) {
	targets := m.availableSkillTargets(item)
	if len(targets) == 0 {
		return SkillTarget{}, false
	}
	if selectedID, ok := m.skillTargetByPath[item.Path]; ok {
		for _, target := range targets {
			if target.ID == selectedID {
				return target, true
			}
		}
	}
	if item.Scope != "project" {
		for _, target := range targets {
			if target.Scope == "project" {
				return target, true
			}
		}
	}
	if item.Scope == "project" {
		for _, target := range targets {
			if target.SameSource {
				continue
			}
			if target.Exists {
				return target, true
			}
		}
		for _, target := range targets {
			if target.Scope != "project" && !target.SameSource {
				return target, true
			}
		}
	}
	return targets[0], true
}

func (m Model) availableSkillTargets(item SkillEntry) []SkillTarget {
	name := skillDirName(item)
	targets := []SkillTarget{}
	seen := map[string]struct{}{}
	add := func(id string, label string, scope string, tool domain.Tool, skillPath string) {
		cleaned := filepath.Clean(skillPath)
		if cleaned == "" {
			return
		}
		if _, ok := seen[cleaned]; ok {
			return
		}
		seen[cleaned] = struct{}{}
		targets = append(targets, SkillTarget{
			ID:         id,
			Label:      label,
			Scope:      scope,
			Tool:       tool,
			Path:       cleaned,
			Exists:     m.skillTargetExists(item, cleaned),
			SameSource: samePath(cleaned, item.Path),
		})
	}

	if projectRoot := m.skillProjectRoot(); projectRoot != "" {
		add("project:"+projectRoot, "project", "project", "", filepath.Join(projectRoot, "skills", name, "SKILL.md"))
	}
	homeDir := strings.TrimSpace(m.snapshot.HomeDir)
	if homeDir != "" {
		add("user:codex", "codex user", "user", domain.ToolCodex, filepath.Join(homeDir, ".codex", "skills", name, "SKILL.md"))
		add("user:claude", "claude user", "user", domain.ToolClaude, filepath.Join(homeDir, ".claude", "skills", name, "SKILL.md"))
		add("user:opencode", "opencode user", "user", domain.ToolOpenCode, filepath.Join(homeDir, ".config", "opencode", "skills", name, "SKILL.md"))
		add("global:opencode", "opencode global", "global", domain.ToolOpenCode, filepath.Join(homeDir, ".local", "share", "opencode", "skills", name, "SKILL.md"))
	}

	sort.SliceStable(targets, func(i, j int) bool {
		if skillScopeRank(targets[i].Scope) == skillScopeRank(targets[j].Scope) {
			if targets[i].Tool == targets[j].Tool {
				return targets[i].Path < targets[j].Path
			}
			return targets[i].Tool < targets[j].Tool
		}
		return skillScopeRank(targets[i].Scope) < skillScopeRank(targets[j].Scope)
	})
	return targets
}

func (m Model) skillTargetExists(item SkillEntry, targetPath string) bool {
	if samePath(item.Path, targetPath) {
		return true
	}
	for _, variant := range item.Variants {
		if samePath(variant.Path, targetPath) {
			return true
		}
	}
	return false
}

func skillDirName(item SkillEntry) string {
	name := filepath.Base(filepath.Dir(item.Path))
	if name != "" && name != "." && name != string(filepath.Separator) {
		return name
	}
	name = sanitizeSegment(item.Name)
	if name == "" {
		return "skill"
	}
	return name
}

func (m Model) currentPreviewTab() PreviewTab {
	switch m.activeSection {
	case sectionSessions:
		return sessionPreviewTabs[wrapIndex(m.sessionTabIdx, len(sessionPreviewTabs))]
	case sectionSkills:
		return skillPreviewTabs[wrapIndex(m.skillTabIdx, len(skillPreviewTabs))]
	case sectionMCP:
		return mcpPreviewTabs[wrapIndex(m.mcpTabIdx, len(mcpPreviewTabs))]
	default:
		return previewSummary
	}
}

func (m Model) previewTabLabel() string {
	if m.activeSection == sectionLogs {
		return ""
	}
	if m.activeSection == sectionProjects {
		return ""
	}
	return string(m.currentPreviewTab())
}

func (m Model) currentSectionTabs() []PreviewTab {
	switch m.activeSection {
	case sectionSessions:
		return sessionPreviewTabs
	case sectionProjects:
		return nil
	case sectionSkills:
		return skillPreviewTabs
	case sectionMCP:
		return mcpPreviewTabs
	default:
		return nil
	}
}

func (m Model) currentLayout() workspaceLayout {
	mode := m.layoutMode()
	bodyY := 2
	contentHeight := maxInt(12, m.height-7)
	switch mode {
	case layoutWide:
		navWidth := 20
		listWidth := maxInt(34, (m.width-navWidth)/3)
		previewWidth := maxInt(36, m.width-navWidth-listWidth)
		return workspaceLayout{
			mode:    mode,
			nav:     rect{X: 0, Y: bodyY, W: navWidth, H: contentHeight},
			list:    rect{X: navWidth, Y: bodyY, W: listWidth, H: contentHeight},
			preview: rect{X: navWidth + listWidth, Y: bodyY, W: previewWidth, H: contentHeight},
		}
	case layoutMedium:
		navWidth := 20
		rightWidth := maxInt(48, m.width-navWidth)
		listHeight := maxInt(8, contentHeight/2)
		previewHeight := maxInt(8, contentHeight-listHeight)
		return workspaceLayout{
			mode:    mode,
			nav:     rect{X: 0, Y: bodyY, W: navWidth, H: contentHeight},
			list:    rect{X: navWidth, Y: bodyY, W: rightWidth, H: listHeight},
			preview: rect{X: navWidth, Y: bodyY + listHeight, W: rightWidth, H: previewHeight},
		}
	default:
		navHeight := 4
		listHeight := maxInt(8, contentHeight-2)
		return workspaceLayout{
			mode:    mode,
			nav:     rect{X: 0, Y: bodyY, W: m.width, H: navHeight},
			list:    rect{X: 0, Y: bodyY + navHeight, W: m.width, H: listHeight},
			preview: rect{X: 0, Y: bodyY + navHeight, W: m.width, H: listHeight},
		}
	}
}

func (m Model) layoutMode() layoutMode {
	switch {
	case m.width >= 120:
		return layoutWide
	case m.width >= 90:
		return layoutMedium
	default:
		return layoutNarrow
	}
}

func (m Model) contextActions() string {
	actions := []string{"[/] search", "[r] refresh", "[?] help", "[q] quit"}
	if m.activeProjectRoot != "" {
		actions = append(actions, "[c] clear-scope")
	}
	switch m.activeSection {
	case sectionSessions:
		actions = append(actions, "[i] import", "[d] doctor", "[e] export", "[t] target")
	case sectionSkills:
		actions = append(actions, "[I] sync", "[t] target")
	case sectionMCP:
		actions = append(actions, "[p] validate")
	}
	return strings.Join(actions, "  ")
}

func (m Model) spinnerFrame() string {
	frames := []string{"-", "\\", "|", "/"}
	return frames[m.spinnerIdx%len(frames)]
}

func (m *Model) hitNav(layout workspaceLayout, x int, y int) (workspaceSection, bool) {
	if layout.mode == layoutNarrow {
		if y != layout.nav.Y+2 {
			return 0, false
		}
		_, hits := m.renderNavCompactHits()
		relX := x - (layout.nav.X + 2)
		for _, hit := range hits {
			if relX >= hit.Start && relX < hit.End {
				return hit.Section, true
			}
		}
		return 0, false
	}

	row := y - (layout.nav.Y + 2)
	if row < 3 {
		return 0, false
	}
	index := row - 3
	if index >= 0 && index < len(sectionOrder) {
		return sectionOrder[index], true
	}
	return 0, false
}

func (m *Model) hitList(layout workspaceLayout, x int, y int) (int, bool) {
	panel := layout.list
	if layout.mode == layoutNarrow && m.focus == focusPreview {
		panel = layout.preview
	}
	row := y - (panel.Y + 2)
	if row < 0 {
		return 0, false
	}
	switch m.activeSection {
	case sectionSessions:
		index := m.sessionListOffset + row/2
		return index, index >= 0 && index < len(m.filteredSessions())
	case sectionProjects:
		index := m.projectListOffset + row/2
		return index, index >= 0 && index < len(m.filteredProjects())
	case sectionSkills:
		index := m.skillListOffset + row/2
		return index, index >= 0 && index < len(m.filteredSkills())
	case sectionMCP:
		index := m.mcpListOffset + row/2
		return index, index >= 0 && index < len(m.filteredMCP())
	default:
		index := m.logListOffset + row
		return index, index >= 0 && index < len(m.filteredLogs())
	}
}

func (m *Model) hitPreviewTab(layout workspaceLayout, x int, y int) (PreviewTab, bool) {
	if m.activeSection == sectionLogs || m.activeSection == sectionProjects {
		return "", false
	}
	if y != layout.preview.Y+2 {
		return "", false
	}
	_, hits := m.renderPreviewTabs()
	relX := x - (layout.preview.X + 2)
	for _, hit := range hits {
		if relX >= hit.Start && relX < hit.End {
			return hit.Tab, true
		}
	}
	return "", false
}

func (m *Model) selectVisibleIndex(index int) {
	switch m.activeSection {
	case sectionSessions:
		m.setSessionIndex(index)
	case sectionProjects:
		items := m.filteredProjects()
		if index >= 0 && index < len(items) && samePath(items[index].Root, m.activeProjectRoot) && index == clampIndex(m.projectIdx, len(items)) {
			m.projectIdx = clampIndex(index, len(items))
			m.clearActiveProject()
			return
		}
		m.setProjectIndex(index)
	case sectionSkills:
		m.setSkillIndex(index)
	case sectionMCP:
		m.setMCPIndex(index)
	case sectionLogs:
		m.setLogIndex(index)
	}
	m.ensureVisibleSelection()
}

func (m *Model) setActiveSection(section workspaceSection) {
	if m.activeSection == section {
		if section == sectionProjects {
			m.syncProjectSelectionToActive()
		}
		return
	}
	m.activeSection = section
	if section == sectionProjects {
		m.syncProjectSelectionToActive()
	}
	m.resetPreviewScroll()
	m.ensureValidSelection()
	m.ensureVisibleSelection()
}

func (m *Model) setSessionIndex(index int) {
	if index == m.sessionIdx {
		return
	}
	m.sessionIdx = clampIndex(index, len(m.filteredSessions()))
	m.resetPreviewScroll()
}

func (m *Model) setProjectIndex(index int) {
	items := m.filteredProjects()
	next := clampIndex(index, len(items))
	if next == m.projectIdx && len(items) > 0 && samePath(items[next].Root, m.activeProjectRoot) {
		return
	}
	m.projectIdx = next
	if len(items) > 0 {
		m.activeProjectRoot = filepath.Clean(items[m.projectIdx].Root)
	}
	m.resetPreviewScroll()
	m.ensureValidSelection()
	m.ensureVisibleSelection()
}

func (m *Model) clearActiveProject() {
	if m.activeProjectRoot == "" {
		return
	}
	m.activeProjectRoot = ""
	m.resetPreviewScroll()
	m.ensureValidSelection()
	m.ensureSectionWithContent()
	m.ensureVisibleSelection()
}

func (m *Model) reconcileActiveProject() {
	if m.activeProjectRoot == "" {
		return
	}
	for _, item := range m.snapshot.Projects {
		if !samePath(item.Root, m.activeProjectRoot) {
			continue
		}
		m.syncProjectSelectionToActive()
		return
	}
	m.activeProjectRoot = ""
}

func (m *Model) syncProjectSelectionToActive() {
	if m.activeProjectRoot == "" {
		return
	}
	items := m.filteredProjects()
	for i, item := range items {
		if samePath(item.Root, m.activeProjectRoot) {
			m.projectIdx = i
			return
		}
	}
}

func (m *Model) setSkillIndex(index int) {
	if index == m.skillIdx {
		return
	}
	m.skillIdx = clampIndex(index, len(m.filteredSkills()))
	m.resetPreviewScroll()
}

func (m *Model) setMCPIndex(index int) {
	if index == m.mcpIdx {
		return
	}
	m.mcpIdx = clampIndex(index, len(m.filteredMCP()))
	m.resetPreviewScroll()
}

func (m *Model) setLogIndex(index int) {
	if index == m.logIdx {
		return
	}
	m.logIdx = clampIndex(index, len(m.filteredLogs()))
	m.resetPreviewScroll()
}

func (m *Model) setPreviewTab(tab PreviewTab) {
	tabs := m.currentSectionTabs()
	for i, candidate := range tabs {
		if candidate != tab {
			continue
		}
		switch m.activeSection {
		case sectionSessions:
			m.sessionTabIdx = i
		case sectionSkills:
			m.skillTabIdx = i
		case sectionMCP:
			m.mcpTabIdx = i
		}
		m.resetPreviewScroll()
		return
	}
}

func (m *Model) resetPreviewScroll() {
	switch m.activeSection {
	case sectionSessions:
		m.sessionPreviewOffset = 0
	case sectionProjects:
		m.projectPreviewOffset = 0
	case sectionSkills:
		m.skillPreviewOffset = 0
	case sectionMCP:
		m.mcpPreviewOffset = 0
	case sectionLogs:
		m.logPreviewOffset = 0
	}
}

func (m Model) currentPreviewOffset() int {
	switch m.activeSection {
	case sectionSessions:
		return m.sessionPreviewOffset
	case sectionProjects:
		return m.projectPreviewOffset
	case sectionSkills:
		return m.skillPreviewOffset
	case sectionMCP:
		return m.mcpPreviewOffset
	default:
		return m.logPreviewOffset
	}
}

func (m *Model) setPreviewOffset(offset int) {
	switch m.activeSection {
	case sectionSessions:
		m.sessionPreviewOffset = offset
	case sectionProjects:
		m.projectPreviewOffset = offset
	case sectionSkills:
		m.skillPreviewOffset = offset
	case sectionMCP:
		m.mcpPreviewOffset = offset
	default:
		m.logPreviewOffset = offset
	}
}

func (m Model) previewLines(height int, content string) ([]string, int) {
	lines := strings.Split(content, "\n")
	if height <= 0 {
		return nil, 0
	}
	offset := m.currentPreviewOffset()
	maxOffset := maxInt(0, len(lines)-height)
	if offset > maxOffset {
		offset = maxOffset
	}
	end := minInt(len(lines), offset+height)
	return lines[offset:end], offset
}

func (m Model) renderNavCompactHits() (string, []navHit) {
	segments := make([]string, 0, len(sectionOrder))
	hits := make([]navHit, 0, len(sectionOrder))
	x := 0
	for _, section := range sectionOrder {
		label := m.navLabel(section)
		tone := "muted"
		if section == m.activeSection {
			tone = "accent"
		}
		rendered := badgeStyle(label, tone)
		segments = append(segments, rendered)
		width := lipgloss.Width(rendered)
		hits = append(hits, navHit{Section: section, Start: x, End: x + width})
		x += width
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, segments...), hits
}

func (m Model) listWindow(total int, itemHeight int, availableLines int, selected int, offset int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	visible := maxInt(1, availableLines/itemHeight)
	offset = ensureWindowOffset(offset, total, itemHeight, availableLines, selected)
	if offset+visible > total {
		visible = total - offset
	}
	return offset, visible
}

func ensureWindowOffset(offset int, total int, itemHeight int, availableLines int, selected int) int {
	if total == 0 {
		return 0
	}
	visible := maxInt(1, availableLines/itemHeight)
	maxOffset := maxInt(0, total-visible)
	if offset > maxOffset {
		offset = maxOffset
	}
	if selected < offset {
		offset = selected
	}
	if selected >= offset+visible {
		offset = selected - visible + 1
	}
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset
}

func panelBodyHeight(panel rect) int {
	return maxInt(1, panel.H-3)
}

func tickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func sourceLabel(source string) string {
	parts := strings.Split(source, string(os.PathSeparator))
	if len(parts) == 0 || source == "" {
		return "project"
	}
	if strings.Contains(source, "skills") || strings.Contains(source, ".codex") || strings.Contains(source, ".claude") || strings.Contains(source, "opencode") {
		return "user"
	}
	return parts[0]
}

func skillScopeRank(scope string) int {
	switch scope {
	case "project":
		return 0
	case "user":
		return 1
	case "global":
		return 2
	default:
		return 3
	}
}

func badgeStyle(label string, tone string) string {
	style := lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("239"))
	switch tone {
	case "accent":
		style = style.Background(lipgloss.Color("45")).Foreground(lipgloss.Color("235"))
	case "success":
		style = style.Background(lipgloss.Color("35")).Foreground(lipgloss.Color("235"))
	case "warning":
		style = style.Background(lipgloss.Color("214")).Foreground(lipgloss.Color("235"))
	case "error":
		style = style.Background(lipgloss.Color("160")).Foreground(lipgloss.Color("255"))
	}
	return style.Render(label)
}

func renderBadges(values []badge) string {
	if len(values) == 0 {
		return ""
	}
	rendered := make([]string, 0, len(values))
	for _, value := range values {
		rendered = append(rendered, badgeStyle(value.label, value.tone))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, rendered...)
}

func exportOutputDir(root string, sessionID string, target domain.Tool) string {
	if root == "" {
		root = filepath.Join(os.TempDir(), defaultExportRoot)
	}
	name := sanitizeSegment(sessionID)
	if name == "" {
		name = "latest"
	}
	return filepath.Join(root, fmt.Sprintf("%s-to-%s", name, target))
}

func sanitizeSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func sessionKeyFor(tool domain.Tool, sessionID string) string {
	return string(tool) + ":" + sessionID
}

func doctorKey(sessionKey string, target domain.Tool) string {
	return sessionKey + "->" + string(target)
}

func mustJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("json error: %v", err)
	}
	return string(data)
}

func matchesQuery(query string, parts ...string) bool {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return true
	}
	for _, part := range parts {
		if strings.Contains(strings.ToLower(part), query) {
			return true
		}
	}
	return false
}

func fitLines(text string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	if height == 1 {
		return lines[0]
	}
	out := append([]string{}, lines[:height-1]...)
	out = append(out, "... more ...")
	return strings.Join(out, "\n")
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

func wrapIndex(idx int, size int) int {
	if size <= 0 {
		return 0
	}
	if idx < 0 {
		return (idx%size + size) % size
	}
	return idx % size
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

func samePath(a string, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func dedupeText(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func boolLabel(ok bool, truthy string, falsy string) string {
	if ok {
		return truthy
	}
	return falsy
}

func indexOfSection(values []workspaceSection, target workspaceSection) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}

func (m Model) sectionCount(section workspaceSection) int {
	switch section {
	case sectionSessions:
		return len(m.filteredSessions())
	case sectionProjects:
		return len(m.filteredProjects())
	case sectionSkills:
		return len(m.filteredSkills())
	case sectionMCP:
		return len(m.filteredMCP())
	default:
		return len(m.filteredLogs())
	}
}
