package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/catalog"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/inspect"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
	"github.com/jaeyoung0509/work-bridge/internal/tui"
)

func TestInstallSkillFromTUICopiesSkillTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")
	srcDir := filepath.Join(homeDir, ".codex", "skills", "frontend-design")

	mkdirAll(t, cwd)
	writeFile(t, filepath.Join(srcDir, "SKILL.md"), "# frontend-design")
	writeFile(t, filepath.Join(srcDir, "notes.txt"), "copy me")

	app := New(nil, nil)
	app.fs = fsx.OSFS{}
	app.getwd = func() (string, error) { return cwd, nil }
	app.home = func() (string, error) { return homeDir, nil }

	target := tui.SkillTarget{
		ID:    "project:" + cwd,
		Label: "project",
		Scope: "project",
		Path:  filepath.Join(cwd, "skills", "frontend-design", "SKILL.md"),
	}
	result, err := app.installSkillFromTUI(context.Background(), tui.SkillEntry{
		Name: "frontend-design",
		Path: filepath.Join(srcDir, "SKILL.md"),
	}, target)
	if err != nil {
		t.Fatalf("install skill failed: %v", err)
	}

	if got := result.InstalledPath; got != filepath.Join(cwd, "skills", "frontend-design", "SKILL.md") {
		t.Fatalf("unexpected installed path %q", got)
	}
	if result.TargetScope != "project" || result.TargetLabel != "project" {
		t.Fatalf("expected target metadata in result, got %#v", result)
	}

	for _, want := range []string{
		filepath.Join(cwd, "skills", "frontend-design", "SKILL.md"),
		filepath.Join(cwd, "skills", "frontend-design", "notes.txt"),
	} {
		if _, err := app.fs.Stat(want); err != nil {
			t.Fatalf("expected copied file %q: %v", want, err)
		}
	}
}

func TestProbeMCPFromTUIProbesRuntimeServer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "settings.json")
	writeFile(t, configPath, `{"mcpServers":{"helper":{"command":"`+os.Args[0]+`"}}}`)

	app := New(nil, nil)
	app.fs = fsx.OSFS{}
	app.look = func(binary string) (string, error) {
		if binary == "claude" {
			return "/opt/bin/claude", nil
		}
		return "", errors.New("not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := app.probeMCPFromTUI(ctx, tui.MCPEntry{
		Name:        "claude settings",
		Path:        configPath,
		Tool:        domain.ToolClaude,
		BinaryFound: true,
		Servers: []tui.MCPServerConfig{{
			Name:      "helper",
			Transport: "stdio",
			Command:   os.Args[0],
			Args:      []string{"-test.run=TestMCPHelperProcess", "--"},
			Env:       map[string]string{"GO_WANT_HELPER_PROCESS": "1"},
		}},
	})
	if err != nil {
		t.Fatalf("probe mcp failed: %v", err)
	}
	if !result.Reachable {
		t.Fatalf("expected runtime probe to be reachable: %#v", result)
	}
	if result.Mode != "runtime-stdio" {
		t.Fatalf("expected runtime-stdio mode, got %q", result.Mode)
	}
	if result.ResourceCount != 2 || result.TemplateCount != 1 || result.ToolCount != 3 || result.PromptCount != 1 {
		t.Fatalf("unexpected aggregate counts: %#v", result)
	}
	if result.ConnectedServers != 1 || len(result.ServerResults) != 1 {
		t.Fatalf("expected single connected server: %#v", result)
	}
	if !result.ServerResults[0].Reachable {
		t.Fatalf("expected helper server to be reachable: %#v", result.ServerResults[0])
	}
}

func TestSummarizeMCPConfigCountsDeclaredServers(t *testing.T) {
	t.Parallel()

	summary := summarizeMCPConfig("config.json", []byte(`{"mcpServers":{"github":{"command":"mcp-github"},"slack":{"command":"mcp-slack"}}}`))
	if summary.Format != "json" {
		t.Fatalf("expected json format, got %q", summary.Format)
	}
	if summary.Status != "parsed" {
		t.Fatalf("expected parsed status, got %q", summary.Status)
	}
	if len(summary.ServerNames) != 2 {
		t.Fatalf("expected two declared servers, got %d", len(summary.ServerNames))
	}
	if got := strings.Join(summary.ServerNames, ","); got != "github,slack" {
		t.Fatalf("unexpected server names %q", got)
	}
	if summary.ParseSource != "mcpServers" {
		t.Fatalf("expected parse source mcpServers, got %q", summary.ParseSource)
	}
	if len(summary.Servers) != 2 {
		t.Fatalf("expected two parsed server definitions, got %d", len(summary.Servers))
	}
	if len(summary.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", summary.Warnings)
	}
}

func TestResolveWorkspaceRootsPrefersConfiguredRoots(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")
	projectRoot := filepath.Join(homeDir, "Projects", "alpha")
	relativeRoot := filepath.Join(cwd, "workspace")
	homeRoot := filepath.Join(homeDir, "work")

	mkdirAll(t, projectRoot)
	mkdirAll(t, relativeRoot)
	mkdirAll(t, homeRoot)

	app := New(nil, nil)
	app.fs = fsx.OSFS{}
	app.config.WorkspaceRoots = []string{"~/work", "./workspace", "~/work"}

	roots := app.resolveWorkspaceRoots(cwd, homeDir, projectRoot)
	if len(roots) != 2 {
		t.Fatalf("expected 2 configured roots, got %#v", roots)
	}
	if roots[0] != relativeRoot && roots[1] != relativeRoot {
		t.Fatalf("expected relative root resolution, got %#v", roots)
	}
	if roots[0] != homeRoot && roots[1] != homeRoot {
		t.Fatalf("expected home root resolution, got %#v", roots)
	}
}

func TestEnrichProjectEntriesTracksSessionCountsByTool(t *testing.T) {
	t.Parallel()

	projects := []catalog.ProjectEntry{
		{Name: "repo", Root: "/workspace/repo", WorkspaceRoot: "/workspace"},
		{Name: "service", Root: "/workspace/repo/service", WorkspaceRoot: "/workspace"},
	}
	snapshot := tui.WorkspaceSnapshot{
		InspectByTool: map[domain.Tool]inspect.Report{
			domain.ToolCodex: {
				Tool: "codex",
				Sessions: []inspect.Session{
					{ID: "root", ProjectRoot: "/workspace/repo"},
					{ID: "nested", ProjectRoot: "/workspace/repo/service"},
				},
			},
			domain.ToolClaude: {
				Tool: "claude",
				Sessions: []inspect.Session{
					{ID: "nested-claude", StoragePath: "/workspace/repo/service/.claude/history.jsonl"},
				},
			},
		},
	}

	enriched := enrichProjectEntries(projects, snapshot)
	if len(enriched) != 2 {
		t.Fatalf("expected two projects, got %d", len(enriched))
	}

	if enriched[0].Name != "repo" || enriched[0].SessionByTool["codex"] != 1 {
		t.Fatalf("expected repo codex session count, got %#v", enriched[0])
	}
	if enriched[1].Name != "service" || enriched[1].SessionByTool["codex"] != 1 || enriched[1].SessionByTool["claude"] != 1 {
		t.Fatalf("expected nested project session counts by tool, got %#v", enriched[1])
	}
}

func TestEnrichSkillEntriesAssignsConflictState(t *testing.T) {
	t.Parallel()

	skills := enrichSkillEntries([]tui.SkillEntry{
		{Name: "frontend-design", Path: "/workspace/repo/skills/frontend-design/SKILL.md", Scope: "project", Content: "# one"},
		{Name: "frontend-design", Path: "/home/me/.codex/skills/frontend-design/SKILL.md", Scope: "user", Tool: domain.ToolCodex, Content: "# one"},
		{Name: "lint-helper", Path: "/home/me/.claude/skills/lint-helper/SKILL.md", Scope: "user", Tool: domain.ToolClaude, Content: "# two"},
	})

	if len(skills) != 3 {
		t.Fatalf("expected enriched skills, got %d", len(skills))
	}
	if skills[0].GroupKey == "" || skills[0].ContentHash == "" {
		t.Fatalf("expected grouping metadata, got %#v", skills[0])
	}

	var frontendProject tui.SkillEntry
	var lintUser tui.SkillEntry
	for _, skill := range skills {
		switch skill.Path {
		case "/workspace/repo/skills/frontend-design/SKILL.md":
			frontendProject = skill
		case "/home/me/.claude/skills/lint-helper/SKILL.md":
			lintUser = skill
		}
	}
	if frontendProject.ConflictState != "both-present" || frontendProject.VariantCount != 2 {
		t.Fatalf("expected both-present grouped skill, got %#v", frontendProject)
	}
	if lintUser.ConflictState != "only-in-user/global" || lintUser.VariantCount != 1 {
		t.Fatalf("expected user-only grouped skill, got %#v", lintUser)
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		message, err := readMCPMessage(reader)
		if err != nil {
			return
		}

		switch message.Method {
		case "initialize":
			_ = writeMCPMessage(os.Stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(message.ID),
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]any{
						"resources": map[string]any{},
						"tools":     map[string]any{},
						"prompts":   map[string]any{},
					},
					"serverInfo": map[string]any{
						"name":    "helper",
						"version": "test",
					},
				},
			})
		case "notifications/initialized":
		case "resources/list":
			_ = writeMCPMessage(os.Stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(message.ID),
				"result": map[string]any{
					"resources": []map[string]any{
						{"uri": "file:///one", "name": "one"},
						{"uri": "file:///two", "name": "two"},
					},
				},
			})
		case "resources/templates/list":
			_ = writeMCPMessage(os.Stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(message.ID),
				"result": map[string]any{
					"resourceTemplates": []map[string]any{
						{"uriTemplate": "file:///{name}", "name": "file"},
					},
				},
			})
		case "tools/list":
			_ = writeMCPMessage(os.Stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(message.ID),
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "one"},
						{"name": "two"},
						{"name": "three"},
					},
				},
			})
		case "prompts/list":
			_ = writeMCPMessage(os.Stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(message.ID),
				"result": map[string]any{
					"prompts": []map[string]any{
						{"name": "prompt-one"},
					},
				},
			})
		default:
			_ = writeMCPMessage(os.Stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(message.ID),
				"error": map[string]any{
					"code":    -32601,
					"message": "unsupported method",
				},
			})
		}
	}
}
