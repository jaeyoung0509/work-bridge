package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	"sessionport/internal/domain"
	"sessionport/internal/inspect"
)

func importGemini(opts Options) (domain.SessionBundle, error) {
	report, err := inspect.Run(inspect.Options{
		FS:       opts.FS,
		CWD:      opts.CWD,
		HomeDir:  opts.HomeDir,
		Tool:     "gemini",
		LookPath: opts.LookPath,
		Limit:    1 << 30,
	})
	if err != nil {
		return domain.SessionBundle{}, err
	}

	session, err := selectSession("gemini", opts.Session, report.Sessions)
	if err != nil {
		return domain.SessionBundle{}, err
	}
	if session.StoragePath == "" {
		return domain.SessionBundle{}, fmt.Errorf("gemini session %q has no backing storage path", session.ID)
	}

	bundle := domain.NewSessionBundle(domain.ToolGemini, session.ProjectRoot)
	bundle.BundleID = "bundle-" + session.ID
	bundle.SourceSessionID = session.ID
	bundle.ImportedAt = opts.ImportedAt
	bundle.TaskTitle = session.Title
	bundle.CurrentGoal = session.Title
	bundle.Summary = summarizeGemini(session)
	bundle.SettingsSnapshot = readSettingsSnapshot(opts.FS, report.Assets)
	bundle.InstructionArtifacts = readInstructionArtifacts(opts.FS, domain.ToolGemini, report.Assets)
	bundle.ResumeHints = []string{
		"source_session_path=" + session.StoragePath,
		"source_tool=gemini",
	}
	bundle.Provenance = append(bundle.Provenance, report.Notes...)

	if err := importGeminiSessionFile(opts, session.StoragePath, &bundle); err != nil {
		return domain.SessionBundle{}, err
	}

	if bundle.ProjectRoot == "" {
		bundle.ProjectRoot = session.ProjectRoot
	}
	if bundle.TaskTitle == "" {
		bundle.TaskTitle = session.Title
	}
	if bundle.CurrentGoal == "" {
		bundle.CurrentGoal = bundle.TaskTitle
	}
	if bundle.Summary == "" {
		bundle.Summary = summarizeGemini(session)
	}

	if err := bundle.Validate(); err != nil {
		return domain.SessionBundle{}, err
	}
	return bundle, nil
}

func importGeminiSessionFile(opts Options, path string, bundle *domain.SessionBundle) error {
	data, err := opts.FS.ReadFile(path)
	if err != nil {
		return err
	}

	var raw struct {
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
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if bundle.SourceSessionID == "" {
		bundle.SourceSessionID = raw.SessionID
	}

	for _, message := range raw.Messages {
		if bundle.CurrentGoal == "" && message.Type == "user" {
			if text := geminiContentToText(message.Content); text != "" {
				bundle.CurrentGoal = truncateText(text, 160)
				if bundle.TaskTitle == "" {
					bundle.TaskTitle = truncateText(text, 80)
				}
			}
		}
		if bundle.Summary == "" && message.Type == "gemini" {
			if text := geminiContentToText(message.Content); text != "" {
				bundle.Summary = truncateText(text, 160)
			}
		}

		for key, value := range message.Tokens {
			bundle.TokenStats[key] += value
		}

		for _, call := range message.ToolCalls {
			bundle.ToolEvents = append(bundle.ToolEvents, domain.ToolEvent{
				Type:      "tool_call",
				Summary:   summarizeGeminiToolCall(call.Name, call.Args),
				Timestamp: call.Timestamp,
				Status:    call.Status,
				RawRef:    call.ID,
			})
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
