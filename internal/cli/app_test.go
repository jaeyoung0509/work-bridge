package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestInspectCommandRendersTextOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".codex", "session_index.jsonl"), `{"id":"11111111-1111-1111-1111-111111111111","thread_name":"inspect me","updated_at":"2026-04-07T15:00:00Z"}`)
	writeFile(t, filepath.Join(homeDir, ".codex", "sessions", "2026", "04", "07", "rollout-2026-04-07T15-00-00-11111111-1111-1111-1111-111111111111.jsonl"),
		`{"type":"session_meta","payload":{"timestamp":"2026-04-07T14:59:00Z","cwd":"/workspace/codex"}}`+"\n")

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

	exitCode := app.Run(context.Background(), []string{"inspect", "codex"})

	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	for _, want := range []string{"Tool: codex", "inspect me", "/workspace/codex"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected inspect output to contain %q, got %q", want, stdout.String())
		}
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

func TestImportCommandRendersBundleJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".codex", "config.toml"), "model = \"gpt-5\"")
	writeFile(t, filepath.Join(homeDir, ".codex", "session_index.jsonl"),
		`{"id":"codex-session","thread_name":"import codex","updated_at":"2026-04-07T15:00:00Z"}`+"\n")
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
	app.clock = fixedClock{value: time.Date(2026, 4, 7, 16, 0, 0, 0, time.UTC)}

	exitCode := app.Run(context.Background(), []string{"import", "--from", "codex", "--session", "latest"})

	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	for _, want := range []string{`"source_tool": "codex"`, `"source_session_id": "codex-session"`, `"project_root": "/workspace/codex"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected import output to contain %q, got %q", want, stdout.String())
		}
	}
}

func TestImportCommandReturnsSessionNotFound(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".codex", "session_index.jsonl"), "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := New(&stdout, &stderr)
	app.getwd = func() (string, error) { return cwd, nil }
	app.home = func() (string, error) { return homeDir, nil }
	app.look = func(string) (string, error) { return "", errors.New("not found") }
	app.clock = fixedClock{value: time.Date(2026, 4, 7, 16, 0, 0, 0, time.UTC)}

	exitCode := app.Run(context.Background(), []string{"import", "--from", "codex", "--session", "latest"})

	if exitCode != ExitSessionNotFound {
		t.Fatalf("expected exit code %d, got %d", ExitSessionNotFound, exitCode)
	}
	if !strings.Contains(stderr.String(), `codex session "latest" was not found`) {
		t.Fatalf("expected session not found error, got %q", stderr.String())
	}
}

func TestImportClaudeCommandRendersBundleJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(cwd, "CLAUDE.md"), "# project claude")
	writeFile(t, filepath.Join(homeDir, ".claude", "settings.json"), `{"model":"opus"}`)
	writeFile(t, filepath.Join(homeDir, ".claude", "history.jsonl"),
		`{"display":"latest prompt","timestamp":1767727760078,"project":"/workspace/claude","sessionId":"claude-session"}`+"\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := New(&stdout, &stderr)
	app.getwd = func() (string, error) { return cwd, nil }
	app.home = func() (string, error) { return homeDir, nil }
	app.look = func(string) (string, error) { return "", errors.New("not found") }
	app.clock = fixedClock{value: time.Date(2026, 4, 7, 16, 0, 0, 0, time.UTC)}

	exitCode := app.Run(context.Background(), []string{"import", "--from", "claude", "--session", "latest"})

	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	for _, want := range []string{`"source_tool": "claude"`, `"source_session_id": "claude-session"`, `"project_root": "/workspace/claude"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected import output to contain %q, got %q", want, stdout.String())
		}
	}
}

func TestDoctorCommandRendersJSONOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".codex", "config.toml"), "model = \"gpt-5\"\nauth_token = \"secret\"")
	writeFile(t, filepath.Join(cwd, "AGENTS.md"), "# project instructions")
	writeFile(t, filepath.Join(homeDir, ".codex", "session_index.jsonl"),
		`{"id":"codex-session","thread_name":"doctor codex","updated_at":"2026-04-07T15:00:00Z"}`+"\n")
	writeFile(t, filepath.Join(homeDir, ".codex", "sessions", "2026", "04", "07", "rollout-2026-04-07T15-00-00-codex-session.jsonl"), ""+
		`{"timestamp":"2026-04-07T14:59:00Z","type":"session_meta","payload":{"id":"codex-session","timestamp":"2026-04-07T14:59:00Z","cwd":"/workspace/codex"}}`+"\n"+
		`{"timestamp":"2026-04-07T15:00:00Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"sed -n '1,10p' README.md\"}","call_id":"call_1"}}`+"\n")

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
	app.clock = fixedClock{value: time.Date(2026, 4, 7, 16, 0, 0, 0, time.UTC)}

	exitCode := app.Run(context.Background(), []string{"--format", "json", "doctor", "--from", "codex", "--session", "latest", "--target", "claude"})

	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d (stderr=%q)", ExitOK, exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	for _, want := range []string{`"source_tool": "codex"`, `"target_tool": "claude"`, `"generated_artifacts"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("expected doctor output to contain %q, got %q", want, stdout.String())
		}
	}
}

func TestDoctorCommandRejectsUnknownTarget(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"doctor", "--from", "codex", "--target", "wat"}, &stdout, &stderr)

	if exitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, exitCode)
	}
	if !strings.Contains(stderr.String(), `unsupported target tool "wat"`) {
		t.Fatalf("expected unsupported target error, got %q", stderr.String())
	}
}

type fixedClock struct {
	value time.Time
}

func (f fixedClock) Now() time.Time {
	return f.value
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
