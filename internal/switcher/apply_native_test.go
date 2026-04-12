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

	gotoml "github.com/pelletier/go-toml/v2"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/pathpatch"
	"github.com/jaeyoung0509/work-bridge/internal/testutil"
)

// ---------------------------------------------------------------------------
// Codex native patch: session_meta.cwd rewrite
// ---------------------------------------------------------------------------

func TestNativePatchCodex_CWDRewrittenInManagedJSONL(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))
	seedSwitchAssets(t, fixture)

	// Inject a synthetic JSONL into the managed root to simulate what would be
	// there if the session had originally been recorded on a different machine.
	srcPath := "/Users/source-machine/project"
	dstPath := fixture.WorkspaceDir

	// Plant a session_meta JSONL in the managed root area.
	managedRoot := filepath.Join(fixture.WorkspaceDir, ".work-bridge", "codex")
	if err := os.MkdirAll(managedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	syntheticJSONL := `{"type":"session_meta","payload":{"id":"test-session","cwd":"` + srcPath + `","timestamp":"2026-01-01T00:00:00Z"}}` + "\n" +
		`{"type":"response_item","payload":{"type":"message","content":"hello from ` + srcPath + `/main.go"}}` + "\n"
	jsonlPath := filepath.Join(managedRoot, "session.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(syntheticJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	// Prepare a service and payload that represents a cross-machine delta.
	svc := newTestService(fixture)
	adapter := &projectAdapter{
		target: domain.ToolCodex,
		fs:     osFS{},
		now:    func() time.Time { return time.Date(2026, 4, 9, 17, 0, 0, 0, time.UTC) },
	}

	bundle := domain.NewSessionBundle(domain.ToolCodex, srcPath)
	payload := domain.SwitchPayload{Bundle: bundle}
	plan := domain.SwitchPlan{
		TargetTool:  domain.ToolCodex,
		ProjectRoot: dstPath,
		ManagedRoot: managedRoot,
		Session: domain.SwitchComponentPlan{
			Files: []string{jsonlPath},
		},
	}

	warnings := adapter.applyNativePatches(payload, plan)
	// There should be no error warnings.
	for _, w := range warnings {
		if strings.HasPrefix(w, "codex cwd patch: cannot") {
			t.Errorf("unexpected codex patch error: %s", w)
		}
	}

	// Read back and verify cwd was rewritten.
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) < 1 {
		t.Fatal("expected at least one JSONL line")
	}
	var meta struct {
		Type    string `json:"type"`
		Payload struct {
			CWD string `json:"cwd"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &meta); err != nil {
		t.Fatalf("cannot parse first line: %v", err)
	}
	if meta.Payload.CWD != dstPath {
		t.Errorf("expected cwd %q, got %q", dstPath, meta.Payload.CWD)
	}

	// Verify tool_result path was also rewritten.
	var second map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("cannot parse second line: %v", err)
	}
	pl := second["payload"].(map[string]any)
	content, _ := pl["content"].(string)
	if strings.Contains(content, srcPath) {
		t.Errorf("source path %q still present in payload: %s", srcPath, content)
	}

	_ = svc // suppress unused warning
}

func TestNativePatchCodex_SameProjectRootIsNoop(t *testing.T) {
	projectRoot := t.TempDir()
	adapter := &projectAdapter{
		target: domain.ToolCodex,
		fs:     osFS{},
		now:    time.Now,
	}
	bundle := domain.NewSessionBundle(domain.ToolCodex, projectRoot)
	payload := domain.SwitchPayload{Bundle: bundle}
	plan := domain.SwitchPlan{
		ProjectRoot: projectRoot,
		ManagedRoot: filepath.Join(projectRoot, ".work-bridge", "codex"),
		Session:     domain.SwitchComponentPlan{},
	}
	warnings := adapter.applyNativePatches(payload, plan)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for same-root noop, got %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// Gemini native patch: .project_root file creation
// ---------------------------------------------------------------------------

func TestNativePatchGemini_ProjectRootFileCreated(t *testing.T) {
	projectRoot := t.TempDir()
	managedRoot := filepath.Join(projectRoot, ".work-bridge", "gemini")
	if err := os.MkdirAll(managedRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	adapter := &projectAdapter{
		target: domain.ToolGemini,
		fs:     osFS{},
		now:    time.Now,
	}

	bundle := domain.NewSessionBundle(domain.ToolGemini, "/Users/other/project")
	payload := domain.SwitchPayload{Bundle: bundle}
	plan := domain.SwitchPlan{
		TargetTool:  domain.ToolGemini,
		ProjectRoot: projectRoot,
		ManagedRoot: managedRoot,
		Session:     domain.SwitchComponentPlan{},
	}

	_ = adapter.applyNativePatches(payload, plan)

	// .project_root must have been created.
	prPath := filepath.Join(managedRoot, ".project_root")
	content, err := os.ReadFile(prPath)
	if err != nil {
		t.Fatalf(".project_root not created: %v", err)
	}
	if strings.TrimSpace(string(content)) != projectRoot {
		t.Errorf(".project_root contains %q, want %q", strings.TrimSpace(string(content)), projectRoot)
	}
}

// ---------------------------------------------------------------------------
// Gemini project patch: does not touch projects.json
// ---------------------------------------------------------------------------

func TestProjectPatchGemini_DoesNotTouchProjectsJSON(t *testing.T) {
	// Create a minimal home dir that mimics ~/.gemini/projects.json
	homeDir := t.TempDir()
	geminiDir := filepath.Join(homeDir, ".gemini")
	if err := os.MkdirAll(geminiDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-populate with an existing entry.
	initial := map[string]any{
		"projects": map[string]string{
			"/other/project": "other-project",
		},
	}
	initialBytes, _ := json.MarshalIndent(initial, "", "  ")
	projectsPath := filepath.Join(geminiDir, "projects.json")
	if err := os.WriteFile(projectsPath, append(initialBytes, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	// The workspace lives inside homeDir so the heuristic lookup finds projects.json
	workspaceDir := filepath.Join(homeDir, "myproject")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	managedRoot := filepath.Join(workspaceDir, ".work-bridge", "gemini")
	if err := os.MkdirAll(managedRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	adapter := &projectAdapter{
		target:  domain.ToolGemini,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}

	bundle := domain.NewSessionBundle(domain.ToolGemini, "/Users/other/original")
	payload := domain.SwitchPayload{Bundle: bundle}
	plan := domain.SwitchPlan{
		TargetTool:  domain.ToolGemini,
		ProjectRoot: workspaceDir,
		ManagedRoot: managedRoot,
		Session:     domain.SwitchComponentPlan{},
	}

	adapter.applyNativePatches(payload, plan)

	// Read back projects.json.
	data, err := os.ReadFile(projectsPath)
	if err != nil {
		t.Fatalf("projects.json not found: %v", err)
	}
	type pf struct {
		Projects map[string]string `json:"projects"`
	}
	var updated pf
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("cannot parse projects.json: %v", err)
	}
	if _, ok := updated.Projects[workspaceDir]; ok {
		t.Errorf("did not expect %q to be registered in projects.json, got %v", workspaceDir, updated.Projects)
	}
}

func TestApplyGlobalSkillsGeminiWritesSharedSkillBundle(t *testing.T) {
	homeDir := t.TempDir()
	adapter := &projectAdapter{
		target:  domain.ToolGemini,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}

	payload := domain.SwitchPayload{
		Skills: []domain.SkillPayload{testSkillPayload(t, filepath.Join(t.TempDir(), "skills"), "reviewer", "user", "", "---\nname: reviewer\ndescription: Review skill\n---\n\n# Reviewer\n\nCheck changes carefully.\n")},
	}
	report := domain.ApplyReport{
		Status: domain.SwitchStateApplied,
		Skills: domain.ApplyComponentResult{State: domain.SwitchStateApplied},
	}

	report, err := adapter.applyGlobalSkills(payload, report)
	if err != nil {
		t.Fatalf("applyGlobalSkills failed: %v", err)
	}

	targetPath := filepath.Join(homeDir, ".agents", "skills", "reviewer", "SKILL.md")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("expected Gemini shared skill bundle, got %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "# Reviewer") {
		t.Fatalf("expected imported skill content, got:\n%s", text)
	}
	if !containsFile(report.FilesUpdated, targetPath) {
		t.Fatalf("expected %s in FilesUpdated, got %v", targetPath, report.FilesUpdated)
	}
	if !containsFile(report.Skills.Files, targetPath) {
		t.Fatalf("expected %s in Skills.Files, got %v", targetPath, report.Skills.Files)
	}
}

func TestApplyGlobalSkillsSkipsExistingIdenticalSkillWithoutWarning(t *testing.T) {
	homeDir := t.TempDir()
	targetPath := filepath.Join(homeDir, ".claude", "skills", "reviewer", "SKILL.md")
	writeFile(t, targetPath, "# Reviewer\n\nCheck changes carefully.\n")

	adapter := &projectAdapter{
		target:  domain.ToolClaude,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}

	payload := domain.SwitchPayload{
		Skills: []domain.SkillPayload{testSkillPayload(t, filepath.Join(t.TempDir(), "skills"), "reviewer", "user", "", "# Reviewer\n\nCheck changes carefully.\n")},
	}
	report := domain.ApplyReport{
		Status: domain.SwitchStateApplied,
		Skills: domain.ApplyComponentResult{State: domain.SwitchStateApplied},
	}

	report, err := adapter.applyGlobalSkills(payload, report)
	if err != nil {
		t.Fatalf("applyGlobalSkills failed: %v", err)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("expected no warnings for identical existing skill, got %v", report.Warnings)
	}
	if containsFile(report.FilesUpdated, targetPath) {
		t.Fatalf("did not expect existing identical skill to be rewritten, got %v", report.FilesUpdated)
	}
}

func TestApplyGlobalSkillsOpenCodeWritesCanonicalSkillPath(t *testing.T) {
	homeDir := t.TempDir()
	adapter := &projectAdapter{
		target:  domain.ToolOpenCode,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}

	payload := domain.SwitchPayload{
		Skills: []domain.SkillPayload{testSkillPayload(t, filepath.Join(t.TempDir(), "skills"), "reviewer", "user", "", "# Reviewer\n\nCheck changes carefully.\n")},
	}
	report := domain.ApplyReport{
		Status: domain.SwitchStateApplied,
		Skills: domain.ApplyComponentResult{State: domain.SwitchStateApplied},
	}

	report, err := adapter.applyGlobalSkills(payload, report)
	if err != nil {
		t.Fatalf("applyGlobalSkills failed: %v", err)
	}

	targetPath := filepath.Join(homeDir, ".config", "opencode", "skills", "reviewer", "SKILL.md")
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("expected opencode global skill at %s: %v", targetPath, err)
	}
	if !containsFile(report.FilesUpdated, targetPath) {
		t.Fatalf("expected %s in FilesUpdated, got %v", targetPath, report.FilesUpdated)
	}
}

func TestApplyGlobalMCPIgnoresGlobalSourcesWithoutServers(t *testing.T) {
	adapter := &projectAdapter{
		target: domain.ToolCodex,
		fs:     osFS{},
		now:    time.Now,
	}

	payload := domain.SwitchPayload{
		MCP: domain.MCPPayload{
			Sources: []domain.MCPSource{{
				Path:          "/tmp/opencode.json",
				Scope:         "user",
				Status:        "broken",
				ParseWarnings: []string{"parse failed"},
			}},
		},
	}
	report := domain.ApplyReport{Status: domain.SwitchStateApplied}

	report, err := adapter.applyGlobalMCP(payload, report)
	if err != nil {
		t.Fatalf("applyGlobalMCP failed: %v", err)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("expected no warnings without parsed global MCP servers, got %v", report.Warnings)
	}
}

func TestApplyGlobalMCPCodexWritesMergedGlobalConfig(t *testing.T) {
	homeDir := t.TempDir()
	targetPath := filepath.Join(homeDir, ".codex", "config.toml")
	writeFile(t, targetPath, "model = \"gpt-5.4\"\n\n[mcp.servers.existing]\ncommand = \"mcp-existing\"\n")

	adapter := &projectAdapter{
		target:  domain.ToolCodex,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}

	payload := domain.SwitchPayload{
		MCP: domain.MCPPayload{
			Sources: []domain.MCPSource{{
				Path:  "/tmp/settings.json",
				Scope: "user",
				Servers: []domain.MCPServerConfig{{
					Name:    "github",
					Command: "mcp-github",
				}},
			}},
		},
	}
	report := domain.ApplyReport{
		Status: domain.SwitchStateApplied,
		MCP:    domain.ApplyComponentResult{State: domain.SwitchStateApplied},
	}

	report, err := adapter.applyGlobalMCP(payload, report)
	if err != nil {
		t.Fatalf("applyGlobalMCP failed: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read codex config failed: %v", err)
	}
	var decoded map[string]any
	if err := gotoml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("parse codex config failed: %v", err)
	}
	if decoded["model"] != "gpt-5.4" {
		t.Fatalf("expected model setting to be preserved, got %#v", decoded["model"])
	}
	if !containsFile(report.FilesUpdated, targetPath) {
		t.Fatalf("expected %s in FilesUpdated, got %v", targetPath, report.FilesUpdated)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", report.Warnings)
	}
}

func TestApplyGlobalMCPGeminiWritesMergedGlobalConfig(t *testing.T) {
	homeDir := t.TempDir()
	targetPath := filepath.Join(homeDir, ".gemini", "settings.json")
	writeFile(t, targetPath, `{"general":{"previewFeatures":true}}`)

	adapter := &projectAdapter{
		target:  domain.ToolGemini,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}

	payload := domain.SwitchPayload{
		MCP: domain.MCPPayload{
			Sources: []domain.MCPSource{{
				Path:  "/tmp/settings.json",
				Scope: "user",
				Servers: []domain.MCPServerConfig{{
					Name:    "github",
					Command: "mcp-github",
				}},
			}},
		},
	}

	report, err := adapter.applyGlobalMCP(payload, domain.ApplyReport{
		Status: domain.SwitchStateApplied,
		MCP:    domain.ApplyComponentResult{State: domain.SwitchStateApplied},
	})
	if err != nil {
		t.Fatalf("applyGlobalMCP failed: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read gemini config failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("parse gemini config failed: %v", err)
	}
	if _, ok := decoded["general"].(map[string]any); !ok {
		t.Fatalf("expected existing general section to be preserved, got %#v", decoded["general"])
	}
	servers, ok := decoded["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers block, got %#v", decoded["mcpServers"])
	}
	if _, ok := servers["github"].(map[string]any); !ok {
		t.Fatalf("expected github server entry, got %#v", servers["github"])
	}
	if !containsFile(report.FilesUpdated, targetPath) {
		t.Fatalf("expected %s in FilesUpdated, got %v", targetPath, report.FilesUpdated)
	}
}

func TestApplyGlobalMCPOpenCodeWritesMcpSection(t *testing.T) {
	homeDir := t.TempDir()
	targetPath := filepath.Join(homeDir, ".config", "opencode", "opencode.jsonc")
	writeFile(t, targetPath, "{\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"provider\": {\n    \"ollama\": {}\n  },\n}\n")

	adapter := &projectAdapter{
		target:  domain.ToolOpenCode,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}

	payload := domain.SwitchPayload{
		MCP: domain.MCPPayload{
			Sources: []domain.MCPSource{{
				Path:  "/tmp/opencode.json",
				Scope: "user",
				Servers: []domain.MCPServerConfig{{
					Name:    "github",
					Command: "mcp-github",
					Args:    []string{"stdio"},
				}},
			}},
		},
	}

	report, err := adapter.applyGlobalMCP(payload, domain.ApplyReport{
		Status: domain.SwitchStateApplied,
		MCP:    domain.ApplyComponentResult{State: domain.SwitchStateApplied},
	})
	if err != nil {
		t.Fatalf("applyGlobalMCP failed: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read opencode config failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("parse opencode config failed: %v", err)
	}
	if _, ok := decoded["provider"].(map[string]any); !ok {
		t.Fatalf("expected provider section to be preserved, got %#v", decoded["provider"])
	}
	if _, exists := decoded["mcpServers"]; exists {
		t.Fatalf("did not expect mcpServers key in opencode config: %#v", decoded)
	}
	servers, ok := decoded["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcp block, got %#v", decoded["mcp"])
	}
	github, ok := servers["github"].(map[string]any)
	if !ok {
		t.Fatalf("expected github server entry, got %#v", servers["github"])
	}
	if github["type"] != "local" {
		t.Fatalf("expected local MCP type, got %#v", github["type"])
	}
	if !containsFile(report.FilesUpdated, targetPath) {
		t.Fatalf("expected %s in FilesUpdated, got %v", targetPath, report.FilesUpdated)
	}
}

func TestApplyGlobalMCPSkipsConflictingExistingServerWithWarning(t *testing.T) {
	homeDir := t.TempDir()
	targetPath := filepath.Join(homeDir, ".gemini", "settings.json")
	writeFile(t, targetPath, `{"mcpServers":{"github":{"command":"mcp-existing"}}}`)

	adapter := &projectAdapter{
		target:  domain.ToolGemini,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}

	payload := domain.SwitchPayload{
		MCP: domain.MCPPayload{
			Sources: []domain.MCPSource{{
				Path:  "/tmp/settings.json",
				Scope: "user",
				Servers: []domain.MCPServerConfig{{
					Name:    "github",
					Command: "mcp-github",
				}},
			}},
		},
	}

	report, err := adapter.applyGlobalMCP(payload, domain.ApplyReport{
		Status: domain.SwitchStateApplied,
		MCP:    domain.ApplyComponentResult{State: domain.SwitchStateApplied},
	})
	if err != nil {
		t.Fatalf("applyGlobalMCP failed: %v", err)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "already exists in the target config") {
		t.Fatalf("expected conflict warning, got %v", report.Warnings)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read gemini config failed: %v", err)
	}
	if !strings.Contains(string(data), "mcp-existing") {
		t.Fatalf("expected existing MCP server to be preserved, got %s", string(data))
	}
}

func TestRenderTargetConfigJSONOpenCodeUsesMcpKey(t *testing.T) {
	homeDir := t.TempDir()
	targetPath := filepath.Join(homeDir, ".opencode", "opencode.jsonc")
	writeFile(t, targetPath, `{"$schema":"https://opencode.ai/config.json"}`)

	adapter := &projectAdapter{
		target:  domain.ToolOpenCode,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}

	content, warning, err := adapter.renderTargetConfigJSON(targetPath, domain.MCPPayload{
		Servers: map[string]domain.MCPServerConfig{
			"github": {Name: "github", Command: "mcp-github"},
		},
	})
	if err != nil {
		t.Fatalf("renderTargetConfigJSON failed: %v", err)
	}
	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		t.Fatalf("parse rendered config failed: %v", err)
	}
	if _, exists := decoded["mcpServers"]; exists {
		t.Fatalf("did not expect mcpServers key, got %#v", decoded)
	}
	if _, exists := decoded["mcp_servers"]; exists {
		t.Fatalf("did not expect mcp_servers key, got %#v", decoded)
	}
	if _, ok := decoded["mcp"].(map[string]any); !ok {
		t.Fatalf("expected mcp key, got %#v", decoded["mcp"])
	}
}

func TestApplyPlanSkipsManagedNativePatchesForNativeMode(t *testing.T) {
	projectRoot := t.TempDir()
	homeDir := t.TempDir()
	adapter := &projectAdapter{
		target:  domain.ToolGemini,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}

	bundle := domain.NewSessionBundle(domain.ToolClaude, "/Users/source/project")
	bundle.SourceSessionID = "session-1"
	payload := domain.SwitchPayload{Bundle: bundle}

	plan, err := adapter.previewNativeGemini(payload, projectRoot, "")
	if err != nil {
		t.Fatalf("previewNativeGemini failed: %v", err)
	}
	report, err := adapter.applyPlan(payload, plan)
	if err != nil {
		t.Fatalf("applyPlan failed: %v", err)
	}
	for _, warning := range report.Warnings {
		if strings.Contains(warning, "projects.json") {
			t.Fatalf("did not expect managed Gemini patch warning in native mode, got %v", report.Warnings)
		}
	}
}

// ---------------------------------------------------------------------------
// Claude native registration: sessions-index.json removal
// ---------------------------------------------------------------------------

func TestApplyNativeRegistrationsClaude_SessionsIndexRemoved(t *testing.T) {
	homeDir := t.TempDir()
	projectRoot := filepath.Join(homeDir, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a fake sessions-index.json in the expected location.
	encodedDir := pathpatch.ClaudeProjectDirName(projectRoot)
	claudeDir := filepath.Join(homeDir, ".claude")
	indexDir := filepath.Join(claudeDir, "projects", encodedDir)
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(indexDir, "sessions-index.json")
	if err := os.WriteFile(indexPath, []byte(`{"sessions":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	managedRoot := filepath.Join(projectRoot, ".work-bridge", "claude")

	adapter := &projectAdapter{
		target:  domain.ToolClaude,
		fs:      osFS{},
		now:     time.Now,
		homeDir: homeDir,
	}
	bundle := domain.NewSessionBundle(domain.ToolClaude, projectRoot)
	payload := domain.SwitchPayload{Bundle: bundle}
	plan := domain.SwitchPlan{
		TargetTool:  domain.ToolClaude,
		ProjectRoot: projectRoot,
		ManagedRoot: managedRoot,
		Session:     domain.SwitchComponentPlan{},
	}

	_ = payload
	adapter.applyNativeRegistrations(plan)

	if _, err := os.Stat(indexPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected sessions-index.json to be removed, err=%v", err)
	}
}

// ---------------------------------------------------------------------------
// OpenCode project patch: does not emit advisory warning
// ---------------------------------------------------------------------------

func TestProjectPatchOpenCode_DoesNotEmitAdvisoryWarning(t *testing.T) {
	projectRoot := t.TempDir()
	adapter := &projectAdapter{
		target: domain.ToolOpenCode,
		fs:     osFS{},
		now:    time.Now,
	}
	bundle := domain.NewSessionBundle(domain.ToolOpenCode, "/other/project")
	payload := domain.SwitchPayload{Bundle: bundle}
	plan := domain.SwitchPlan{
		TargetTool:  domain.ToolOpenCode,
		ProjectRoot: projectRoot,
		ManagedRoot: filepath.Join(projectRoot, ".work-bridge", "opencode"),
		Session:     domain.SwitchComponentPlan{},
	}

	warnings := adapter.applyNativePatches(payload, plan)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "opencode") && strings.Contains(w, "session import") {
			found = true
			break
		}
	}
	if found {
		t.Errorf("did not expect opencode advisory warning, got %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// Codex TOML MCP config generation
// ---------------------------------------------------------------------------

func TestApplyCodexTOMLMCPConfig(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))
	seedSwitchAssets(t, fixture)
	svc := newTestService(fixture)

	result, err := svc.Apply(context.Background(), Request{
		From:          domain.ToolCodex,
		Session:       "latest",
		To:            domain.ToolCodex,
		ProjectRoot:   fixture.WorkspaceDir,
		IncludeSkills: false,
		IncludeMCP:    true,
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if result.Report == nil {
		t.Fatal("expected apply report")
	}

	configPath := filepath.Join(fixture.WorkspaceDir, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected .codex/config.toml, got: %v", err)
	}
	// Verify it contains the MCP [mcp] section.
	if !strings.Contains(string(data), "[mcp") {
		t.Errorf("expected TOML [mcp] section, got:\n%s", string(data))
	}
}

// ---------------------------------------------------------------------------
// Manifest idempotency guard
// ---------------------------------------------------------------------------

func TestNativePatchDoesNotModifyManifestJSON(t *testing.T) {
	projectRoot := t.TempDir()
	managedRoot := filepath.Join(projectRoot, ".work-bridge", "claude")
	if err := os.MkdirAll(managedRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	srcRoot := "/Users/source/project"
	// Write a manifest.json that contains srcRoot paths.
	manifestContent := `{"source_tool":"claude","project_root":"` + srcRoot + `"}` + "\n"
	manifestPath := filepath.Join(managedRoot, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o644); err != nil {
		t.Fatal(err)
	}
	originalModTime := func() time.Time {
		info, _ := os.Stat(manifestPath)
		return info.ModTime()
	}()

	adapter := &projectAdapter{
		target: domain.ToolClaude,
		fs:     osFS{},
		now:    time.Now,
	}
	bundle := domain.NewSessionBundle(domain.ToolClaude, srcRoot)
	payload := domain.SwitchPayload{Bundle: bundle}
	plan := domain.SwitchPlan{
		TargetTool:  domain.ToolClaude,
		ProjectRoot: projectRoot,
		ManagedRoot: managedRoot,
		Session: domain.SwitchComponentPlan{
			Files: []string{manifestPath},
		},
	}

	adapter.applyNativePatches(payload, plan)

	info, _ := os.Stat(manifestPath)
	if !info.ModTime().Equal(originalModTime) {
		t.Error("manifest.json was modified by native patch; expected it to be excluded")
	}
}

func testSkillPayload(t *testing.T, root string, name string, scope string, tool domain.Tool, body string) domain.SkillPayload {
	t.Helper()
	skillRoot := filepath.Join(root, name)
	entryPath := filepath.Join(skillRoot, "SKILL.md")
	writeFile(t, entryPath, body)
	return domain.SkillPayload{
		Name:        name,
		Description: "test skill",
		RootPath:    skillRoot,
		EntryPath:   entryPath,
		Files:       []string{entryPath},
		Scope:       scope,
		Tool:        tool,
	}
}
