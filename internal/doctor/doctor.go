package doctor

import (
	"fmt"
	"sort"

	"sessionport/internal/domain"
)

type Options struct {
	Bundle domain.SessionBundle
	Target domain.Tool
}

func Analyze(opts Options) (domain.CompatibilityReport, error) {
	if err := opts.Bundle.Validate(); err != nil {
		return domain.CompatibilityReport{}, err
	}
	if !opts.Target.IsKnown() {
		return domain.CompatibilityReport{}, fmt.Errorf("unsupported target tool %q", opts.Target)
	}

	report := domain.CompatibilityReport{
		BundleID:           opts.Bundle.BundleID,
		SourceTool:         opts.Bundle.SourceTool,
		SourceSessionID:    opts.Bundle.SourceSessionID,
		ProjectRoot:        opts.Bundle.ProjectRoot,
		TargetTool:         opts.Target,
		CompatibleFields:   []string{},
		PartialFields:      []string{},
		UnsupportedFields:  []string{},
		RedactedFields:     []string{},
		GeneratedArtifacts: generatedArtifactsFor(opts.Target),
		Warnings:           []string{},
	}

	if opts.Bundle.ProjectRoot != "" {
		report.CompatibleFields = append(report.CompatibleFields, "project_root")
	}
	if opts.Bundle.TaskTitle != "" {
		report.CompatibleFields = append(report.CompatibleFields, "task_title")
	}
	if opts.Bundle.CurrentGoal != "" {
		report.CompatibleFields = append(report.CompatibleFields, "current_goal")
	}
	if opts.Bundle.Summary != "" {
		report.CompatibleFields = append(report.CompatibleFields, "summary")
	}
	if len(opts.Bundle.InstructionArtifacts) > 0 {
		report.CompatibleFields = append(report.CompatibleFields, "instruction_artifacts")
	} else {
		report.Warnings = append(report.Warnings, "No instruction artifacts were imported; target bootstrap will rely on summary and prompt notes only.")
	}

	if len(opts.Bundle.SettingsSnapshot.Included) > 0 || len(opts.Bundle.SettingsSnapshot.ExcludedKeys) > 0 {
		report.PartialFields = append(report.PartialFields, "settings_snapshot")
	}
	report.PartialFields = append(report.PartialFields, "raw_transcript")

	if len(opts.Bundle.ToolEvents) > 0 {
		report.PartialFields = append(report.PartialFields, "tool_events")
		report.PartialFields = append(report.PartialFields, "tool_outputs")
	}
	if len(opts.Bundle.TouchedFiles) > 0 {
		report.PartialFields = append(report.PartialFields, "touched_files")
	}
	if len(opts.Bundle.Decisions) > 0 {
		report.PartialFields = append(report.PartialFields, "decisions")
	}
	if len(opts.Bundle.Failures) > 0 {
		report.PartialFields = append(report.PartialFields, "failures")
	}
	if len(opts.Bundle.ResumeHints) > 0 {
		report.PartialFields = append(report.PartialFields, "resume_hints")
	}
	if len(opts.Bundle.TokenStats) > 0 {
		report.PartialFields = append(report.PartialFields, "token_stats")
	}

	report.UnsupportedFields = append(report.UnsupportedFields,
		"hidden_reasoning",
		"vendor_specific_options",
		"native_hook_plugin_state",
	)

	for _, key := range opts.Bundle.SettingsSnapshot.ExcludedKeys {
		report.RedactedFields = append(report.RedactedFields, "settings."+key)
	}
	report.RedactedFields = append(report.RedactedFields, opts.Bundle.Redactions...)

	report.Warnings = append(report.Warnings, opts.Bundle.Warnings...)
	report.Warnings = append(report.Warnings, policyWarnings(opts.Target, opts.Bundle)...)

	if len(opts.Bundle.SettingsSnapshot.ExcludedKeys) > 0 {
		report.Warnings = append(report.Warnings, fmt.Sprintf("%d settings keys remain redacted in portability output.", len(opts.Bundle.SettingsSnapshot.ExcludedKeys)))
	}
	if opts.Bundle.TaskTitle == "" && opts.Bundle.CurrentGoal == "" {
		report.Warnings = append(report.Warnings, "Task title and current goal are missing; exporters will fall back to summary-only bootstrap text.")
	}

	sort.Strings(report.GeneratedArtifacts)
	report.CompatibleFields = uniqueSorted(report.CompatibleFields)
	report.PartialFields = uniqueSorted(report.PartialFields)
	report.UnsupportedFields = uniqueSorted(report.UnsupportedFields)
	report.RedactedFields = uniqueSorted(report.RedactedFields)
	report.Warnings = uniqueSorted(report.Warnings)

	return report, nil
}

func generatedArtifactsFor(target domain.Tool) []string {
	switch target {
	case domain.ToolCodex:
		return []string{
			"AGENTS.sessionport.md",
			"CONFIG_HINTS.md",
			"STARTER_PROMPT.md",
		}
	case domain.ToolGemini:
		return []string{
			"GEMINI.sessionport.md",
			"SETTINGS_PATCH.json",
			"STARTER_PROMPT.md",
		}
	case domain.ToolClaude:
		return []string{
			"CLAUDE.sessionport.md",
			"MEMORY_NOTE.md",
			"STARTER_PROMPT.md",
		}
	default:
		return nil
	}
}

func policyWarnings(target domain.Tool, bundle domain.SessionBundle) []string {
	warnings := []string{}

	if len(bundle.ResumeHints) > 0 {
		warnings = append(warnings, "Source-native resume hints are carried as plain notes only and do not recreate native resume state.")
	}

	switch target {
	case domain.ToolCodex:
		warnings = append(warnings, "Codex export will provide config hints only; vendor-native session resume state is not reconstructed.")
	case domain.ToolGemini:
		warnings = append(warnings, "Gemini export will emit a settings patch suggestion rather than replacing the full local profile.")
	case domain.ToolClaude:
		warnings = append(warnings, "Claude export will convert portable context into CLAUDE.md supplements and plain memory notes.")
	default:
	}

	return warnings
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	sort.Strings(values)
	deduped := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if len(deduped) == 0 || deduped[len(deduped)-1] != value {
			deduped = append(deduped, value)
		}
	}
	return deduped
}
