package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"sessionport/internal/domain"
)

func (SessionNormalizer) Normalize(raw RawImportResult) (domain.SessionBundle, error) {
	if raw.AssetKind == "" {
		raw.AssetKind = domain.AssetKindSession
	}
	if !raw.AssetKind.IsKnown() {
		return domain.SessionBundle{}, fmt.Errorf("unsupported asset_kind %q", raw.AssetKind)
	}
	if raw.AssetKind != domain.AssetKindSession {
		return domain.SessionBundle{}, fmt.Errorf("session normalizer only supports asset_kind %q", domain.AssetKindSession)
	}
	if !raw.SourceTool.IsKnown() {
		return domain.SessionBundle{}, fmt.Errorf("unsupported source_tool %q", raw.SourceTool)
	}

	bundle := domain.NewSessionBundle(raw.SourceTool, strings.TrimSpace(raw.ProjectRoot))
	bundle.AssetKind = raw.AssetKind
	bundle.BundleID = strings.TrimSpace(raw.BundleID)
	if bundle.BundleID == "" {
		bundle.BundleID = normalizeBundleID(raw)
	}
	bundle.SourceSessionID = strings.TrimSpace(raw.SourceSessionID)
	bundle.ImportedAt = strings.TrimSpace(raw.ImportedAt)
	bundle.ProjectRoot = strings.TrimSpace(raw.ProjectRoot)
	bundle.TaskTitle = strings.TrimSpace(raw.TaskTitle)
	bundle.CurrentGoal = strings.TrimSpace(raw.CurrentGoal)
	bundle.Summary = strings.TrimSpace(raw.Summary)
	bundle.InstructionArtifacts = normalizeInstructionArtifacts(raw.InstructionArtifacts)
	bundle.SettingsSnapshot = normalizeSettingsSnapshot(raw.SettingsSnapshot)
	bundle.ToolEvents = normalizeToolEvents(raw.ToolEvents)
	bundle.TouchedFiles = uniqueSortedStrings(raw.TouchedFiles)
	bundle.Decisions = normalizeDecisions(raw.Decisions)
	bundle.Failures = normalizeFailures(raw.Failures)
	bundle.ResumeHints = uniqueSortedStrings(raw.ResumeHints)
	bundle.TokenStats = normalizeTokenStats(raw.TokenStats)
	bundle.Provenance = uniqueSortedStrings(raw.Provenance)
	bundle.Redactions = uniqueSortedStrings(append(append([]string{}, raw.Redactions...), settingsRedactions(raw.SettingsSnapshot)...))
	bundle.Warnings = uniqueSortedStrings(raw.Warnings)

	if bundle.CurrentGoal == "" {
		bundle.CurrentGoal = bundle.TaskTitle
	}
	if bundle.Summary == "" && bundle.TaskTitle != "" {
		bundle.Summary = bundle.TaskTitle
	}

	if err := bundle.Validate(); err != nil {
		return domain.SessionBundle{}, err
	}
	return bundle, nil
}

func normalizeBundleID(raw RawImportResult) string {
	if raw.SourceSessionID != "" {
		return "bundle-" + raw.SourceSessionID
	}

	sum := sha256.Sum256([]byte(strings.Join([]string{
		string(raw.SourceTool),
		raw.ProjectRoot,
		raw.TaskTitle,
		raw.CurrentGoal,
		raw.ImportedAt,
	}, "|")))
	return "bundle-" + hex.EncodeToString(sum[:8])
}

func normalizeInstructionArtifacts(values []domain.InstructionArtifact) []domain.InstructionArtifact {
	if len(values) == 0 {
		return []domain.InstructionArtifact{}
	}

	seen := map[string]struct{}{}
	normalized := make([]domain.InstructionArtifact, 0, len(values))
	for _, value := range values {
		key := strings.Join([]string{string(value.Tool), value.Kind, value.Scope, value.Path, value.ContentHash}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, value)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].Path == normalized[j].Path {
			if normalized[i].Scope == normalized[j].Scope {
				return normalized[i].Kind < normalized[j].Kind
			}
			return normalized[i].Scope < normalized[j].Scope
		}
		return normalized[i].Path < normalized[j].Path
	})

	return normalized
}

func normalizeSettingsSnapshot(snapshot domain.SettingsSnapshot) domain.SettingsSnapshot {
	if snapshot.Included == nil {
		snapshot.Included = map[string]any{}
	}
	if snapshot.ExcludedKeys == nil {
		snapshot.ExcludedKeys = []string{}
	}
	sort.Strings(snapshot.ExcludedKeys)
	return snapshot
}

func normalizeToolEvents(values []domain.ToolEvent) []domain.ToolEvent {
	if len(values) == 0 {
		return []domain.ToolEvent{}
	}

	seen := map[string]struct{}{}
	normalized := make([]domain.ToolEvent, 0, len(values))
	for _, value := range values {
		key := strings.Join([]string{value.Type, value.Summary, value.Timestamp, value.Status, value.RawRef}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, value)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].Timestamp == normalized[j].Timestamp {
			if normalized[i].Type == normalized[j].Type {
				return normalized[i].Summary < normalized[j].Summary
			}
			return normalized[i].Type < normalized[j].Type
		}
		return normalized[i].Timestamp < normalized[j].Timestamp
	})

	return normalized
}

func normalizeDecisions(values []domain.Decision) []domain.Decision {
	if len(values) == 0 {
		return []domain.Decision{}
	}

	seen := map[string]struct{}{}
	normalized := make([]domain.Decision, 0, len(values))
	for _, value := range values {
		value.SourceRefs = uniqueSortedStrings(value.SourceRefs)
		key := strings.Join([]string{value.Summary, value.Reason, fmt.Sprintf("%.2f", value.Confidence), strings.Join(value.SourceRefs, "|")}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, value)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Summary < normalized[j].Summary
	})
	return normalized
}

func normalizeFailures(values []domain.Failure) []domain.Failure {
	if len(values) == 0 {
		return []domain.Failure{}
	}

	seen := map[string]struct{}{}
	normalized := make([]domain.Failure, 0, len(values))
	for _, value := range values {
		value.SourceRefs = uniqueSortedStrings(value.SourceRefs)
		key := strings.Join([]string{value.Summary, value.AttemptedFix, value.Status, strings.Join(value.SourceRefs, "|")}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, value)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Summary < normalized[j].Summary
	})
	return normalized
}

func normalizeTokenStats(values map[string]int64) map[string]int64 {
	if values == nil {
		return map[string]int64{}
	}

	normalized := make(map[string]int64, len(values))
	for key, value := range values {
		normalized[key] = value
	}
	return normalized
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	sort.Strings(values)
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(normalized) == 0 || normalized[len(normalized)-1] != value {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func settingsRedactions(snapshot domain.SettingsSnapshot) []string {
	if len(snapshot.ExcludedKeys) == 0 {
		return nil
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
