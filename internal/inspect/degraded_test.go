package inspect

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"sessionport/internal/platform/fsx"
	"sessionport/internal/testutil"
)

func TestInspectCodexMissingSessionFixture(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "missing_session"))

	report, err := Run(Options{
		FS:       fsx.OSFS{},
		CWD:      fixture.WorkspaceDir,
		HomeDir:  fixture.HomeDir,
		Tool:     "codex",
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	if report.TotalSessions != 1 {
		t.Fatalf("expected one indexed session, got %#v", report)
	}
	if report.Sessions[0].StoragePath != "" {
		t.Fatalf("expected missing storage path, got %#v", report.Sessions[0])
	}
	if !containsNote(report.Notes, "backing JSONL file was not found") {
		t.Fatalf("expected missing session note, got %#v", report.Notes)
	}
}

func TestInspectGeminiMalformedHistoryFixture(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "gemini", "malformed_history"))

	report, err := Run(Options{
		FS:       fsx.OSFS{},
		CWD:      fixture.WorkspaceDir,
		HomeDir:  fixture.HomeDir,
		Tool:     "gemini",
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	if report.TotalSessions != 0 {
		t.Fatalf("expected zero importable sessions, got %#v", report)
	}
	if !containsNote(report.Notes, "could not be parsed") {
		t.Fatalf("expected malformed session note, got %#v", report.Notes)
	}
}

func containsNote(notes []string, want string) bool {
	for _, note := range notes {
		if strings.Contains(note, want) {
			return true
		}
	}
	return false
}
