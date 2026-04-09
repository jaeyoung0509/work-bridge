package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectCommandUsesConfiguredToolPaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")
	customCodexDir := filepath.Join(root, "alt-codex")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(cwd, ".work-bridge.json"), `{
  "format": "json",
  "paths": {
    "codex": "`+filepath.ToSlash(customCodexDir)+`"
  }
}`)
	writeFile(t, filepath.Join(customCodexDir, "config.toml"), `model = "gpt-5"`)

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
	if !strings.Contains(stdout.String(), filepath.ToSlash(filepath.Join(customCodexDir, "config.toml"))) {
		t.Fatalf("expected detect output to use configured codex path, got %q", stdout.String())
	}
}

func TestImportCommandUsesConfiguredOutputDefault(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")
	defaultOut := filepath.Join(root, "artifacts", "bundle.json")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(cwd, ".work-bridge.json"), `{
  "output": {
    "import_bundle_path": "`+filepath.ToSlash(defaultOut)+`"
  }
}`)
	writeFile(t, filepath.Join(homeDir, ".codex", "session_index.jsonl"),
		`{"id":"codex-session","thread_name":"config output","updated_at":"2026-04-07T15:00:00Z"}`+"\n")
	writeFile(t, filepath.Join(homeDir, ".codex", "sessions", "2026", "04", "07", "rollout-2026-04-07T15-00-00-codex-session.jsonl"),
		`{"timestamp":"2026-04-07T14:59:00Z","type":"session_meta","payload":{"id":"codex-session","timestamp":"2026-04-07T14:59:00Z","cwd":"/workspace/codex"}}`+"\n")

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

	exitCode := app.Run(context.Background(), []string{"import", "--from", "codex", "--session", "latest"})
	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}

	data, err := app.fs.ReadFile(defaultOut)
	if err != nil {
		t.Fatalf("expected default import output file to be written: %v", err)
	}
	if !strings.Contains(string(data), `"source_tool": "codex"`) {
		t.Fatalf("expected bundle content, got %q", string(data))
	}
}

func TestFlagOverridesEnvFormat(t *testing.T) {
	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".claude", "settings.json"), "{}")
	t.Setenv("WORK_BRIDGE_FORMAT", "json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := New(&stdout, &stderr)
	app.getwd = func() (string, error) { return cwd, nil }
	app.home = func() (string, error) { return homeDir, nil }
	app.look = func(string) (string, error) { return "", errors.New("not found") }

	exitCode := app.Run(context.Background(), []string{"--format", "text", "detect"})
	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Current directory:") {
		t.Fatalf("expected flag-selected text output, got %q", stdout.String())
	}
	if strings.Contains(stdout.String(), `"project_root"`) {
		t.Fatalf("expected text output, got %q", stdout.String())
	}
}
