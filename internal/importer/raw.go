package importer

import "github.com/jaeyoung0509/work-bridge/internal/domain"

type RawImportResult struct {
	AssetKind            domain.AssetKind
	BundleID             string
	SourceTool           domain.Tool
	SourceSessionID      string
	ImportedAt           string
	ProjectRoot          string
	TaskTitle            string
	CurrentGoal          string
	Summary              string
	InstructionArtifacts []domain.InstructionArtifact
	SettingsSnapshot     domain.SettingsSnapshot
	ToolEvents           []domain.ToolEvent
	TouchedFiles         []string
	Decisions            []domain.Decision
	Failures             []domain.Failure
	ResumeHints          []string
	TokenStats           map[string]int64
	Provenance           []string
	Redactions           []string
	Warnings             []string
}

func newRawImportResult(tool domain.Tool) RawImportResult {
	return RawImportResult{
		AssetKind:            domain.AssetKindSession,
		SourceTool:           tool,
		InstructionArtifacts: []domain.InstructionArtifact{},
		SettingsSnapshot: domain.SettingsSnapshot{
			Included:     map[string]any{},
			ExcludedKeys: []string{},
		},
		ToolEvents:   []domain.ToolEvent{},
		TouchedFiles: []string{},
		Decisions:    []domain.Decision{},
		Failures:     []domain.Failure{},
		ResumeHints:  []string{},
		TokenStats:   map[string]int64{},
		Provenance:   []string{},
		Redactions:   []string{},
		Warnings:     []string{},
	}
}

type Normalizer interface {
	Normalize(raw RawImportResult) (domain.SessionBundle, error)
}

type SessionNormalizer struct{}

func NewSessionNormalizer() SessionNormalizer {
	return SessionNormalizer{}
}
