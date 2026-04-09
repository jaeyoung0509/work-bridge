package importer

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
	"github.com/jaeyoung0509/work-bridge/internal/testutil"
)

func TestImportFixturesMatchGolden(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)

	cases := []struct {
		name       string
		tool       string
		fixture    string
		session    string
		importedAt string
	}{
		{
			name:       "codex basic latest",
			tool:       "codex",
			fixture:    "basic_latest",
			session:    "latest",
			importedAt: "2026-04-07T16:00:00Z",
		},
		{
			name:       "gemini explicit session",
			tool:       "gemini",
			fixture:    "explicit_session",
			session:    "gemini-session",
			importedAt: "2026-04-07T16:00:00Z",
		},
		{
			name:       "claude partial history",
			tool:       "claude",
			fixture:    "partial_history",
			session:    "latest",
			importedAt: "2026-04-07T16:00:00Z",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", tc.tool, tc.fixture))

			bundle, err := Import(Options{
				FS:         fsx.OSFS{},
				CWD:        fixture.WorkspaceDir,
				HomeDir:    fixture.HomeDir,
				Tool:       tc.tool,
				Session:    tc.session,
				ImportedAt: tc.importedAt,
				LookPath:   func(string) (string, error) { return "", errors.New("not found") },
			})
			if err != nil {
				t.Fatalf("import failed: %v", err)
			}

			normalized := normalizeBundleForGolden(bundle, fixture)
			goldenPath := filepath.Join(repoRoot, "testdata", "golden", "importer", tc.tool, tc.fixture+".json")
			testutil.AssertGoldenJSON(t, goldenPath, normalized)
		})
	}
}

func normalizeBundleForGolden(bundle domain.SessionBundle, fixture testutil.Fixture) domain.SessionBundle {
	bundle.ProjectRoot = normalizeGoldenPath(bundle.ProjectRoot, fixture)

	for index := range bundle.InstructionArtifacts {
		bundle.InstructionArtifacts[index].Path = normalizeGoldenPath(bundle.InstructionArtifacts[index].Path, fixture)
	}

	for index, hint := range bundle.ResumeHints {
		if strings.HasPrefix(hint, "source_session_path=") {
			path := strings.TrimPrefix(hint, "source_session_path=")
			bundle.ResumeHints[index] = "source_session_path=" + normalizeGoldenPath(path, fixture)
			continue
		}
		if strings.HasPrefix(hint, "source_history_path=") {
			path := strings.TrimPrefix(hint, "source_history_path=")
			bundle.ResumeHints[index] = "source_history_path=" + normalizeGoldenPath(path, fixture)
		}
	}

	return bundle
}

func normalizeGoldenPath(value string, fixture testutil.Fixture) string {
	value = strings.ReplaceAll(value, filepath.Clean(fixture.HomeDir), "$HOME")
	value = strings.ReplaceAll(value, filepath.Clean(fixture.WorkspaceDir), "$WORKSPACE")
	return filepath.ToSlash(value)
}
