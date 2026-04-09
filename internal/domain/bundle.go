package domain

import (
	"errors"
	"fmt"
)

type Tool string

const (
	ToolCodex  Tool = "codex"
	ToolGemini Tool = "gemini"
	ToolClaude Tool = "claude"
	BundleV0        = "v0"
)

type SessionBundle struct {
	AssetKind            AssetKind             `json:"asset_kind"`
	BundleVersion        string                `json:"bundle_version"`
	BundleID             string                `json:"bundle_id,omitempty"`
	SourceTool           Tool                  `json:"source_tool"`
	SourceSessionID      string                `json:"source_session_id,omitempty"`
	ImportedAt           string                `json:"imported_at,omitempty"`
	ProjectRoot          string                `json:"project_root"`
	TaskTitle            string                `json:"task_title,omitempty"`
	CurrentGoal          string                `json:"current_goal,omitempty"`
	Summary              string                `json:"summary,omitempty"`
	InstructionArtifacts []InstructionArtifact `json:"instruction_artifacts"`
	SettingsSnapshot     SettingsSnapshot      `json:"settings_snapshot"`
	ToolEvents           []ToolEvent           `json:"tool_events"`
	TouchedFiles         []string              `json:"touched_files"`
	Decisions            []Decision            `json:"decisions"`
	Failures             []Failure             `json:"failures"`
	ResumeHints          []string              `json:"resume_hints"`
	TokenStats           map[string]int64      `json:"token_stats"`
	Provenance           []string              `json:"provenance"`
	Redactions           []string              `json:"redactions"`
	Warnings             []string              `json:"warnings"`
}

type InstructionArtifact struct {
	Tool        Tool   `json:"tool"`
	Kind        string `json:"kind"`
	Path        string `json:"path,omitempty"`
	Scope       string `json:"scope"`
	Content     string `json:"content,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
}

type SettingsSnapshot struct {
	Included     map[string]any `json:"included"`
	ExcludedKeys []string       `json:"excluded_keys"`
}

type ToolEvent struct {
	Type      string `json:"type"`
	Summary   string `json:"summary"`
	Timestamp string `json:"timestamp,omitempty"`
	Status    string `json:"status,omitempty"`
	RawRef    string `json:"raw_ref,omitempty"`
}

type Decision struct {
	Summary    string   `json:"summary"`
	Reason     string   `json:"reason,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	SourceRefs []string `json:"source_refs"`
}

type Failure struct {
	Summary      string   `json:"summary"`
	AttemptedFix string   `json:"attempted_fix,omitempty"`
	Status       string   `json:"status,omitempty"`
	SourceRefs   []string `json:"source_refs"`
}

func NewSessionBundle(tool Tool, projectRoot string) SessionBundle {
	return SessionBundle{
		AssetKind:            AssetKindSession,
		BundleVersion:        BundleV0,
		SourceTool:           tool,
		ProjectRoot:          projectRoot,
		InstructionArtifacts: []InstructionArtifact{},
		SettingsSnapshot: SettingsSnapshot{
			Included:     map[string]any{},
			ExcludedKeys: []string{},
		},
		ToolEvents:   []ToolEvent{},
		TouchedFiles: []string{},
		Decisions:    []Decision{},
		Failures:     []Failure{},
		ResumeHints:  []string{},
		TokenStats:   map[string]int64{},
		Provenance:   []string{},
		Redactions:   []string{},
		Warnings:     []string{},
	}
}

func (b SessionBundle) Validate() error {
	if b.AssetKind == "" {
		return errors.New("asset_kind is required")
	}
	if !b.AssetKind.IsKnown() {
		return fmt.Errorf("unsupported asset_kind %q", b.AssetKind)
	}
	if b.AssetKind != AssetKindSession {
		return fmt.Errorf("session bundle must use asset_kind %q", AssetKindSession)
	}
	if b.BundleVersion == "" {
		return errors.New("bundle_version is required")
	}
	if b.BundleVersion != BundleV0 {
		return fmt.Errorf("unsupported bundle_version %q", b.BundleVersion)
	}
	if !b.SourceTool.IsKnown() {
		return fmt.Errorf("unsupported source_tool %q", b.SourceTool)
	}
	if b.ProjectRoot == "" {
		return errors.New("project_root is required")
	}
	if b.InstructionArtifacts == nil {
		return errors.New("instruction_artifacts must be initialized")
	}
	if b.SettingsSnapshot.Included == nil {
		return errors.New("settings_snapshot.included must be initialized")
	}
	if b.SettingsSnapshot.ExcludedKeys == nil {
		return errors.New("settings_snapshot.excluded_keys must be initialized")
	}
	return nil
}

func (t Tool) IsKnown() bool {
	switch t {
	case ToolCodex, ToolGemini, ToolClaude:
		return true
	default:
		return false
	}
}
