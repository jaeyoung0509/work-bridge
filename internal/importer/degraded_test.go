package importer

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
	"github.com/jaeyoung0509/work-bridge/internal/testutil"
)

func TestImportCodexMissingSessionFixture(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "missing_session"))

	_, err := Import(Options{
		FS:         fsx.OSFS{},
		CWD:        fixture.WorkspaceDir,
		HomeDir:    fixture.HomeDir,
		Tool:       "codex",
		Session:    "latest",
		ImportedAt: "2026-04-07T16:00:00Z",
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
	})
	if err == nil || err.Error() != `codex session "missing-codex-session" has no backing storage path` {
		t.Fatalf("expected missing backing storage error, got %v", err)
	}
}

func TestImportCodexSecretRedactionFixture(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "secret_redaction"))

	bundle, err := Import(Options{
		FS:         fsx.OSFS{},
		CWD:        fixture.WorkspaceDir,
		HomeDir:    fixture.HomeDir,
		Tool:       "codex",
		Session:    "latest",
		ImportedAt: "2026-04-07T16:00:00Z",
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if bundle.SettingsSnapshot.Included["theme"] != "ansi" {
		t.Fatalf("expected theme to stay included, got %#v", bundle.SettingsSnapshot.Included)
	}
	for _, key := range []string{"api_key", "oauth_token", "workspace"} {
		if !slices.Contains(bundle.SettingsSnapshot.ExcludedKeys, key) {
			t.Fatalf("expected excluded key %q in %#v", key, bundle.SettingsSnapshot.ExcludedKeys)
		}
	}
}

func TestImportGeminiUnmappedProjectFallsBackToWorkspace(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "gemini", "unmapped_project"))

	bundle, err := Import(Options{
		FS:         fsx.OSFS{},
		CWD:        fixture.WorkspaceDir,
		HomeDir:    fixture.HomeDir,
		Tool:       "gemini",
		Session:    "latest",
		ImportedAt: "2026-04-07T16:00:00Z",
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if bundle.ProjectRoot != fixture.WorkspaceDir {
		t.Fatalf("expected fallback project root %q, got %q", fixture.WorkspaceDir, bundle.ProjectRoot)
	}
	if !slices.Contains(bundle.Warnings, "Gemini session did not map to a known project root; importer fell back to the current workspace.") {
		t.Fatalf("expected workspace fallback warning, got %#v", bundle.Warnings)
	}
}

func TestImportRespectsAdditionalSensitiveKeys(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".codex", "config.toml"), ""+
		"model = \"gpt-5\"\n"+
		"workspace_secret = \"value\"\n")
	writeFile(t, filepath.Join(homeDir, ".codex", "session_index.jsonl"),
		`{"id":"codex-session","thread_name":"codex task","updated_at":"2026-04-07T15:00:00Z"}`+"\n")
	writeFile(t, filepath.Join(homeDir, ".codex", "sessions", "2026", "04", "07", "rollout-2026-04-07T15-00-00-codex-session.jsonl"),
		`{"timestamp":"2026-04-07T14:59:00Z","type":"session_meta","payload":{"id":"codex-session","timestamp":"2026-04-07T14:59:00Z","cwd":"/workspace/codex"}}`+"\n")

	bundle, err := Import(Options{
		FS:         fsx.OSFS{},
		CWD:        cwd,
		HomeDir:    homeDir,
		Tool:       "codex",
		Session:    "latest",
		ImportedAt: "2026-04-07T16:00:00Z",
		Redaction: domain.RedactionPolicy{
			AdditionalSensitiveKeys: []string{"workspace_secret"},
			DetectSensitiveValues:   true,
		},
		LookPath: func(string) (string, error) {
			return "", errors.New("not found")
		},
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if _, ok := bundle.SettingsSnapshot.Included["workspace_secret"]; ok {
		t.Fatalf("expected workspace_secret to be redacted, got %#v", bundle.SettingsSnapshot.Included)
	}
	if !slices.Contains(bundle.SettingsSnapshot.ExcludedKeys, "workspace_secret") {
		t.Fatalf("expected excluded workspace_secret, got %#v", bundle.SettingsSnapshot.ExcludedKeys)
	}
}
