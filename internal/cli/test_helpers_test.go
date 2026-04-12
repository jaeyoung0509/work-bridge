package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
	"github.com/jaeyoung0509/work-bridge/internal/testutil"
)

type fixedClock struct {
	value time.Time
}

func (f fixedClock) Now() time.Time {
	return f.value
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %q failed: %v", path, err)
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %q failed: %v", path, err)
	}
}

func newFixtureApp(t *testing.T, fixture testutil.Fixture) *App {
	t.Helper()
	app := New(nil, nil)
	app.fs = fsx.OSFS{}
	app.getwd = func() (string, error) { return fixture.WorkspaceDir, nil }
	app.home = func() (string, error) { return fixture.HomeDir, nil }
	app.look = func(binary string) (string, error) {
		switch binary {
		case "codex", "gemini", "claude", "opencode":
			return "/opt/bin/" + binary, nil
		default:
			return "", errors.New("not found")
		}
	}
	app.clock = fixedClock{value: time.Date(2026, 4, 9, 17, 0, 0, 0, time.UTC)}
	return app
}

func seedSwitchAssetsForCLI(t *testing.T, fixture testutil.Fixture) {
	t.Helper()
	writeFile(t, filepath.Join(fixture.WorkspaceDir, ".agents", "skills", "project-helper", "SKILL.md"), "# project-helper\n\nProject helper")
	writeFile(t, filepath.Join(fixture.HomeDir, ".claude", "skills", "user-helper", "SKILL.md"), "# user-helper\n\nUser helper")
	writeFile(t, filepath.Join(fixture.WorkspaceDir, ".gemini", "settings.json"), `{"mcpServers":{"github":{"command":"mcp-github"}}}`)
	writeFile(t, filepath.Join(fixture.HomeDir, ".claude", "settings.json"), `{"mcpServers":{"slack":{"command":"mcp-slack"}}}`)
	writeFile(t, filepath.Join(fixture.WorkspaceDir, ".claude", "settings.local.json"), `{"model":"opus"}`)
	writeFile(t, filepath.Join(fixture.WorkspaceDir, ".opencode", "opencode.jsonc"), `{"theme":"light"}`)
}
