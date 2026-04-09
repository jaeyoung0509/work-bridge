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

func TestImportRawNormalizedBundlesSatisfyContract(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)

	cases := []struct {
		name    string
		tool    string
		fixture string
		session string
	}{
		{name: "codex", tool: "codex", fixture: "basic_latest", session: "latest"},
		{name: "gemini", tool: "gemini", fixture: "explicit_session", session: "gemini-session"},
		{name: "claude", tool: "claude", fixture: "partial_history", session: "latest"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", tc.tool, tc.fixture))

			raw, err := ImportRaw(Options{
				FS:         fsx.OSFS{},
				CWD:        fixture.WorkspaceDir,
				HomeDir:    fixture.HomeDir,
				Tool:       tc.tool,
				Session:    tc.session,
				ImportedAt: "2026-04-07T16:00:00Z",
				LookPath:   func(string) (string, error) { return "", errors.New("not found") },
			})
			if err != nil {
				t.Fatalf("ImportRaw failed: %v", err)
			}

			bundle, err := NewSessionNormalizer().Normalize(raw)
			if err != nil {
				t.Fatalf("Normalize failed: %v", err)
			}
			if err := bundle.Validate(); err != nil {
				t.Fatalf("Validate failed: %v", err)
			}
			if bundle.AssetKind != domain.AssetKindSession {
				t.Fatalf("expected asset kind %q, got %q", domain.AssetKindSession, bundle.AssetKind)
			}
			if bundle.ProjectRoot == "" {
				t.Fatal("expected project_root to be populated")
			}
			if bundle.SettingsSnapshot.Included == nil || bundle.SettingsSnapshot.ExcludedKeys == nil {
				t.Fatalf("expected initialized settings snapshot, got %#v", bundle.SettingsSnapshot)
			}
			if bundle.TokenStats == nil || bundle.Provenance == nil || bundle.Redactions == nil || bundle.Warnings == nil {
				t.Fatalf("expected initialized normalized collections, got %#v", bundle)
			}
			if len(bundle.SettingsSnapshot.ExcludedKeys) > 0 {
				for _, key := range bundle.SettingsSnapshot.ExcludedKeys {
					if !slices.Contains(bundle.Redactions, "settings."+key) {
						t.Fatalf("expected redaction for key %q in %#v", key, bundle.Redactions)
					}
				}
			}
		})
	}
}
