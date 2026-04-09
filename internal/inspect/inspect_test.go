package inspect

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
)

func TestRunCodexInventory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".codex", "session_index.jsonl"), strings.Join([]string{
		`{"id":"11111111-1111-1111-1111-111111111111","thread_name":"recent codex","updated_at":"2026-04-07T15:00:00Z"}`,
		`{"id":"00000000-0000-0000-0000-000000000000","thread_name":"older codex","updated_at":"2026-04-06T15:00:00Z"}`,
	}, "\n"))
	writeFile(t, filepath.Join(homeDir, ".codex", "config.toml"), "model = 'gpt-5'")
	writeFile(t, filepath.Join(homeDir, ".codex", "sessions", "2026", "04", "07", "rollout-2026-04-07T15-00-00-11111111-1111-1111-1111-111111111111.jsonl"),
		`{"type":"session_meta","payload":{"timestamp":"2026-04-07T14:59:00Z","cwd":"/workspace/codex"}}`+"\n")
	writeFile(t, filepath.Join(homeDir, ".codex", "sessions", "2026", "04", "06", "rollout-2026-04-06T15-00-00-00000000-0000-0000-0000-000000000000.jsonl"),
		`{"type":"session_meta","payload":{"timestamp":"2026-04-06T14:59:00Z","cwd":"/workspace/older"}}`+"\n")

	report, err := Run(Options{
		FS:      fsx.OSFS{},
		CWD:     cwd,
		HomeDir: homeDir,
		Tool:    "codex",
		Limit:   1,
		LookPath: func(binary string) (string, error) {
			if binary == "codex" {
				return "/opt/bin/codex", nil
			}
			return "", errors.New("not found")
		},
	})
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	if report.TotalSessions != 2 {
		t.Fatalf("expected total sessions 2, got %d", report.TotalSessions)
	}
	if len(report.Sessions) != 1 {
		t.Fatalf("expected limited sessions 1, got %d", len(report.Sessions))
	}
	if report.Sessions[0].ProjectRoot != "/workspace/codex" {
		t.Fatalf("expected codex cwd, got %#v", report.Sessions[0])
	}
}

func TestRunGeminiInventory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".gemini", "projects.json"), `{"projects":{"/workspace/gemini":"demo"}}`)
	writeFile(t, filepath.Join(homeDir, ".gemini", "settings.json"), `{}`)
	writeFile(t, filepath.Join(homeDir, ".gemini", "tmp", "demo", "chats", "session-2026-03-31T14-38-demo.json"), `{
  "sessionId":"gemini-session",
  "startTime":"2026-03-31T14:39:18.214Z",
  "lastUpdated":"2026-03-31T14:41:09.489Z",
  "messages":[
    {"type":"user","content":[{"text":"please inspect gemini session"}]},
    {"type":"gemini","content":"ok"}
  ]
}`)

	report, err := Run(Options{
		FS:       fsx.OSFS{},
		CWD:      cwd,
		HomeDir:  homeDir,
		Tool:     "gemini",
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	if report.TotalSessions != 1 {
		t.Fatalf("expected total sessions 1, got %d", report.TotalSessions)
	}
	if report.Sessions[0].Title != "please inspect gemini session" {
		t.Fatalf("expected title from first user message, got %#v", report.Sessions[0])
	}
	if report.Sessions[0].ProjectRoot != "/workspace/gemini" {
		t.Fatalf("expected project root from projects.json, got %#v", report.Sessions[0])
	}
}

func TestRunClaudeInventory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	mkdirAll(t, filepath.Join(cwd, ".git"))
	writeFile(t, filepath.Join(homeDir, ".claude", "history.jsonl"), strings.Join([]string{
		`{"display":"first prompt","timestamp":1767727725221,"project":"/workspace/claude","sessionId":"claude-session"}`,
		`{"display":"latest prompt","timestamp":1767727760078,"project":"/workspace/claude","sessionId":"claude-session"}`,
	}, "\n"))
	writeFile(t, filepath.Join(homeDir, ".claude", "settings.json"), `{}`)

	report, err := Run(Options{
		FS:       fsx.OSFS{},
		CWD:      cwd,
		HomeDir:  homeDir,
		Tool:     "claude",
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
	})
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}

	if report.TotalSessions != 1 {
		t.Fatalf("expected total sessions 1, got %d", report.TotalSessions)
	}
	if report.Sessions[0].Title != "latest prompt" {
		t.Fatalf("expected latest prompt title, got %#v", report.Sessions[0])
	}
	if report.Sessions[0].MessageCount != 2 {
		t.Fatalf("expected message count 2, got %#v", report.Sessions[0])
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
