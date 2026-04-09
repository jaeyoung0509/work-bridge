package importer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/inspect"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
)

func importClaude(opts Options) (RawImportResult, error) {
	report, err := inspect.Run(inspectOptions(opts, "claude"))
	if err != nil {
		return RawImportResult{}, err
	}

	session, err := selectSession("claude", opts.Session, report.Sessions)
	if err != nil {
		return RawImportResult{}, err
	}

	raw := newRawImportResult(domain.ToolClaude)
	raw.BundleID = "bundle-" + session.ID
	raw.SourceSessionID = session.ID
	raw.ImportedAt = opts.ImportedAt
	raw.ProjectRoot = session.ProjectRoot
	raw.TaskTitle = session.Title
	raw.CurrentGoal = session.Title
	raw.Summary = summarizeClaude(session)
	mergeSettings(&raw, readSettingsSnapshot(opts.FS, report.Assets, opts.Redaction))
	raw.InstructionArtifacts = readInstructionArtifacts(opts.FS, domain.ToolClaude, report.Assets)
	claudeRoot := defaultToolRoot(opts, domain.ToolClaude)
	raw.ResumeHints = []string{
		"source_history_path=" + filepath.Join(claudeRoot, "history.jsonl"),
		"source_tool=claude",
	}
	raw.Provenance = append(raw.Provenance, report.Notes...)
	raw.Warnings = append(raw.Warnings,
		"Claude import is history-based. Raw transcript, tool events, and token usage were not available from local session storage.",
	)

	if err := importClaudeHistory(opts, session.ID, &raw); err != nil {
		return RawImportResult{}, err
	}
	importClaudeTranscriptCandidates(opts, session.ID, &raw)

	if raw.ProjectRoot == "" {
		raw.ProjectRoot = session.ProjectRoot
	}
	if raw.TaskTitle == "" {
		raw.TaskTitle = session.Title
	}
	if raw.CurrentGoal == "" {
		raw.CurrentGoal = raw.TaskTitle
	}
	if raw.Summary == "" {
		raw.Summary = summarizeClaude(session)
	}
	return raw, nil
}

func importClaudeHistory(opts Options, sessionID string, raw *RawImportResult) error {
	path := filepath.Join(defaultToolRoot(opts, domain.ToolClaude), "history.jsonl")
	data, err := opts.FS.ReadFile(path)
	if err != nil {
		return err
	}

	lines, err := splitJSONLLines(data)
	if err != nil {
		return err
	}

	matchCount := 0
	latestDisplay := ""
	var latestTS int64

	for _, line := range lines {
		var entry struct {
			Display   string `json:"display"`
			Timestamp int64  `json:"timestamp"`
			Project   string `json:"project"`
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.SessionID != sessionID {
			continue
		}

		matchCount++
		if raw.ProjectRoot == "" && entry.Project != "" {
			raw.ProjectRoot = entry.Project
		}
		if entry.Timestamp >= latestTS {
			latestTS = entry.Timestamp
			if entry.Display != "" {
				latestDisplay = entry.Display
			}
		}
		addNarrativeSignals(raw, entry.Display, fmt.Sprintf("claude.history.%d", entry.Timestamp))
	}

	if matchCount == 0 {
		return &SessionNotFoundError{Tool: "claude", Session: sessionID}
	}

	if latestDisplay != "" {
		raw.TaskTitle = latestDisplay
		raw.CurrentGoal = latestDisplay
	}
	raw.Provenance = append(raw.Provenance, fmt.Sprintf("claude.history_entries=%d", matchCount))
	raw.ResumeHints = append(raw.ResumeHints, fmt.Sprintf("history_entries=%d", matchCount))
	return nil
}

func importClaudeTranscriptCandidates(opts Options, sessionID string, raw *RawImportResult) {
	root := defaultToolRoot(opts, domain.ToolClaude)
	files, err := inspectClaudeTranscriptCandidates(opts, root, sessionID)
	if err != nil || len(files) == 0 {
		return
	}

	augmented := 0
	for _, path := range files {
		data, err := opts.FS.ReadFile(path)
		if err != nil {
			continue
		}
		lines, err := splitJSONLLines(data)
		if err != nil || len(lines) == 0 {
			continue
		}

		for _, line := range lines {
			var payload map[string]any
			if err := json.Unmarshal(line, &payload); err != nil {
				continue
			}

			text := extractClaudeTranscriptText(payload)
			if raw.Summary == "" && text != "" {
				raw.Summary = truncateText(text, 160)
			}
			addNarrativeSignals(raw, text, path)

			if event, ok := buildClaudeToolEvent(payload); ok {
				addToolCallSignal(raw, event, payload)
			}
			augmented++
		}
		raw.ResumeHints = append(raw.ResumeHints, "source_session_path="+path)
	}

	if augmented > 0 {
		raw.Provenance = append(raw.Provenance, fmt.Sprintf("claude.transcript_records=%d", augmented))
		raw.Warnings = append(raw.Warnings, "Claude transcript augmentation used best-effort local session storage and may omit unsupported record types.")
	}
}

func inspectClaudeTranscriptCandidates(opts Options, root string, sessionID string) ([]string, error) {
	files, err := listFilesRecursiveImporter(opts.FS, root)
	if err != nil {
		return nil, err
	}

	candidates := []string{}
	for _, path := range files {
		if !strings.Contains(path, sessionID) {
			continue
		}
		if path == filepath.Join(root, "history.jsonl") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".jsonl" {
			continue
		}
		candidates = append(candidates, path)
	}
	return candidates, nil
}

func listFilesRecursiveImporter(fs fsx.FS, root string) ([]string, error) {
	info, err := fs.Stat(root)
	if err != nil {
		return nil, nil
	}
	if !info.IsDir() {
		return []string{root}, nil
	}

	queue := []string{root}
	files := []string{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		entries, err := fs.ReadDir(current)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			path := filepath.Join(current, entry.Name())
			if entry.IsDir() {
				queue = append(queue, path)
				continue
			}
			files = append(files, path)
		}
	}
	return files, nil
}

func extractClaudeTranscriptText(payload map[string]any) string {
	for _, key := range []string{"message", "text", "content", "summary", "display"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func buildClaudeToolEvent(payload map[string]any) (domain.ToolEvent, bool) {
	name, _ := payload["tool"].(string)
	if name == "" {
		name, _ = payload["name"].(string)
	}
	if strings.TrimSpace(name) == "" {
		return domain.ToolEvent{}, false
	}

	status, _ := payload["status"].(string)
	ref, _ := payload["id"].(string)
	timestamp, _ := payload["timestamp"].(string)
	return domain.ToolEvent{
		Type:      "tool_call",
		Summary:   strings.TrimSpace(name),
		Status:    strings.TrimSpace(status),
		Timestamp: strings.TrimSpace(timestamp),
		RawRef:    strings.TrimSpace(ref),
	}, true
}

func summarizeClaude(session inspect.Session) string {
	if session.Title != "" {
		return fmt.Sprintf("Imported Claude session %q from history.", session.Title)
	}
	return "Imported Claude session from history."
}
