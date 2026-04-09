package capability

import (
	"fmt"
	"slices"

	"sessionport/internal/domain"
)

type Support string

const (
	SupportCompatible  Support = "compatible"
	SupportPartial     Support = "partial"
	SupportUnsupported Support = "unsupported"
)

type Profile struct {
	TargetTool         domain.Tool
	AssetKind          domain.AssetKind
	FieldSupport       map[string]Support
	GeneratedArtifacts []string
	Warnings           []string
}

func ProfileFor(target domain.Tool, assetKind domain.AssetKind) (Profile, error) {
	if !target.IsKnown() {
		return Profile{}, fmt.Errorf("unsupported target tool %q", target)
	}
	if assetKind != domain.AssetKindSession {
		return Profile{}, fmt.Errorf("unsupported asset kind %q", assetKind)
	}

	profile := Profile{
		TargetTool: target,
		AssetKind:  assetKind,
		FieldSupport: map[string]Support{
			"project_root":             SupportCompatible,
			"task_title":               SupportCompatible,
			"current_goal":             SupportCompatible,
			"summary":                  SupportCompatible,
			"instruction_artifacts":    SupportCompatible,
			"settings_snapshot":        SupportPartial,
			"raw_transcript":           SupportPartial,
			"tool_events":              SupportPartial,
			"tool_outputs":             SupportPartial,
			"touched_files":            SupportPartial,
			"decisions":                SupportPartial,
			"failures":                 SupportPartial,
			"resume_hints":             SupportPartial,
			"token_stats":              SupportPartial,
			"hidden_reasoning":         SupportUnsupported,
			"vendor_specific_options":  SupportUnsupported,
			"native_hook_plugin_state": SupportUnsupported,
		},
		GeneratedArtifacts: generatedArtifactsFor(target),
		Warnings:           targetWarnings(target),
	}
	return profile, nil
}

func generatedArtifactsFor(target domain.Tool) []string {
	switch target {
	case domain.ToolCodex:
		return []string{"AGENTS.sessionport.md", "CONFIG_HINTS.md", "STARTER_PROMPT.md"}
	case domain.ToolGemini:
		return []string{"GEMINI.sessionport.md", "SETTINGS_PATCH.json", "STARTER_PROMPT.md"}
	case domain.ToolClaude:
		return []string{"CLAUDE.sessionport.md", "MEMORY_NOTE.md", "STARTER_PROMPT.md"}
	case domain.ToolOpenCode:
		return []string{"OPENCODE.sessionport.md", "CONFIG_HINTS.md", "STARTER_PROMPT.md"}
	default:
		return []string{}
	}
}

func targetWarnings(target domain.Tool) []string {
	switch target {
	case domain.ToolCodex:
		return []string{"Codex export will provide config hints only; vendor-native session resume state is not reconstructed."}
	case domain.ToolGemini:
		return []string{"Gemini export will emit a settings patch suggestion rather than replacing the full local profile."}
	case domain.ToolClaude:
		return []string{"Claude export will convert portable context into CLAUDE.md supplements and plain memory notes."}
	case domain.ToolOpenCode:
		return []string{"OpenCode export will emit a portable supplement and config hints; provider state is not reconstructed."}
	default:
		return []string{}
	}
}

func SupportsField(profile Profile, field string, support Support) bool {
	return profile.FieldSupport[field] == support
}

func SortedGeneratedArtifacts(profile Profile) []string {
	items := append([]string{}, profile.GeneratedArtifacts...)
	slices.Sort(items)
	return items
}
