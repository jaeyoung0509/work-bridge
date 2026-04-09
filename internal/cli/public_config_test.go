package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/testutil"
)

func TestExportUsesConfiguredOutputDefault(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))
	seedSwitchAssetsForCLI(t, fixture)
	defaultOut := filepath.Join(t.TempDir(), "configured-export")
	writeFile(t, filepath.Join(fixture.WorkspaceDir, ".work-bridge.json"), `{
  "output": {
    "export_dir": "`+filepath.ToSlash(defaultOut)+`"
  }
}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newFixtureApp(t, fixture)
	app.stdout = &stdout
	app.stderr = &stderr

	exitCode := app.Run(context.Background(), []string{
		"export",
		"--from", "codex",
		"--session", "latest",
		"--to", "gemini",
		"--project", fixture.WorkspaceDir,
	})
	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	if _, err := app.fs.Stat(filepath.Join(defaultOut, "GEMINI.md")); err != nil {
		t.Fatalf("expected configured export root to be used: %v", err)
	}
}

func TestFlagOverridesEnvFormatForInspect(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))

	t.Setenv("WORK_BRIDGE_FORMAT", "json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newFixtureApp(t, fixture)
	app.stdout = &stdout
	app.stderr = &stderr

	exitCode := app.Run(context.Background(), []string{"--format", "text", "inspect", "codex", "--limit", "1"})
	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Tool: codex") {
		t.Fatalf("expected text inspect output, got %q", stdout.String())
	}
	if strings.Contains(stdout.String(), `"tool": "codex"`) {
		t.Fatalf("expected text output, got %q", stdout.String())
	}
}
