package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
)

type fakeBackend struct {
	workspace   switcher.Workspace
	loadErr     error
	previewResp switcher.Result
	previewErr  error
	applyResp   switcher.Result
	applyErr    error
	exportResp  switcher.Result
	exportErr   error

	previewCalls []switcher.Request
	applyCalls   []switcher.Request
	exportCalls  []switcher.Request
	exportDirs   []string
}

func (f *fakeBackend) LoadWorkspace(context.Context) (switcher.Workspace, error) {
	return f.workspace, f.loadErr
}

func (f *fakeBackend) Preview(_ context.Context, req switcher.Request) (switcher.Result, error) {
	f.previewCalls = append(f.previewCalls, req)
	return f.previewResp, f.previewErr
}

func (f *fakeBackend) Apply(_ context.Context, req switcher.Request) (switcher.Result, error) {
	f.applyCalls = append(f.applyCalls, req)
	return f.applyResp, f.applyErr
}

func (f *fakeBackend) Export(_ context.Context, req switcher.Request, outDir string) (switcher.Result, error) {
	f.exportCalls = append(f.exportCalls, req)
	f.exportDirs = append(f.exportDirs, outDir)
	return f.exportResp, f.exportErr
}

func TestMainModelSessionEnterTransitionsToTargetStep(t *testing.T) {
	t.Parallel()

	model, _ := bootstrapModel(t, newFakeBackend())
	model = runKey(t, model, specialKey(tea.KeyEnter))

	if model.state != StateSelectTarget {
		t.Fatalf("expected target step, got %v", model.state)
	}
	if model.selectedSession == nil || model.selectedSession.ID != "session-1" {
		t.Fatalf("expected selected session to be set, got %#v", model.selectedSession)
	}
	if model.target != domain.ToolGemini {
		t.Fatalf("expected default target to skip source tool, got %s", model.target)
	}
}

func TestMainModelBuildsPreviewRequestFromSelections(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	model, backend := bootstrapModel(t, backend)
	model = runKey(t, model, specialKey(tea.KeyEnter))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyEnter))

	if len(backend.previewCalls) != 1 {
		t.Fatalf("expected one preview call, got %d", len(backend.previewCalls))
	}

	req := backend.previewCalls[0]
	if req.From != domain.ToolCodex || req.Session != "session-1" {
		t.Fatalf("unexpected source selection in request: %#v", req)
	}
	if req.To != domain.ToolGemini {
		t.Fatalf("expected default target gemini, got %s", req.To)
	}
	if req.Mode != domain.SwitchModeProject {
		t.Fatalf("expected project mode, got %s", req.Mode)
	}
	if !req.IncludeSkills || !req.IncludeMCP {
		t.Fatalf("expected full preview by default, got %#v", req)
	}
	if req.ProjectRoot != "/repo/project" {
		t.Fatalf("expected workspace project root, got %q", req.ProjectRoot)
	}
	if model.state != StatePreview {
		t.Fatalf("expected preview state, got %v", model.state)
	}
	if model.lastPreview == nil {
		t.Fatalf("expected preview result to be stored")
	}
}

func TestMainModelSessionOnlyDisablesSkillsAndMCP(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	model, backend := bootstrapModel(t, backend)
	model = runKey(t, model, specialKey(tea.KeyEnter))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyEnter))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyEnter))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyEnter))

	if len(backend.previewCalls) != 1 {
		t.Fatalf("expected one preview call, got %d", len(backend.previewCalls))
	}

	req := backend.previewCalls[0]
	if req.IncludeSkills || req.IncludeMCP {
		t.Fatalf("expected session-only preview to disable skills and mcp, got %#v", req)
	}
}

func TestMainModelExportUsesConfiguredDefaultThenFallbackPath(t *testing.T) {
	t.Parallel()

	t.Run("configured default", func(t *testing.T) {
		model, _ := previewReadyModel(t, newFakeBackend(), Options{
			ProjectRoot:      "/repo/project",
			DefaultExportDir: "/tmp/configured-out",
		})

		model = runKey(t, model, runeKey("e"))
		if model.state != StateConfirm {
			t.Fatalf("expected confirm state, got %v", model.state)
		}
		if model.confirmInput != "/tmp/configured-out" {
			t.Fatalf("expected configured export dir, got %q", model.confirmInput)
		}
	})

	t.Run("project fallback", func(t *testing.T) {
		model, _ := previewReadyModel(t, newFakeBackend(), Options{
			ProjectRoot: "/repo/project",
		})

		model = runKey(t, model, runeKey("e"))
		want := filepath.Join("/repo/project", ".work-bridge", "exports", string(domain.ToolGemini))
		if model.confirmInput != want {
			t.Fatalf("expected fallback export path %q, got %q", want, model.confirmInput)
		}
	})
}

func TestMainModelApplyAndExportRequireConfirmBeforeRunning(t *testing.T) {
	t.Parallel()

	t.Run("apply waits for confirm", func(t *testing.T) {
		backend := newFakeBackend()
		model, backend := previewReadyModel(t, backend, Options{ProjectRoot: "/repo/project"})

		model = runKey(t, model, runeKey("a"))
		if len(backend.applyCalls) != 0 {
			t.Fatalf("did not expect apply before confirmation")
		}

		model = runKey(t, model, specialKey(tea.KeyEnter))
		if len(backend.applyCalls) != 1 {
			t.Fatalf("expected one apply after confirmation, got %d", len(backend.applyCalls))
		}
		if model.state != StateResult {
			t.Fatalf("expected result state, got %v", model.state)
		}
	})

	t.Run("export waits for confirm", func(t *testing.T) {
		backend := newFakeBackend()
		model, backend := previewReadyModel(t, backend, Options{ProjectRoot: "/repo/project"})

		model = runKey(t, model, runeKey("e"))
		if len(backend.exportCalls) != 0 {
			t.Fatalf("did not expect export before confirmation")
		}

		model = runKey(t, model, specialKey(tea.KeyEnter))
		if len(backend.exportCalls) != 1 {
			t.Fatalf("expected one export after confirmation, got %d", len(backend.exportCalls))
		}
		if backend.exportDirs[0] != filepath.Join("/repo/project", ".work-bridge", "exports", string(domain.ToolGemini)) {
			t.Fatalf("unexpected export directory: %q", backend.exportDirs[0])
		}
		if model.state != StateResult {
			t.Fatalf("expected result state, got %v", model.state)
		}
	})
}

func TestMainModelPreviewAndResultViewsShowWarningsAndErrors(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	model, backend := previewReadyModel(t, backend, Options{ProjectRoot: "/repo/project"})

	previewView := model.View().Content
	if !strings.Contains(previewView, "portability warning") {
		t.Fatalf("expected preview warning in view, got %q", previewView)
	}
	if !strings.Contains(previewView, "PARTIAL") {
		t.Fatalf("expected preview status in view, got %q", previewView)
	}

	model = runKey(t, model, runeKey("a"))
	model = runKey(t, model, specialKey(tea.KeyEnter))
	resultView := model.View().Content
	if !strings.Contains(resultView, "report warning") {
		t.Fatalf("expected result warning in view, got %q", resultView)
	}
	if !strings.Contains(resultView, "updated files: 2") {
		t.Fatalf("expected result summary in view, got %q", resultView)
	}
	if len(backend.applyCalls) != 1 {
		t.Fatalf("expected apply to execute once, got %d", len(backend.applyCalls))
	}
}

func TestMainModelAllowsSameToolTargetSelection(t *testing.T) {
	t.Parallel()

	backend := newFakeBackend()
	model, backend := bootstrapModel(t, backend)
	model = runKey(t, model, specialKey(tea.KeyEnter))
	model = runKey(t, model, specialKey(tea.KeyLeft))
	if model.target != domain.ToolCodex {
		t.Fatalf("expected same-tool target selection to be allowed, got %s", model.target)
	}
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyEnter))

	if got := backend.previewCalls[0].To; got != domain.ToolCodex {
		t.Fatalf("expected preview request to keep same-tool target, got %s", got)
	}
}

func bootstrapModel(t *testing.T, backend *fakeBackend) (MainModel, *fakeBackend) {
	t.Helper()

	model := NewMainModel(context.Background(), backend, Options{ProjectRoot: "/repo/project"})
	initMsg := runCmd(t, model.Init())
	updated, cmd := model.Update(initMsg)
	model = updated.(MainModel)
	if cmd != nil {
		t.Fatalf("did not expect follow-up command after workspace load")
	}
	return model, backend
}

func previewReadyModel(t *testing.T, backend *fakeBackend, opts Options) (MainModel, *fakeBackend) {
	t.Helper()

	model := NewMainModel(context.Background(), backend, opts)
	model = processCmd(t, model, model.Init())
	model = runKey(t, model, specialKey(tea.KeyEnter))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyDown))
	model = runKey(t, model, specialKey(tea.KeyEnter))
	return model, backend
}

func processCmd(t *testing.T, model MainModel, cmd tea.Cmd) MainModel {
	t.Helper()
	if cmd == nil {
		return model
	}
	msg := runCmd(t, cmd)
	updated, followup := model.Update(msg)
	model = updated.(MainModel)
	if followup != nil {
		t.Fatalf("unexpected nested follow-up command")
	}
	return model
}

func runKey(t *testing.T, model MainModel, msg tea.KeyPressMsg) MainModel {
	t.Helper()
	updated, cmd := model.Update(msg)
	model = updated.(MainModel)
	return processCmd(t, model, cmd)
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected command")
	}
	return cmd()
}

func specialKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func runeKey(text string) tea.KeyPressMsg {
	runes := []rune(text)
	return tea.KeyPressMsg{Text: text, Code: runes[0]}
}

func newFakeBackend() *fakeBackend {
	preview := switcher.Result{
		Payload: domain.SwitchPayload{
			Bundle: domain.SessionBundle{
				SourceTool:      domain.ToolCodex,
				SourceSessionID: "session-1",
			},
		},
		Plan: domain.SwitchPlan{
			TargetTool:      domain.ToolGemini,
			ProjectRoot:     "/repo/project",
			DestinationRoot: "/repo/project",
			Status:          domain.SwitchStatePartial,
			Session: domain.SwitchComponentPlan{
				State:   domain.SwitchStateReady,
				Summary: "2 managed session artifacts",
			},
			Skills: domain.SwitchComponentPlan{
				State:   domain.SwitchStateReady,
				Summary: "1 skill bundle",
			},
			MCP: domain.SwitchComponentPlan{
				State:   domain.SwitchStatePartial,
				Summary: "1 managed MCP server",
			},
			PlannedFiles: []domain.PlannedFileChange{
				{Section: "session", Path: "/repo/project/CLAUDE.md", Action: "update"},
				{Section: "mcp", Path: "/repo/project/.work-bridge/gemini/mcp.json", Action: "write"},
			},
			Warnings: []string{"portability warning"},
		},
	}
	action := switcher.Result{
		Plan: preview.Plan,
		Report: &domain.ApplyReport{
			Status:          domain.SwitchStatePartial,
			DestinationRoot: "/repo/project",
			FilesUpdated:    []string{"/repo/project/CLAUDE.md", "/repo/project/.work-bridge/gemini/mcp.json"},
			Session: domain.ApplyComponentResult{
				State:   domain.SwitchStateApplied,
				Summary: "2 session files applied",
			},
			Skills: domain.ApplyComponentResult{
				State:   domain.SwitchStateApplied,
				Summary: "1 skill files applied",
			},
			MCP: domain.ApplyComponentResult{
				State:   domain.SwitchStatePartial,
				Summary: "1 MCP files applied",
			},
			Warnings: []string{"report warning"},
		},
	}
	return &fakeBackend{
		workspace: switcher.Workspace{
			ProjectRoot: "/repo/project",
			Sessions: []switcher.WorkspaceItem{
				{
					Tool:        domain.ToolCodex,
					ID:          "session-1",
					Title:       "Codex task",
					ProjectRoot: "/repo/project",
				},
			},
		},
		previewResp: preview,
		applyResp:   action,
		exportResp:  action,
	}
}
