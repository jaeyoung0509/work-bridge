package exporter

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"sessionport/internal/doctor"
	"sessionport/internal/domain"
	"sessionport/internal/importer"
	"sessionport/internal/platform/fsx"
	"sessionport/internal/testutil"
)

func TestExporterFixturesMatchGolden(t *testing.T) {
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

	cases := []struct {
		name   string
		target domain.Tool
		golden string
	}{
		{name: "codex", target: domain.ToolCodex, golden: "codex_bundle_to_codex.json"},
		{name: "gemini", target: domain.ToolGemini, golden: "codex_bundle_to_gemini.json"},
		{name: "claude", target: domain.ToolClaude, golden: "codex_bundle_to_claude.json"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, err := doctor.Analyze(doctor.Options{
				Bundle: bundle,
				Target: tc.target,
			})
			if err != nil {
				t.Fatalf("Analyze failed: %v", err)
			}

			outDir := filepath.Join(t.TempDir(), "out")
			if _, err := Export(Options{
				FS:     fsx.OSFS{},
				Bundle: bundle,
				Report: report,
				OutDir: outDir,
			}); err != nil {
				t.Fatalf("Export failed: %v", err)
			}

			got := normalizeSnapshot(testutil.SnapshotDir(t, outDir), fixture, outDir)
			goldenPath := filepath.Join(repoRoot, "testdata", "golden", "exporter", tc.golden)
			testutil.AssertGoldenJSON(t, goldenPath, got)
		})
	}
}

func normalizeSnapshot(files map[string]string, fixture testutil.Fixture, outDir string) map[string]string {
	normalized := make(map[string]string, len(files))
	fixtureRoot := filepath.ToSlash(filepath.Clean(fixture.Root))
	outputRoot := filepath.ToSlash(filepath.Clean(outDir))

	for path, content := range files {
		value := strings.ReplaceAll(content, fixtureRoot, "$FIXTURE")
		value = strings.ReplaceAll(value, outputRoot, "$OUT")
		normalized[path] = filepath.ToSlash(value)
	}

	return normalized
}
