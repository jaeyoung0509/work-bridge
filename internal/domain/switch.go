package domain

type SwitchState string

const (
	SwitchStateReady   SwitchState = "READY"
	SwitchStateApplied SwitchState = "APPLIED"
	SwitchStatePartial SwitchState = "PARTIAL"
	SwitchStateError   SwitchState = "ERROR"
)

type SwitchMode string

const (
	SwitchModeProject SwitchMode = "project"
	SwitchModeNative  SwitchMode = "native"
)

func (m SwitchMode) IsKnown() bool {
	switch m {
	case SwitchModeProject, SwitchModeNative:
		return true
	default:
		return false
	}
}

type SwitchPayload struct {
	Bundle   SessionBundle   `json:"bundle"`
	Skills   []SkillPayload  `json:"skills"`
	MCP      MCPPayload      `json:"mcp"`
	Warnings []string        `json:"warnings,omitempty"`
}

type SkillPayload struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	RootPath    string   `json:"root_path"`
	EntryPath   string   `json:"entry_path"`
	Files       []string `json:"files,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Tool        Tool     `json:"tool,omitempty"`
}

type MCPPayload struct {
	Sources  []MCPSource                `json:"sources"`
	Servers  map[string]MCPServerConfig `json:"servers"`
	Warnings []string                   `json:"warnings,omitempty"`
}

type MCPSource struct {
	Path          string            `json:"path"`
	Scope         string            `json:"scope,omitempty"`
	Tool          Tool              `json:"tool,omitempty"`
	Format        string            `json:"format,omitempty"`
	Status        string            `json:"status,omitempty"`
	ServerNames   []string          `json:"server_names,omitempty"`
	Servers       []MCPServerConfig `json:"servers,omitempty"`
	ParseWarnings []string          `json:"parse_warnings,omitempty"`
	RawConfig     string            `json:"raw_config,omitempty"`
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

type SwitchPlan struct {
	Mode          SwitchMode            `json:"mode"`
	TargetTool    Tool                  `json:"target_tool"`
	ProjectRoot   string                `json:"project_root"`
	DestinationRoot string              `json:"destination_root"`
	ManagedRoot   string                `json:"managed_root"`
	Status        SwitchState           `json:"status"`
	Compatibility CompatibilityReport   `json:"compatibility"`
	Session       SwitchComponentPlan   `json:"session"`
	Skills        SwitchComponentPlan   `json:"skills"`
	MCP           SwitchComponentPlan   `json:"mcp"`
	PlannedFiles  []PlannedFileChange   `json:"planned_files"`
	Warnings      []string              `json:"warnings,omitempty"`
	Errors        []string              `json:"errors,omitempty"`
}

type PlannedFileChange struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Section string `json:"section"`
}

type SwitchComponentPlan struct {
	State    SwitchState `json:"state"`
	Summary  string      `json:"summary,omitempty"`
	Files    []string    `json:"files,omitempty"`
	Warnings []string    `json:"warnings,omitempty"`
	Errors   []string    `json:"errors,omitempty"`
}

type ApplyReport struct {
	TargetTool      Tool                   `json:"target_tool"`
	AppliedMode     string                 `json:"applied_mode"`
	ProjectRoot     string                 `json:"project_root"`
	DestinationRoot string                 `json:"destination_root"`
	ManagedRoot     string                 `json:"managed_root"`
	DryRun          bool                   `json:"dry_run,omitempty"`
	Status          SwitchState            `json:"status"`
	FilesUpdated    []string               `json:"files_updated,omitempty"`
	BackupsCreated  []string               `json:"backups_created,omitempty"`
	Session         ApplyComponentResult   `json:"session"`
	Skills          ApplyComponentResult   `json:"skills"`
	MCP             ApplyComponentResult   `json:"mcp"`
	Warnings        []string               `json:"warnings,omitempty"`
	Errors          []string               `json:"errors,omitempty"`
}

type ApplyComponentResult struct {
	State   SwitchState `json:"state"`
	Summary string      `json:"summary,omitempty"`
	Files   []string    `json:"files,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}

type TargetAdapter interface {
	Target() Tool
	Preview(payload SwitchPayload, projectRoot string, mode SwitchMode, destinationOverride string) (SwitchPlan, error)
	ApplyProject(payload SwitchPayload, plan SwitchPlan) (ApplyReport, error)
	ApplyNativeProject(payload SwitchPayload, plan SwitchPlan) (ApplyReport, error)
	ExportProject(payload SwitchPayload, plan SwitchPlan) (ApplyReport, error)
	ExportNative(payload SwitchPayload, plan SwitchPlan) (ApplyReport, error)
}
