package importer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/inspect"
)

func importCodex(opts Options) (RawImportResult, error) {
	report, err := inspect.Run(inspectOptions(opts, "codex"))
	if err != nil {
		return RawImportResult{}, err
	}

	session, err := selectSession("codex", opts.Session, report.Sessions)
	if err != nil {
		return RawImportResult{}, err
	}
	if session.StoragePath == "" {
		return RawImportResult{}, fmt.Errorf("codex session %q has no backing storage path", session.ID)
	}

	raw := newRawImportResult(domain.ToolCodex)
	raw.BundleID = "bundle-" + session.ID
	raw.SourceSessionID = session.ID
	raw.ImportedAt = opts.ImportedAt
	raw.ProjectRoot = session.ProjectRoot
	raw.TaskTitle = session.Title
	raw.CurrentGoal = session.Title
	raw.Summary = summarizeCodex(session)
	mergeSettings(&raw, readSettingsSnapshot(opts.FS, report.Assets, opts.Redaction))
	raw.InstructionArtifacts = readInstructionArtifacts(opts.FS, domain.ToolCodex, report.Assets)
	raw.ResumeHints = []string{
		"source_session_path=" + session.StoragePath,
		"source_tool=codex",
	}
	raw.Provenance = append(raw.Provenance, report.Notes...)

	if err := importCodexSessionFile(opts, session.StoragePath, &raw); err != nil {
		return RawImportResult{}, err
	}

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
		raw.Summary = summarizeCodex(session)
	}
	return raw, nil
}

func importCodexSessionFile(opts Options, path string, raw *RawImportResult) error {
	data, err := opts.FS.ReadFile(path)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var record struct {
			Timestamp string          `json:"timestamp"`
			Type      string          `json:"type"`
			Payload   json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}

		switch record.Type {
		case "session_meta":
			var payload struct {
				ID        string `json:"id"`
				Timestamp string `json:"timestamp"`
				CWD       string `json:"cwd"`
			}
			if err := json.Unmarshal(record.Payload, &payload); err == nil {
				if raw.SourceSessionID == "" {
					raw.SourceSessionID = payload.ID
				}
				if raw.ProjectRoot == "" {
					raw.ProjectRoot = payload.CWD
				}
				raw.Provenance = append(raw.Provenance, "codex.session_meta")
			}
		case "response_item":
			var envelope struct {
				Type      string `json:"type"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
				CallID    string `json:"call_id"`
				Role      string `json:"role"`
			}
			if err := json.Unmarshal(record.Payload, &envelope); err != nil {
				continue
			}
			if envelope.Type == "function_call" {
				addToolCallSignal(raw, domain.ToolEvent{
					Type:      "tool_call",
					Summary:   summarizeCodexFunctionCall(envelope.Name, envelope.Arguments),
					Timestamp: record.Timestamp,
					Status:    "called",
					RawRef:    envelope.CallID,
				}, envelope.Arguments)
			}
		case "event_msg":
			var envelope struct {
				Type string `json:"type"`
				Info struct {
					TotalTokenUsage map[string]int64 `json:"total_token_usage"`
				} `json:"info"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(record.Payload, &envelope); err != nil {
				continue
			}
			switch envelope.Type {
			case "token_count":
				for key, value := range envelope.Info.TotalTokenUsage {
					raw.TokenStats[key] = value
				}
			case "agent_message":
				if raw.Summary == "" && envelope.Message != "" {
					raw.Summary = truncateText(envelope.Message, 160)
				}
				addNarrativeSignals(raw, envelope.Message, record.Timestamp)
			}
		}
	}

	return scanner.Err()
}

func summarizeCodex(session inspect.Session) string {
	if session.Title != "" {
		return fmt.Sprintf("Imported Codex session %q.", session.Title)
	}
	return "Imported Codex session."
}

func summarizeCodexFunctionCall(name string, arguments string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unknown"
	}
	if arguments == "" {
		return name
	}
	return fmt.Sprintf("%s %s", name, truncateText(arguments, 120))
}
