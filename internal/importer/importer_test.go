package importer

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
)

func TestImportCodexLatest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".codex", "config.toml"), ""+
		"model = \"gpt-5\"\n"+
		"project_doc_fallback_filenames = [\"AGENTS.md\"]\n"+
		"auth_token = \"secret\"\n")
	writeFile(t, filepath.Join(cwd, "AGENTS.md"), "# repo instructions")
	writeFile(t, filepath.Join(homeDir, ".codex", "session_index.jsonl"),
		`{"id":"codex-session","thread_name":"codex task","updated_at":"2026-04-07T15:00:00Z"}`+"\n")
	writeFile(t, filepath.Join(homeDir, ".codex", "sessions", "2026", "04", "07", "rollout-2026-04-07T15-00-00-codex-session.jsonl"), ""+
		`{"timestamp":"2026-04-07T14:59:00Z","type":"session_meta","payload":{"id":"codex-session","timestamp":"2026-04-07T14:59:00Z","cwd":"/workspace/codex"}}`+"\n"+
		`{"timestamp":"2026-04-07T15:00:00Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"sed -n '1,10p' README.md\"}","call_id":"call_1"}}`+"\n"+
		`{"timestamp":"2026-04-07T15:00:01Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}}`+"\n")

	bundle, err := Import(Options{
		FS:         fsx.OSFS{},
		CWD:        cwd,
		HomeDir:    homeDir,
		Tool:       "codex",
		Session:    "latest",
		ImportedAt: "2026-04-07T16:00:00Z",
		LookPath: func(binary string) (string, error) {
			if binary == "codex" {
				return "/opt/bin/codex", nil
			}
			return "", errors.New("not found")
		},
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if bundle.SourceTool != "codex" || bundle.SourceSessionID != "codex-session" {
		t.Fatalf("unexpected bundle identity: %#v", bundle)
	}
	if bundle.ProjectRoot != "/workspace/codex" {
		t.Fatalf("expected project root from session_meta, got %#v", bundle.ProjectRoot)
	}
	if bundle.SettingsSnapshot.Included["model"] != "gpt-5" {
		t.Fatalf("expected model setting, got %#v", bundle.SettingsSnapshot.Included)
	}
	if len(bundle.SettingsSnapshot.ExcludedKeys) == 0 {
		t.Fatalf("expected excluded sensitive keys, got %#v", bundle.SettingsSnapshot)
	}
	if len(bundle.InstructionArtifacts) == 0 {
		t.Fatalf("expected AGENTS instruction artifact, got %#v", bundle.InstructionArtifacts)
	}
	if len(bundle.ToolEvents) != 1 || bundle.ToolEvents[0].Type != "tool_call" {
		t.Fatalf("expected tool event, got %#v", bundle.ToolEvents)
	}
	if bundle.TokenStats["total_tokens"] != 15 {
		t.Fatalf("expected token stats, got %#v", bundle.TokenStats)
	}
}

func TestImportGeminiExplicitSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".gemini", "settings.json"), `{"theme":"ansi","apiToken":"secret"}`)
	writeFile(t, filepath.Join(homeDir, ".gemini", "GEMINI.md"), "# global gemini")
	writeFile(t, filepath.Join(homeDir, ".gemini", "projects.json"), `{"projects":{"/workspace/gemini":"demo"}}`)
	writeFile(t, filepath.Join(homeDir, ".gemini", "tmp", "demo", "chats", "session-2026-03-31T20-40-demo.json"), `{
  "sessionId":"gemini-session",
  "startTime":"2026-03-31T20:40:44Z",
  "lastUpdated":"2026-03-31T20:40:56Z",
  "messages":[
    {"type":"user","timestamp":"2026-03-31T20:40:44Z","content":[{"text":"implement bounded worker pool"}]},
    {"type":"gemini","timestamp":"2026-03-31T20:40:56Z","content":"use worker goroutines and a result channel","tokens":{"input":10,"output":5,"total":15},"toolCalls":[{"id":"tool_1","name":"read_file","status":"success","timestamp":"2026-03-31T20:40:50Z","args":{"path":"docs.md"}}]}
  ]
}`)

	bundle, err := Import(Options{
		FS:         fsx.OSFS{},
		CWD:        cwd,
		HomeDir:    homeDir,
		Tool:       "gemini",
		Session:    "gemini-session",
		ImportedAt: "2026-04-07T16:00:00Z",
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if bundle.SourceTool != "gemini" || bundle.SourceSessionID != "gemini-session" {
		t.Fatalf("unexpected bundle identity: %#v", bundle)
	}
	if bundle.TaskTitle == "" || bundle.CurrentGoal == "" {
		t.Fatalf("expected task title/current goal, got %#v", bundle)
	}
	if bundle.ProjectRoot != "/workspace/gemini" {
		t.Fatalf("expected gemini project root, got %#v", bundle.ProjectRoot)
	}
	if bundle.SettingsSnapshot.Included["theme"] != "ansi" {
		t.Fatalf("expected theme setting, got %#v", bundle.SettingsSnapshot.Included)
	}
	if bundle.TokenStats["total"] != 15 {
		t.Fatalf("expected token aggregation, got %#v", bundle.TokenStats)
	}
	if len(bundle.ToolEvents) != 1 || bundle.ToolEvents[0].Summary == "" {
		t.Fatalf("expected gemini tool events, got %#v", bundle.ToolEvents)
	}
}

func TestImportClaudeLatest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".claude", "settings.json"), `{"model":"opus","apiKey":"secret"}`)
	writeFile(t, filepath.Join(cwd, "CLAUDE.md"), "# project claude")
	writeFile(t, filepath.Join(homeDir, ".claude", "history.jsonl"), ""+
		`{"display":"first prompt","timestamp":1767727725221,"project":"/workspace/claude","sessionId":"claude-session"}`+"\n"+
		`{"display":"latest prompt","timestamp":1767727760078,"project":"/workspace/claude","sessionId":"claude-session"}`+"\n")

	bundle, err := Import(Options{
		FS:         fsx.OSFS{},
		CWD:        cwd,
		HomeDir:    homeDir,
		Tool:       "claude",
		Session:    "latest",
		ImportedAt: "2026-04-07T16:00:00Z",
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if bundle.SourceTool != "claude" || bundle.SourceSessionID != "claude-session" {
		t.Fatalf("unexpected bundle identity: %#v", bundle)
	}
	if bundle.ProjectRoot != "/workspace/claude" {
		t.Fatalf("expected claude project root, got %#v", bundle.ProjectRoot)
	}
	if bundle.TaskTitle != "latest prompt" || bundle.CurrentGoal != "latest prompt" {
		t.Fatalf("expected latest display to drive title/goal, got %#v", bundle)
	}
	if bundle.SettingsSnapshot.Included["model"] != "opus" {
		t.Fatalf("expected model setting, got %#v", bundle.SettingsSnapshot.Included)
	}
	if len(bundle.SettingsSnapshot.ExcludedKeys) == 0 {
		t.Fatalf("expected excluded sensitive keys, got %#v", bundle.SettingsSnapshot)
	}
	if len(bundle.InstructionArtifacts) == 0 {
		t.Fatalf("expected CLAUDE instruction artifact, got %#v", bundle.InstructionArtifacts)
	}
	if len(bundle.Warnings) == 0 {
		t.Fatalf("expected partial import warning, got %#v", bundle.Warnings)
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
