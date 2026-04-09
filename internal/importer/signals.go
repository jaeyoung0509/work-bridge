package importer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"sessionport/internal/domain"
)

var (
	commandPathPattern = regexp.MustCompile(`(^|[[:space:]"'])(([A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+\.[A-Za-z0-9_.-]+)($|[[:space:]"'])`)
	patchPathPattern   = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Delete|Update) File: (.+)$`)
)

func addToolCallSignal(raw *RawImportResult, event domain.ToolEvent, args any) {
	raw.ToolEvents = append(raw.ToolEvents, event)
	raw.TouchedFiles = append(raw.TouchedFiles, extractTouchedFiles(args)...)

	if status := strings.ToLower(strings.TrimSpace(event.Status)); status != "" && status != "success" && status != "ok" && status != "called" {
		raw.Failures = append(raw.Failures, domain.Failure{
			Summary:    truncateText(fmt.Sprintf("tool call %s returned status %s", event.Summary, event.Status), 160),
			Status:     status,
			SourceRefs: sourceRefs(event.RawRef),
		})
	}
}

func addNarrativeSignals(raw *RawImportResult, text string, sourceRef string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	lower := strings.ToLower(text)
	if strings.Contains(lower, "decision:") || strings.Contains(lower, "decided to") || strings.Contains(lower, "we should ") {
		raw.Decisions = append(raw.Decisions, domain.Decision{
			Summary:    truncateText(text, 160),
			Confidence: 0.6,
			SourceRefs: sourceRefs(sourceRef),
		})
	}

	if strings.Contains(lower, "failure:") || strings.Contains(lower, "failed") || strings.Contains(lower, "unable to") || strings.Contains(lower, "could not") || strings.Contains(lower, "error:") {
		raw.Failures = append(raw.Failures, domain.Failure{
			Summary:    truncateText(text, 160),
			Status:     "reported",
			SourceRefs: sourceRefs(sourceRef),
		})
	}
}

func extractTouchedFiles(value any) []string {
	paths := extractTouchedFilesRecursive("", value)
	if len(paths) == 0 {
		return []string{}
	}

	slices.Sort(paths)
	return slices.Compact(paths)
}

func extractTouchedFilesRecursive(parentKey string, value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return pathCandidates(parentKey, typed)
	case []any:
		paths := []string{}
		for _, item := range typed {
			paths = append(paths, extractTouchedFilesRecursive(parentKey, item)...)
		}
		return paths
	case map[string]any:
		paths := []string{}
		for key, item := range typed {
			paths = append(paths, extractTouchedFilesRecursive(strings.ToLower(key), item)...)
		}
		return paths
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return nil
		}

		var generic any
		if err := json.Unmarshal(data, &generic); err != nil {
			return nil
		}
		return extractTouchedFilesRecursive(parentKey, generic)
	}
}

func pathCandidates(key string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	if key == "cmd" || key == "command" || key == "arguments" {
		return extractPathsFromCommand(value)
	}
	if strings.Contains(strings.ToLower(key), "patch") {
		return extractPathsFromPatch(value)
	}

	if looksLikePathKey(key) && looksLikeFilePath(value) {
		return []string{filepath.Clean(value)}
	}

	if json.Valid([]byte(value)) {
		var nested any
		if err := json.Unmarshal([]byte(value), &nested); err == nil {
			return extractTouchedFilesRecursive(key, nested)
		}
	}

	return nil
}

func looksLikePathKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	for _, needle := range []string{"path", "file", "filepath", "filename", "target", "source", "destination", "output", "input"} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

func looksLikeFilePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.Contains(value, "\n") {
		return false
	}
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		return false
	}
	if strings.Contains(value, string(filepath.Separator)) {
		return true
	}
	base := filepath.Base(value)
	return strings.Contains(base, ".") && !strings.HasPrefix(base, ".")
}

func extractPathsFromCommand(command string) []string {
	matches := commandPathPattern.FindAllStringSubmatch(command, -1)
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		candidate := strings.TrimSpace(match[2])
		if candidate == "" || strings.HasPrefix(candidate, "-") {
			continue
		}
		paths = append(paths, filepath.Clean(candidate))
	}
	return paths
}

func extractPathsFromPatch(patch string) []string {
	matches := patchPathPattern.FindAllStringSubmatch(patch, -1)
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		paths = append(paths, filepath.Clean(strings.TrimSpace(match[1])))
	}
	return paths
}

func sourceRefs(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	return []string{value}
}
