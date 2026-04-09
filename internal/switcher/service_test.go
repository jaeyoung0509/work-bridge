package switcher

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/testutil"
)

func TestPreviewPlansAllTargets(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))
	seedSwitchAssets(t, fixture)
	service := newTestService(fixture)

	for _, target := range []domain.Tool{domain.ToolCodex, domain.ToolGemini, domain.ToolClaude, domain.ToolOpenCode} {
		t.Run(string(target), func(t *testing.T) {
			result, err := service.Preview(context.Background(), Request{
				From:          domain.ToolCodex,
				Session:       "latest",
				To:            target,
				ProjectRoot:   fixture.WorkspaceDir,
				IncludeSkills: true,
				IncludeMCP:    true,
			})
			if err != nil {
				t.Fatalf("Preview failed: %v", err)
			}
			if result.Plan.TargetTool != target {
				t.Fatalf("expected target %q, got %q", target, result.Plan.TargetTool)
			}
			if result.Plan.Status == domain.SwitchStateError {
				t.Fatalf("expected non-error plan, got %#v", result.Plan)
			}
			if !containsPath(result.Plan.PlannedFiles, filepath.Join(fixture.WorkspaceDir, ".work-bridge", string(target))) {
				t.Fatalf("expected managed root files in plan, got %#v", result.Plan.PlannedFiles)
			}
			switch target {
			case domain.ToolClaude:
				if !containsFile(result.Plan.Session.Files, filepath.Join(fixture.WorkspaceDir, "CLAUDE.md")) {
					t.Fatalf("expected CLAUDE.md in session files: %#v", result.Plan.Session.Files)
				}
			case domain.ToolGemini:
				if !containsFile(result.Plan.Session.Files, filepath.Join(fixture.WorkspaceDir, "GEMINI.md")) {
					t.Fatalf("expected GEMINI.md in session files: %#v", result.Plan.Session.Files)
				}
			default:
				if !containsFile(result.Plan.Session.Files, filepath.Join(fixture.WorkspaceDir, "AGENTS.md")) {
					t.Fatalf("expected AGENTS.md in session files: %#v", result.Plan.Session.Files)
				}
			}
		})
	}
}

func TestApplyWritesProjectNativeStateAndIsIdempotent(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))
	seedSwitchAssets(t, fixture)
	service := newTestService(fixture)

	req := Request{
		From:          domain.ToolCodex,
		Session:       "latest",
		To:            domain.ToolClaude,
		ProjectRoot:   fixture.WorkspaceDir,
		IncludeSkills: true,
		IncludeMCP:    true,
	}
	result, err := service.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if result.Report == nil {
		t.Fatalf("expected apply report")
	}
	for _, path := range []string{
		filepath.Join(fixture.WorkspaceDir, ".work-bridge", "claude", "CLAUDE.work-bridge.md"),
		filepath.Join(fixture.WorkspaceDir, ".work-bridge", "claude", "MEMORY_NOTE.md"),
		filepath.Join(fixture.WorkspaceDir, ".work-bridge", "claude", "STARTER_PROMPT.md"),
		filepath.Join(fixture.WorkspaceDir, ".work-bridge", "claude", "manifest.json"),
		filepath.Join(fixture.WorkspaceDir, ".work-bridge", "claude", "skills", "index.json"),
		filepath.Join(fixture.WorkspaceDir, ".work-bridge", "claude", "mcp.json"),
		filepath.Join(fixture.WorkspaceDir, "CLAUDE.md"),
		filepath.Join(fixture.WorkspaceDir, ".claude", "settings.local.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	claudeData, err := os.ReadFile(filepath.Join(fixture.WorkspaceDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md failed: %v", err)
	}
	if count := strings.Count(string(claudeData), managedBlockStart); count != 1 {
		t.Fatalf("expected one managed block, got %d in %q", count, string(claudeData))
	}

	configData, err := os.ReadFile(filepath.Join(fixture.WorkspaceDir, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("read claude settings failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(configData, &decoded); err != nil {
		t.Fatalf("parse claude settings failed: %v", err)
	}
	if _, ok := decoded["mcpServers"]; !ok {
		t.Fatalf("expected mcpServers patch in %q", string(configData))
	}

	second, err := service.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("second Apply failed: %v", err)
	}
	if second.Report == nil {
		t.Fatalf("expected second apply report")
	}
	if len(second.Report.BackupsCreated) != 0 {
		t.Fatalf("expected idempotent second apply with no backups, got %#v", second.Report.BackupsCreated)
	}
}

func TestApplyHonorsSessionOnlyAndCodexSkipsConfigPatch(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))
	seedSwitchAssets(t, fixture)
	service := newTestService(fixture)

	sessionOnly, err := service.Apply(context.Background(), Request{
		From:          domain.ToolCodex,
		Session:       "latest",
		To:            domain.ToolGemini,
		ProjectRoot:   fixture.WorkspaceDir,
		IncludeSkills: false,
		IncludeMCP:    false,
	})
	if err != nil {
		t.Fatalf("session-only apply setup failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.WorkspaceDir, ".work-bridge", "gemini", "skills", "index.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("did not expect session-only skill export, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.WorkspaceDir, ".work-bridge", "gemini", "mcp.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("did not expect session-only mcp export, got err=%v", err)
	}
	if sessionOnly.Report == nil {
		t.Fatalf("expected session-only report")
	}
	if sessionOnly.Report.Skills.Summary != "No skills selected" {
		t.Fatalf("expected session-only skill summary, got %q", sessionOnly.Report.Skills.Summary)
	}
	if sessionOnly.Report.MCP.Summary != "No MCP servers selected" {
		t.Fatalf("expected session-only mcp summary, got %q", sessionOnly.Report.MCP.Summary)
	}

	result, err := service.Apply(context.Background(), Request{
		From:          domain.ToolCodex,
		Session:       "latest",
		To:            domain.ToolCodex,
		ProjectRoot:   fixture.WorkspaceDir,
		IncludeSkills: true,
		IncludeMCP:    true,
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.WorkspaceDir, ".work-bridge", "codex", "mcp.json")); err != nil {
		t.Fatalf("expected codex managed MCP file: %v", err)
	}
	// Codex now supports a project-local .codex/config.toml for MCP servers.
	// When MCP servers are present, the config.toml should be written.
	if _, err := os.Stat(filepath.Join(fixture.WorkspaceDir, ".codex", "config.toml")); err != nil {
		t.Fatalf("expected codex project config.toml to be written with MCP servers: %v", err)
	}

	result, err = service.Apply(context.Background(), Request{
		From:          domain.ToolCodex,
		Session:       "latest",
		To:            domain.ToolGemini,
		ProjectRoot:   fixture.WorkspaceDir,
		IncludeSkills: true,
		IncludeMCP:    true,
	})
	if err != nil {
		t.Fatalf("gemini Apply failed: %v", err)
	}
	if result.Report == nil {
		t.Fatalf("expected report")
	}
}

func TestExportWritesTargetReadyTreeOutsideProject(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))
	seedSwitchAssets(t, fixture)
	service := newTestService(fixture)
	outRoot := filepath.Join(t.TempDir(), "claude-export")

	result, err := service.Export(context.Background(), Request{
		From:          domain.ToolCodex,
		Session:       "latest",
		To:            domain.ToolClaude,
		ProjectRoot:   fixture.WorkspaceDir,
		IncludeSkills: true,
		IncludeMCP:    true,
	}, outRoot)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if result.Report == nil {
		t.Fatalf("expected export report")
	}
	if result.Report.AppliedMode != "project" {
		t.Fatalf("expected project report, got %q", result.Report.AppliedMode)
	}
	for _, path := range []string{
		filepath.Join(outRoot, ".work-bridge", "claude", "CLAUDE.work-bridge.md"),
		filepath.Join(outRoot, ".work-bridge", "claude", "MEMORY_NOTE.md"),
		filepath.Join(outRoot, ".work-bridge", "claude", "STARTER_PROMPT.md"),
		filepath.Join(outRoot, ".work-bridge", "claude", "manifest.json"),
		filepath.Join(outRoot, ".work-bridge", "claude", "skills", "index.json"),
		filepath.Join(outRoot, ".work-bridge", "claude", "mcp.json"),
		filepath.Join(outRoot, "CLAUDE.md"),
		filepath.Join(outRoot, ".claude", "settings.local.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.WorkspaceDir, "CLAUDE.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("did not expect source project to be modified during export, err=%v", err)
	}
}

func newTestService(fixture testutil.Fixture) *Service {
	return New(Options{
		FS:      osFS{},
		CWD:     fixture.WorkspaceDir,
		HomeDir: fixture.HomeDir,
		LookPath: func(binary string) (string, error) {
			switch binary {
			case "codex", "gemini", "claude", "opencode":
				return "/opt/bin/" + binary, nil
			default:
				return "", errors.New("not found")
			}
		},
		Now: func() time.Time {
			return time.Date(2026, 4, 9, 17, 0, 0, 0, time.UTC)
		},
	})
}

func seedSwitchAssets(t *testing.T, fixture testutil.Fixture) {
	t.Helper()
	writeFile(t, filepath.Join(fixture.WorkspaceDir, "skills", "project-helper", "SKILL.md"), "# project-helper\n\nProject helper")
	writeFile(t, filepath.Join(fixture.HomeDir, ".claude", "skills", "user-helper", "SKILL.md"), "# user-helper\n\nUser helper")
	writeFile(t, filepath.Join(fixture.WorkspaceDir, ".gemini", "settings.json"), `{"mcpServers":{"github":{"command":"mcp-github"}}}`)
	writeFile(t, filepath.Join(fixture.HomeDir, ".claude", "settings.json"), `{"mcpServers":{"slack":{"command":"mcp-slack"}}}`)
	writeFile(t, filepath.Join(fixture.WorkspaceDir, ".claude", "settings.local.json"), `{"model":"opus"}`)
	writeFile(t, filepath.Join(fixture.WorkspaceDir, ".opencode", "opencode.jsonc"), `{"theme":"light"}`)
}

type osFS struct{}

func (osFS) ReadFile(name string) ([]byte, error)                      { return os.ReadFile(name) }
func (osFS) WriteFile(name string, data []byte, perm os.FileMode) error { return os.WriteFile(name, data, perm) }
func (osFS) Stat(name string) (os.FileInfo, error)                     { return os.Stat(name) }
func (osFS) ReadDir(name string) ([]os.DirEntry, error)                { return os.ReadDir(name) }
func (osFS) MkdirAll(path string, perm os.FileMode) error              { return os.MkdirAll(path, perm) }
func (osFS) Remove(name string) error                                  { return os.Remove(name) }


func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %q failed: %v", path, err)
	}
}

func containsFile(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsPath(values []domain.PlannedFileChange, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value.Path, prefix) {
			return true
		}
	}
	return false
}
