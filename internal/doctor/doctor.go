package doctor

import (
	"fmt"
	"sort"

	"sessionport/internal/capability"
	"sessionport/internal/domain"
)

type Options struct {
	Bundle domain.SessionBundle
	Target domain.Tool
}

func Analyze(opts Options) (domain.CompatibilityReport, error) {
	if opts.Bundle.AssetKind == "" {
		opts.Bundle.AssetKind = domain.AssetKindSession
	}
	if err := opts.Bundle.Validate(); err != nil {
		return domain.CompatibilityReport{}, err
	}

	profile, err := capability.ProfileFor(opts.Target, opts.Bundle.AssetKind)
	if err != nil {
		return domain.CompatibilityReport{}, err
	}

	report := domain.CompatibilityReport{
		AssetKind:          opts.Bundle.AssetKind,
		BundleID:           opts.Bundle.BundleID,
		SourceTool:         opts.Bundle.SourceTool,
		SourceSessionID:    opts.Bundle.SourceSessionID,
		ProjectRoot:        opts.Bundle.ProjectRoot,
		TargetTool:         opts.Target,
		CompatibleFields:   []string{},
		PartialFields:      []string{},
		UnsupportedFields:  []string{},
		RedactedFields:     append(append([]string{}, opts.Bundle.Redactions...), settingsRedactions(opts.Bundle.SettingsSnapshot)...),
		GeneratedArtifacts: append([]string{}, profile.GeneratedArtifacts...),
		Warnings:           []string{},
	}

	appendPresentField := func(field string, present bool) {
		if !present {
			return
		}
		switch profile.FieldSupport[field] {
		case capability.SupportCompatible:
			report.CompatibleFields = append(report.CompatibleFields, field)
		case capability.SupportPartial:
			report.PartialFields = append(report.PartialFields, field)
		case capability.SupportUnsupported:
			report.UnsupportedFields = append(report.UnsupportedFields, field)
		}
	}

	appendPresentField("project_root", opts.Bundle.ProjectRoot != "")
	appendPresentField("task_title", opts.Bundle.TaskTitle != "")
	appendPresentField("current_goal", opts.Bundle.CurrentGoal != "")
	appendPresentField("summary", opts.Bundle.Summary != "")
	if len(opts.Bundle.InstructionArtifacts) > 0 {
		appendPresentField("instruction_artifacts", true)
	} else {
		report.Warnings = append(report.Warnings, "No instruction artifacts were imported; target bootstrap will rely on summary and prompt notes only.")
	}
	appendPresentField("settings_snapshot", len(opts.Bundle.SettingsSnapshot.Included) > 0 || len(opts.Bundle.SettingsSnapshot.ExcludedKeys) > 0)
	appendPresentField("tool_events", len(opts.Bundle.ToolEvents) > 0)
	appendPresentField("tool_outputs", len(opts.Bundle.ToolEvents) > 0)
	appendPresentField("touched_files", len(opts.Bundle.TouchedFiles) > 0)
	appendPresentField("decisions", len(opts.Bundle.Decisions) > 0)
	appendPresentField("failures", len(opts.Bundle.Failures) > 0)
	appendPresentField("resume_hints", len(opts.Bundle.ResumeHints) > 0)
	appendPresentField("token_stats", len(opts.Bundle.TokenStats) > 0)
	appendPresentField("raw_transcript", true)

	for field, support := range profile.FieldSupport {
		if support == capability.SupportUnsupported {
			report.UnsupportedFields = append(report.UnsupportedFields, field)
		}
	}

	report.Warnings = append(report.Warnings, opts.Bundle.Warnings...)
	report.Warnings = append(report.Warnings, profile.Warnings...)
	if len(opts.Bundle.ResumeHints) > 0 {
		report.Warnings = append(report.Warnings, "Source-native resume hints are carried as plain notes only and do not recreate native resume state.")
	}

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

func settingsRedactions(snapshot domain.SettingsSnapshot) []string {
	if len(snapshot.ExcludedKeys) == 0 {
		return []string{}
	}

	values := make([]string, 0, len(snapshot.ExcludedKeys))
	for _, key := range snapshot.ExcludedKeys {
		if key == "" {
			continue
		}
		values = append(values, "settings."+key)
	}
	return values
}
