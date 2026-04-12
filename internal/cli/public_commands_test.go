package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/testutil"
)

func TestRunPrintsPublicHelpByDefault(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), nil, &stdout, &stderr)
	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	for _, want := range []string{"inspect", "switch", "export", "version"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected help to contain %q, got %q", want, stdout.String())
		}
	}
	for _, legacy := range []string{"detect", "import", "doctor", "pack", "unpack"} {
		if strings.Contains(stdout.String(), "\n  "+legacy+" ") {
			t.Fatalf("did not expect help to contain legacy command %q, got %q", legacy, stdout.String())
		}
	}
}

func TestRunReturnsUsageForUnknownLegacyCommand(t *testing.T) {
	t.Parallel()

	for _, legacy := range []string{"detect", "import", "doctor", "pack", "unpack"} {
		t.Run(legacy, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := Run(context.Background(), []string{legacy}, &stdout, &stderr)
			if exitCode != ExitUsage {
				t.Fatalf("expected exit code %d, got %d", ExitUsage, exitCode)
			}
			if !strings.Contains(stderr.String(), `unknown command "`+legacy+`"`) {
				t.Fatalf("expected unknown command error, got %q", stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("expected empty stdout, got %q", stdout.String())
			}
		})
	}
}

func TestVersionCommandPrintsVersion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"version"}, &stdout, &stderr)
	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, exitCode)
	}
	if !strings.Contains(stdout.String(), "work-bridge") {
		t.Fatalf("expected version output, got %q", stdout.String())
	}
}

func TestInspectCommandRejectsUnknownTool(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"inspect", "wat"}, &stdout, &stderr)
	if exitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, exitCode)
	}
	if !strings.Contains(stderr.String(), `unsupported tool "wat"`) {
		t.Fatalf("expected unsupported tool error, got %q", stderr.String())
	}
}

func TestSwitchCommandRequiresProject(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(context.Background(), []string{"switch", "--from", "codex", "--to", "claude"}, &stdout, &stderr)
	if exitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, exitCode)
	}
	if !strings.Contains(stderr.String(), "--project is required") {
		t.Fatalf("expected missing project error, got %q", stderr.String())
	}
}

func TestSwitchCommandRendersJSONDryRun(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))
	seedSwitchAssetsForCLI(t, fixture)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newFixtureApp(t, fixture)
	app.stdout = &stdout
	app.stderr = &stderr

	exitCode := app.Run(context.Background(), []string{
		"--format", "json",
		"switch",
		"--from", "codex",
		"--session", "latest",
		"--to", "claude",
		"--project", fixture.WorkspaceDir,
		"--dry-run",
	})
	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	for _, want := range []string{`"target_tool": "claude"`, `"status":`, `"managed_root":`, `"planned_files":`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected switch preview output to contain %q, got %q", want, stdout.String())
		}
	}
}

func TestExportCommandWritesTargetReadyFiles(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))
	seedSwitchAssetsForCLI(t, fixture)
	outRoot := filepath.Join(t.TempDir(), "claude-export")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := newFixtureApp(t, fixture)
	app.stdout = &stdout
	app.stderr = &stderr

	exitCode := app.Run(context.Background(), []string{
		"export",
		"--from", "codex",
		"--session", "latest",
		"--to", "claude",
		"--project", fixture.WorkspaceDir,
		"--out", outRoot,
	})
	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	for _, path := range []string{
		filepath.Join(outRoot, ".work-bridge", "claude", "manifest.json"),
		filepath.Join(outRoot, ".work-bridge", "claude", "mcp.json"),
		filepath.Join(outRoot, ".claude", "skills", "project-helper", "SKILL.md"),
		filepath.Join(outRoot, "CLAUDE.md"),
		filepath.Join(outRoot, ".claude", "settings.local.json"),
	} {
		if _, err := app.fs.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
	if _, err := app.fs.Stat(filepath.Join(fixture.WorkspaceDir, "CLAUDE.md")); err == nil {
		t.Fatalf("did not expect source project to be modified by export")
	}
}
