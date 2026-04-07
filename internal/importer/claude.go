package importer

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"sessionport/internal/domain"
	"sessionport/internal/inspect"
)

func importClaude(opts Options) (domain.SessionBundle, error) {
	report, err := inspect.Run(inspect.Options{
		FS:       opts.FS,
		CWD:      opts.CWD,
		HomeDir:  opts.HomeDir,
		Tool:     "claude",
		LookPath: opts.LookPath,
		Limit:    1 << 30,
	})
	if err != nil {
		return domain.SessionBundle{}, err
	}

	session, err := selectSession("claude", opts.Session, report.Sessions)
	if err != nil {
		return domain.SessionBundle{}, err
	}

	bundle := domain.NewSessionBundle(domain.ToolClaude, session.ProjectRoot)
	bundle.BundleID = "bundle-" + session.ID
	bundle.SourceSessionID = session.ID
	bundle.ImportedAt = opts.ImportedAt
	bundle.TaskTitle = session.Title
	bundle.CurrentGoal = session.Title
	bundle.Summary = summarizeClaude(session)
	bundle.SettingsSnapshot = readSettingsSnapshot(opts.FS, report.Assets)
	bundle.InstructionArtifacts = readInstructionArtifacts(opts.FS, domain.ToolClaude, report.Assets)
	bundle.ResumeHints = []string{
		"source_history_path=" + filepath.Join(opts.HomeDir, ".claude", "history.jsonl"),
		"source_tool=claude",
	}
	bundle.Provenance = append(bundle.Provenance, report.Notes...)
	bundle.Warnings = append(bundle.Warnings,
		"Claude import is history-based. Raw transcript, tool events, and token usage were not available from local session storage.",
	)

	if err := importClaudeHistory(opts, session.ID, &bundle); err != nil {
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
		bundle.Summary = summarizeClaude(session)
	}

	if err := bundle.Validate(); err != nil {
		return domain.SessionBundle{}, err
	}
	return bundle, nil
}

func importClaudeHistory(opts Options, sessionID string, bundle *domain.SessionBundle) error {
	path := filepath.Join(opts.HomeDir, ".claude", "history.jsonl")
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
		if bundle.ProjectRoot == "" && entry.Project != "" {
			bundle.ProjectRoot = entry.Project
		}
		if entry.Timestamp >= latestTS {
			latestTS = entry.Timestamp
			if entry.Display != "" {
				latestDisplay = entry.Display
			}
		}
	}

	if matchCount == 0 {
		return &SessionNotFoundError{Tool: "claude", Session: sessionID}
	}

	if latestDisplay != "" {
		bundle.TaskTitle = latestDisplay
		bundle.CurrentGoal = latestDisplay
	}
	bundle.Provenance = append(bundle.Provenance, fmt.Sprintf("claude.history_entries=%d", matchCount))
	bundle.ResumeHints = append(bundle.ResumeHints, fmt.Sprintf("history_entries=%d", matchCount))
	return nil
}

func summarizeClaude(session inspect.Session) string {
	if session.Title != "" {
		return fmt.Sprintf("Imported Claude session %q from history.", session.Title)
	}
	return "Imported Claude session from history."
}
