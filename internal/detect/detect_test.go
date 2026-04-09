package detect

import (
	"path/filepath"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
)

func TestRunFindsArtifactsAcrossHomeAndProject(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	projectRoot := filepath.Join(root, "repo")
	cwd := filepath.Join(projectRoot, "pkg", "nested")

	mkdirAll(t, filepath.Join(projectRoot, ".git"))
	mkdirAll(t, cwd)
	writeFile(t, filepath.Join(homeDir, ".codex", "config.toml"), "model = \"gpt-5\"")
	writeFile(t, filepath.Join(homeDir, ".gemini", "settings.json"), "{}")
	writeFile(t, filepath.Join(homeDir, ".claude", "settings.json"), "{}")
	writeFile(t, filepath.Join(projectRoot, "AGENTS.md"), "# codex")
	writeFile(t, filepath.Join(projectRoot, ".gemini", "settings.json"), "{}")
	writeFile(t, filepath.Join(projectRoot, "pkg", "GEMINI.md"), "# gemini")
	writeFile(t, filepath.Join(projectRoot, "pkg", ".claude", "CLAUDE.md"), "# claude")

	report, err := Run(Options{
		FS:      fsx.OSFS{},
		CWD:     cwd,
		HomeDir: homeDir,
		LookPath: func(binary string) (string, error) {
			if binary == "codex" {
				return "/opt/tools/codex", nil
			}
			return "", filepath.ErrBadPattern
		},
	})
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}

	if report.ProjectRoot != projectRoot {
		t.Fatalf("expected project root %q, got %q", projectRoot, report.ProjectRoot)
	}

	codex := findTool(t, report, "codex")
	if !codex.Installed {
		t.Fatal("expected codex to be detected")
	}
	if !codex.Binary.Found || codex.Binary.Path != "/opt/tools/codex" {
		t.Fatalf("expected codex binary to be detected, got %#v", codex.Binary)
	}
	if !hasFoundArtifact(codex, filepath.Join(projectRoot, "AGENTS.md")) {
		t.Fatalf("expected project AGENTS.md to be detected: %#v", codex.Artifacts)
	}

	gemini := findTool(t, report, "gemini")
	if !hasFoundArtifact(gemini, filepath.Join(projectRoot, ".gemini", "settings.json")) {
		t.Fatalf("expected project Gemini settings to be detected: %#v", gemini.Artifacts)
	}
	if !hasFoundArtifact(gemini, filepath.Join(projectRoot, "pkg", "GEMINI.md")) {
		t.Fatalf("expected ancestor GEMINI.md to be detected: %#v", gemini.Artifacts)
	}

	claude := findTool(t, report, "claude")
	if !hasFoundArtifact(claude, filepath.Join(projectRoot, "pkg", ".claude", "CLAUDE.md")) {
		t.Fatalf("expected nested Claude instruction to be detected: %#v", claude.Artifacts)
	}
}

func TestRunFallsBackToCwdWithoutGitRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "workspace")
	mkdirAll(t, cwd)

	report, err := Run(Options{
		FS:       fsx.OSFS{},
		CWD:      cwd,
		HomeDir:  homeDir,
		LookPath: func(string) (string, error) { return "", filepath.ErrBadPattern },
	})
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}

	if report.ProjectRoot != cwd {
		t.Fatalf("expected cwd fallback as project root, got %q", report.ProjectRoot)
	}
}

func findTool(t *testing.T, report Report, tool string) ToolReport {
	t.Helper()

	for _, candidate := range report.Tools {
		if candidate.Tool == tool {
			return candidate
		}
	}
	t.Fatalf("tool %q not found in report %#v", tool, report.Tools)
	return ToolReport{}
}

func hasFoundArtifact(tool ToolReport, path string) bool {
	for _, artifact := range tool.Artifacts {
		if artifact.Path == path && artifact.Found {
			return true
		}
	}
	return false
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
