package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"sessionport/internal/platform/fsx"
)

func TestRunPrintsHelpByDefault(t *testing.T) {
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
	output := stdout.String()
	for _, want := range []string{"sessionport", "detect", "inspect", "doctor", "--config"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected help output to contain %q, got %q", want, output)
		}
	}
}

func TestRunReturnsUsageForUnknownCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"wat"}, &stdout, &stderr)

	if exitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, exitCode)
	}
	if !strings.Contains(stderr.String(), `unknown command "wat"`) {
		t.Fatalf("expected unknown command error, got %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout for unknown command, got %q", stdout.String())
	}
}

func TestDetectCommandRendersTextOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	projectRoot := filepath.Join(root, "repo")
	cwd := filepath.Join(projectRoot, "service")

	mkdirAll(t, filepath.Join(projectRoot, ".git"))
	mkdirAll(t, cwd)
	writeFile(t, filepath.Join(homeDir, ".codex", "config.toml"), "model = \"gpt-5\"")
	writeFile(t, filepath.Join(projectRoot, "AGENTS.md"), "# project")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := New(&stdout, &stderr)
	app.getwd = func() (string, error) { return cwd, nil }
	app.home = func() (string, error) { return homeDir, nil }
	app.look = func(binary string) (string, error) {
		if binary == "codex" {
			return "/opt/bin/codex", nil
		}
		return "", errors.New("not found")
	}

	exitCode := app.Run(context.Background(), []string{"detect"})

	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	for _, want := range []string{"Current directory:", "Project root:", "Codex", "/opt/bin/codex", filepath.Join(projectRoot, "AGENTS.md")} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected detect output to contain %q, got %q", want, stdout.String())
		}
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
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "sessionport") {
		t.Fatalf("expected version output, got %q", stdout.String())
	}
}

func TestDetectCommandRendersJSONOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".claude", "settings.json"), "{}")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := New(&stdout, &stderr)
	app.getwd = func() (string, error) { return cwd, nil }
	app.home = func() (string, error) { return homeDir, nil }
	app.look = func(string) (string, error) { return "", errors.New("not found") }

	exitCode := app.Run(context.Background(), []string{"--format", "json", "detect"})

	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	for _, want := range []string{`"project_root"`, `"tool": "claude"`, `"installed": true`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected json detect output to contain %q, got %q", want, stdout.String())
		}
	}
}

func TestDetectCommandLoadsDefaultConfigFileFromCwd(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(cwd, ".sessionport.json"), `{"format":"json"}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := New(&stdout, &stderr)
	app.getwd = func() (string, error) { return cwd, nil }
	app.home = func() (string, error) { return homeDir, nil }
	app.look = func(string) (string, error) { return "", errors.New("not found") }

	exitCode := app.Run(context.Background(), []string{"detect"})

	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"project_root"`) {
		t.Fatalf("expected config-driven json output, got %q", stdout.String())
	}
}

func TestPlaceholderCommandReturnsNotImplemented(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"inspect", "codex"}, &stdout, &stderr)

	if exitCode != ExitNotImplemented {
		t.Fatalf("expected exit code %d, got %d", ExitNotImplemented, exitCode)
	}
	if !strings.Contains(stderr.String(), "inspect command is scaffolded but not implemented yet") {
		t.Fatalf("expected placeholder message, got %q", stderr.String())
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := (fsx.OSFS{}).MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %q failed: %v", path, err)
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := (fsx.OSFS{}).WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %q failed: %v", path, err)
	}
}
