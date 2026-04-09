package doctor

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/importer"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
	"github.com/jaeyoung0509/work-bridge/internal/testutil"
)

func TestDoctorFixturesMatchGolden(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)

	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))
	bundle, err := importer.Import(importer.Options{
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

	report, err := Analyze(Options{
		Bundle: bundle,
		Target: domain.ToolClaude,
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	goldenPath := filepath.Join(repoRoot, "testdata", "golden", "doctor", "codex_to_claude.json")
	testutil.AssertGoldenJSON(t, goldenPath, report)
}
