package importer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"sessionport/internal/domain"
	"sessionport/internal/inspect"
)

func importCodex(opts Options) (domain.SessionBundle, error) {
	report, err := inspect.Run(inspect.Options{
		FS:       opts.FS,
		CWD:      opts.CWD,
		HomeDir:  opts.HomeDir,
		Tool:     "codex",
		LookPath: opts.LookPath,
		Limit:    1 << 30,
	})
	if err != nil {
		return domain.SessionBundle{}, err
	}

	session, err := selectSession("codex", opts.Session, report.Sessions)
	if err != nil {
		return domain.SessionBundle{}, err
	}
	if session.StoragePath == "" {
		return domain.SessionBundle{}, fmt.Errorf("codex session %q has no backing storage path", session.ID)
	}

	bundle := domain.NewSessionBundle(domain.ToolCodex, session.ProjectRoot)
	bundle.BundleID = "bundle-" + session.ID
	bundle.SourceSessionID = session.ID
	bundle.ImportedAt = opts.ImportedAt
	bundle.TaskTitle = session.Title
	bundle.CurrentGoal = session.Title
	bundle.Summary = summarizeCodex(session)
	bundle.SettingsSnapshot = readSettingsSnapshot(opts.FS, report.Assets)
	bundle.InstructionArtifacts = readInstructionArtifacts(opts.FS, domain.ToolCodex, report.Assets)
	bundle.ResumeHints = []string{
		"source_session_path=" + session.StoragePath,
		"source_tool=codex",
	}
	bundle.Provenance = append(bundle.Provenance, report.Notes...)

	if err := importCodexSessionFile(opts, session.StoragePath, &bundle); err != nil {
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
		bundle.Summary = summarizeCodex(session)
	}

	if err := bundle.Validate(); err != nil {
		return domain.SessionBundle{}, err
	}
	return bundle, nil
}

func importCodexSessionFile(opts Options, path string, bundle *domain.SessionBundle) error {
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
				if bundle.SourceSessionID == "" {
					bundle.SourceSessionID = payload.ID
				}
				if bundle.ProjectRoot == "" {
					bundle.ProjectRoot = payload.CWD
				}
				bundle.Provenance = append(bundle.Provenance, "codex.session_meta")
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
				bundle.ToolEvents = append(bundle.ToolEvents, domain.ToolEvent{
					Type:      "tool_call",
					Summary:   summarizeCodexFunctionCall(envelope.Name, envelope.Arguments),
					Timestamp: record.Timestamp,
					Status:    "called",
					RawRef:    envelope.CallID,
				})
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
					bundle.TokenStats[key] = value
				}
			case "agent_message":
				if bundle.Summary == "" && envelope.Message != "" {
					bundle.Summary = truncateText(envelope.Message, 160)
				}
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
