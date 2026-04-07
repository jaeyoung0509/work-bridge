package importer

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"sessionport/internal/platform/fsx"
	"sessionport/internal/testutil"
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
