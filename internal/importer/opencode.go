package importer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/inspect"
)

func importOpenCode(opts Options) (RawImportResult, error) {
	report, err := inspect.Run(inspectOptions(opts, "opencode"))
	if err != nil {
		return RawImportResult{}, err
	}

	session, err := selectSession("opencode", opts.Session, report.Sessions)
	if err != nil {
		return RawImportResult{}, err
	}
	if session.StoragePath == "" {
		return RawImportResult{}, fmt.Errorf("opencode session %q has no backing storage path", session.ID)
	}

	raw := newRawImportResult(domain.ToolOpenCode)
	raw.BundleID = "bundle-" + session.ID
	raw.SourceSessionID = session.ID
	raw.ImportedAt = opts.ImportedAt
	raw.ProjectRoot = session.ProjectRoot
	raw.TaskTitle = session.Title
	raw.CurrentGoal = session.Title
	raw.Summary = summarizeOpenCode(session)
	mergeSettings(&raw, readSettingsSnapshot(opts.FS, report.Assets, opts.Redaction))
	raw.InstructionArtifacts = readInstructionArtifacts(opts.FS, domain.ToolOpenCode, report.Assets)
	raw.ResumeHints = []string{
		"source_session_path=" + session.StoragePath,
		"source_tool=opencode",
	}
	raw.Provenance = append(raw.Provenance, report.Notes...)

	if err := importOpenCodeSessionFile(opts, session.StoragePath, &raw); err != nil {
		return RawImportResult{}, err
	}

	if raw.ProjectRoot == "" {
		raw.ProjectRoot = session.ProjectRoot
	}
	if raw.ProjectRoot == "" {
		raw.ProjectRoot = opts.CWD
	}
	if raw.TaskTitle == "" {
		raw.TaskTitle = session.Title
	}
	if raw.CurrentGoal == "" {
		raw.CurrentGoal = raw.TaskTitle
	}
	if raw.Summary == "" {
		raw.Summary = summarizeOpenCode(session)
	}
	if len(raw.Warnings) == 0 {
		raw.Warnings = append(raw.Warnings, "OpenCode import is best-effort and may omit provider-specific runtime state.")
	}
	return raw, nil
}

func importOpenCodeSessionFile(opts Options, path string, raw *RawImportResult) error {
	data, err := opts.FS.ReadFile(path)
	if err != nil {
		return err
	}

	entries, err := splitOpenCodeRecords(data)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		var payload map[string]any
		if err := json.Unmarshal(stripJSONCComments(data), &payload); err != nil {
			return err
		}
		entries = append(entries, payload)
	}

	for _, payload := range entries {
		if raw.SourceSessionID == "" {
			if id := stringField(payload, "id", "sessionId", "session_id"); id != "" {
				raw.SourceSessionID = id
			}
		}
		if raw.TaskTitle == "" {
			if title := stringField(payload, "title", "display", "prompt", "name"); title != "" {
				raw.TaskTitle = truncateText(title, 80)
				raw.CurrentGoal = raw.TaskTitle
			}
		}
		if raw.ProjectRoot == "" {
			raw.ProjectRoot = stringField(payload, "projectRoot", "project_root", "cwd", "dir")
		}
		if summary := stringField(payload, "summary", "message", "content"); raw.Summary == "" && summary != "" {
			raw.Summary = truncateText(summary, 160)
		}
		if ts := stringField(payload, "timestamp", "createdAt", "created_at", "updatedAt", "updated_at"); ts != "" {
			raw.ToolEvents = append(raw.ToolEvents, domain.ToolEvent{
				Type:      "session_record",
				Summary:   truncateText(raw.TaskTitle, 120),
				Timestamp: ts,
				Status:    "observed",
				RawRef:    path,
			})
		}

		if msgs, ok := payload["messages"].([]any); ok {
			raw.ResumeHints = append(raw.ResumeHints, fmt.Sprintf("message_count=%d", len(msgs)))
			for _, item := range msgs {
				msg, ok := item.(map[string]any)
				if !ok {
					continue
				}
				text := stringField(msg, "text", "content", "message", "summary")
				if raw.Summary == "" && text != "" {
					raw.Summary = truncateText(text, 160)
				}
				addNarrativeSignals(raw, text, path)
			}
		}

		if files := stringSlice(payload, "files", "touchedFiles", "touched_files"); len(files) > 0 {
			raw.TouchedFiles = append(raw.TouchedFiles, files...)
		}
	}

	return nil
}

func splitOpenCodeRecords(data []byte) ([]map[string]any, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)

	records := []map[string]any{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(stripJSONCComments([]byte(line)), &payload); err != nil {
			continue
		}
		records = append(records, payload)
	}
	return records, scanner.Err()
}

func summarizeOpenCode(session inspect.Session) string {
	if session.Title != "" {
		return fmt.Sprintf("Imported OpenCode session %q.", session.Title)
	}
	return "Imported OpenCode session."
}

func stringSlice(raw map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case []any:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, s)
				}
			}
			if len(out) > 0 {
				return out
			}
		case []string:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				if strings.TrimSpace(item) != "" {
					out = append(out, item)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return nil
}

func stripJSONCComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return data
	}
	var b strings.Builder
	inBlock := false
	for _, line := range lines {
		text := line
		if inBlock {
			if end := strings.Index(text, "*/"); end >= 0 {
				inBlock = false
				text = text[end+2:]
			} else {
				continue
			}
		}
		for {
			start := strings.Index(text, "/*")
			if start < 0 {
				break
			}
			end := strings.Index(text[start+2:], "*/")
			if end < 0 {
				text = text[:start]
				inBlock = true
				break
			}
			text = text[:start] + text[start+2+end+2:]
		}
		if idx := strings.Index(text, "//"); idx >= 0 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		text = strings.TrimRight(text, ",")
		b.WriteString(text)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}
