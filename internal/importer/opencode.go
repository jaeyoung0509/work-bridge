package importer

import (
	"fmt"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/inspect"
	_ "modernc.org/sqlite"
)

func init() {
	Register("opencode", &opencodeImporter{})
}

type opencodeImporter struct{}

func (i *opencodeImporter) ImportRaw(opts Options) (RawImportResult, error) {
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

	if err := importOpenCodeSQLite(session.StoragePath, session.ID, &raw); err != nil {
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

func summarizeOpenCode(session inspect.Session) string {
	if session.Title != "" {
		return fmt.Sprintf("Imported OpenCode session %q.", session.Title)
	}
	return "Imported OpenCode session."
}
