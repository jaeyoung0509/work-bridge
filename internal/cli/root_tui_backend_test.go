package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
		Path:  filepath.Join(cwd, ".agents", "skills", "frontend-design", "SKILL.md"),
	}
	result, err := app.installSkillFromTUI(context.Background(), tui.SkillEntry{
		Name: "frontend-design",
		Path: filepath.Join(srcDir, "SKILL.md"),
	}, target)
	if err != nil {
		t.Fatalf("install skill failed: %v", err)
	}

	if got := result.InstalledPath; got != filepath.Join(cwd, ".agents", "skills", "frontend-design", "SKILL.md") {
		t.Fatalf("unexpected installed path %q", got)
	}
	if result.TargetScope != "project" || result.TargetLabel != "project" {
		t.Fatalf("expected target metadata in result, got %#v", result)
	}

	for _, want := range []string{
		filepath.Join(cwd, ".agents", "skills", "frontend-design", "SKILL.md"),
		filepath.Join(cwd, ".agents", "skills", "frontend-design", "notes.txt"),
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

func TestProbeMCPFromTUIProbesHTTPServer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "settings.json")
	writeFile(t, configPath, `{"mcpServers":{"github":{"url":"http://example.test"}}}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := payload["method"].(string)
		if method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mcpTestResponse(payload["id"], method))
	}))
	defer server.Close()

	app := New(nil, nil)
	app.fs = fsx.OSFS{}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := app.probeMCPFromTUI(ctx, tui.MCPEntry{
		Name: "github",
		Path: configPath,
		Tool: domain.ToolClaude,
		Servers: []tui.MCPServerConfig{{
			Name:      "github",
			Transport: "http",
			URL:       server.URL,
		}},
	})
	if err != nil {
		t.Fatalf("probe mcp http failed: %v", err)
	}
	if !result.Reachable {
		t.Fatalf("expected http probe to be reachable: %#v", result)
	}
	if result.Mode != "runtime-http" {
		t.Fatalf("expected runtime-http mode, got %q", result.Mode)
	}
	if result.ResourceCount != 2 || result.TemplateCount != 1 || result.ToolCount != 3 || result.PromptCount != 1 {
		t.Fatalf("unexpected aggregate counts: %#v", result)
	}
}

func TestProbeMCPFromTUIProbesLegacySSEServer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "settings.json")
	writeFile(t, configPath, `{"mcpServers":{"github":{"transport":"sse","url":"http://example.test/events"}}}`)

	responses := make(chan map[string]any, 16)
	mux := http.NewServeMux()
	var server *httptest.Server

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatalf("response writer does not support flushing")
		}
		fmt.Fprintf(w, "event: endpoint\ndata: %s/messages\n\n", server.URL)
		flusher.Flush()
		for {
			select {
			case payload := <-responses:
				body, err := json.Marshal(payload)
				if err != nil {
					t.Fatalf("marshal sse payload: %v", err)
				}
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", string(body))
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode sse request: %v", err)
		}
		method, _ := payload["method"].(string)
		if method != "notifications/initialized" {
			responses <- mcpTestResponse(payload["id"], method)
		}
		w.WriteHeader(http.StatusAccepted)
	})
	server = httptest.NewServer(mux)
	defer server.Close()

	app := New(nil, nil)
	app.fs = fsx.OSFS{}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := app.probeMCPFromTUI(ctx, tui.MCPEntry{
		Name: "github",
		Path: configPath,
		Tool: domain.ToolClaude,
		Servers: []tui.MCPServerConfig{{
			Name:      "github",
			Transport: "sse",
			URL:       server.URL + "/events",
		}},
	})
	if err != nil {
		t.Fatalf("probe mcp sse failed: %v", err)
	}
	if !result.Reachable {
		t.Fatalf("expected sse probe to be reachable: %#v", result)
	}
	if result.Mode != "runtime-sse" {
		t.Fatalf("expected runtime-sse mode, got %q", result.Mode)
	}
	if result.ResourceCount != 2 || result.TemplateCount != 1 || result.ToolCount != 3 || result.PromptCount != 1 {
		t.Fatalf("unexpected aggregate counts: %#v", result)
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

func TestBuildLogicalMCPEntriesMergesDeclarationsByServer(t *testing.T) {
	t.Parallel()

	entries := buildLogicalMCPEntries([]mcpConfigProfile{
		{
			Name:        "project claude settings",
			Path:        "/workspace/repo/.claude/settings.json",
			Source:      "project",
			Tool:        domain.ToolClaude,
			BinaryFound: true,
			BinaryPath:  "/opt/bin/claude",
			Summary: mcpConfigSummary{
				Status: "parsed",
				Servers: []tui.MCPServerConfig{{
					Name:      "github",
					Transport: "stdio",
					Command:   "mcp-github-project",
				}},
			},
		},
		{
			Name:        "global claude settings",
			Path:        "/home/me/.claude/settings.json",
			Source:      "user",
			Tool:        domain.ToolClaude,
			BinaryFound: true,
			BinaryPath:  "/opt/bin/claude",
			Summary: mcpConfigSummary{
				Status: "parsed",
				Servers: []tui.MCPServerConfig{{
					Name:      "github",
					Transport: "stdio",
					Command:   "mcp-github-user",
				}},
			},
		},
	})

	if len(entries) != 1 {
		t.Fatalf("expected one logical mcp entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Kind != "server" || entry.Name != "github" {
		t.Fatalf("expected logical github server entry, got %#v", entry)
	}
	if entry.Scope != "project" {
		t.Fatalf("expected project declaration to win by default, got %#v", entry)
	}
	if len(entry.Declarations) != 2 {
		t.Fatalf("expected merged declarations, got %#v", entry)
	}
	if len(entry.HiddenScopes) != 1 {
		t.Fatalf("expected one hidden scope, got %#v", entry.HiddenScopes)
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
		{Name: "frontend-design", Path: "/workspace/repo/.agents/skills/frontend-design/SKILL.md", Scope: "project", Content: "# one"},
		{Name: "frontend-design", Path: "/home/me/.agents/skills/frontend-design/SKILL.md", Scope: "user", Tool: domain.ToolCodex, Content: "# one"},
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
		case "/workspace/repo/.agents/skills/frontend-design/SKILL.md":
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

func mcpTestResponse(id any, method string) map[string]any {
	response := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
	}
	switch method {
	case "initialize":
		response["result"] = map[string]any{
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
		}
	case "resources/list":
		response["result"] = map[string]any{
			"resources": []map[string]any{
				{"uri": "file:///one", "name": "one"},
				{"uri": "file:///two", "name": "two"},
			},
		}
	case "resources/templates/list":
		response["result"] = map[string]any{
			"resourceTemplates": []map[string]any{
				{"uriTemplate": "file:///{name}", "name": "file"},
			},
		}
	case "tools/list":
		response["result"] = map[string]any{
			"tools": []map[string]any{
				{"name": "one"},
				{"name": "two"},
				{"name": "three"},
			},
		}
	case "prompts/list":
		response["result"] = map[string]any{
			"prompts": []map[string]any{
				{"name": "prompt-one"},
			},
		}
	default:
		response["error"] = map[string]any{
			"code":    -32601,
			"message": "unsupported method",
		}
	}
	return response
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
