package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	"sessionport/internal/domain"
	"sessionport/internal/inspect"
)

func importGemini(opts Options) (RawImportResult, error) {
	report, err := inspect.Run(inspectOptions(opts, "gemini"))
	if err != nil {
		return RawImportResult{}, err
	}

	session, err := selectSession("gemini", opts.Session, report.Sessions)
	if err != nil {
		return RawImportResult{}, err
	}
	if session.StoragePath == "" {
		return RawImportResult{}, fmt.Errorf("gemini session %q has no backing storage path", session.ID)
	}

	raw := newRawImportResult(domain.ToolGemini)
	raw.BundleID = "bundle-" + session.ID
	raw.SourceSessionID = session.ID
	raw.ImportedAt = opts.ImportedAt
	raw.ProjectRoot = session.ProjectRoot
	raw.TaskTitle = session.Title
	raw.CurrentGoal = session.Title
	raw.Summary = summarizeGemini(session)
	mergeSettings(&raw, readSettingsSnapshot(opts.FS, report.Assets, opts.Redaction))
	raw.InstructionArtifacts = readInstructionArtifacts(opts.FS, domain.ToolGemini, report.Assets)
	raw.ResumeHints = []string{
		"source_session_path=" + session.StoragePath,
		"source_tool=gemini",
	}
	raw.Provenance = append(raw.Provenance, report.Notes...)

	if err := importGeminiSessionFile(opts, session.StoragePath, &raw); err != nil {
		return RawImportResult{}, err
	}

	if raw.ProjectRoot == "" {
		raw.ProjectRoot = session.ProjectRoot
	}
	if raw.ProjectRoot == "" {
		raw.ProjectRoot = opts.CWD
		raw.Warnings = append(raw.Warnings, "Gemini session did not map to a known project root; importer fell back to the current workspace.")
	}
	if raw.TaskTitle == "" {
		raw.TaskTitle = session.Title
	}
	if raw.CurrentGoal == "" {
		raw.CurrentGoal = raw.TaskTitle
	}
	if raw.Summary == "" {
		raw.Summary = summarizeGemini(session)
	}
	return raw, nil
}

func importGeminiSessionFile(opts Options, path string, raw *RawImportResult) error {
	data, err := opts.FS.ReadFile(path)
	if err != nil {
		return err
	}

	var sessionData struct {
		SessionID   string `json:"sessionId"`
		StartTime   string `json:"startTime"`
		LastUpdated string `json:"lastUpdated"`
		Messages    []struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Content   any    `json:"content"`
			ToolCalls []struct {
				Name      string `json:"name"`
				Status    string `json:"status"`
				Timestamp string `json:"timestamp"`
				ID        string `json:"id"`
				Args      any    `json:"args"`
			} `json:"toolCalls"`
			Tokens map[string]int64 `json:"tokens"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &sessionData); err != nil {
		return err
	}

	if raw.SourceSessionID == "" {
		raw.SourceSessionID = sessionData.SessionID
	}

	for _, message := range sessionData.Messages {
		if raw.CurrentGoal == "" && message.Type == "user" {
			if text := geminiContentToText(message.Content); text != "" {
				raw.CurrentGoal = truncateText(text, 160)
				if raw.TaskTitle == "" {
					raw.TaskTitle = truncateText(text, 80)
				}
			}
		}
		if raw.Summary == "" && message.Type == "gemini" {
			if text := geminiContentToText(message.Content); text != "" {
				raw.Summary = truncateText(text, 160)
			}
		}
		addNarrativeSignals(raw, geminiContentToText(message.Content), message.Timestamp)

		for key, value := range message.Tokens {
			raw.TokenStats[key] += value
		}

		for _, call := range message.ToolCalls {
			addToolCallSignal(raw, domain.ToolEvent{
				Type:      "tool_call",
				Summary:   summarizeGeminiToolCall(call.Name, call.Args),
				Timestamp: call.Timestamp,
				Status:    call.Status,
				RawRef:    call.ID,
			}, call.Args)
		}
	}

	return nil
}

func summarizeGemini(session inspect.Session) string {
	if session.Title != "" {
		return fmt.Sprintf("Imported Gemini session %q.", session.Title)
	}
	return "Imported Gemini session."
}

func geminiContentToText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		parts := []string{}
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text, _ := block["text"].(string)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func summarizeGeminiToolCall(name string, args any) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unknown"
	}
	if args == nil {
		return name
	}
	data, err := json.Marshal(args)
	if err != nil {
		return name
	}
	return fmt.Sprintf("%s %s", name, truncateText(string(data), 120))
}
