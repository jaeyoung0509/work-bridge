package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type Fixture struct {
	Root         string
	HomeDir      string
	WorkspaceDir string
}

func RepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	current := wd
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("repo root not found from %q", wd)
		}
		current = parent
	}
}

func StageFixture(t *testing.T, fixtureDir string) Fixture {
	t.Helper()

	root := t.TempDir()
	inputDir := filepath.Join(fixtureDir, "input")
	if err := copyTree(inputDir, root); err != nil {
		t.Fatalf("stage fixture %q failed: %v", fixtureDir, err)
	}

	return Fixture{
		Root:         root,
		HomeDir:      filepath.Join(root, "home"),
		WorkspaceDir: filepath.Join(root, "workspace"),
	}
}

func AssertGoldenJSON(t *testing.T, goldenPath string, got any) {
	t.Helper()

	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden payload failed: %v", err)
	}
	data = append(data, '\n')

	if os.Getenv("SESSIONPORT_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir golden dir failed: %v", err)
		}
		if err := os.WriteFile(goldenPath, data, 0o644); err != nil {
			t.Fatalf("write golden failed: %v", err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %q failed: %v", goldenPath, err)
	}

	if string(want) != string(data) {
		t.Fatalf("golden mismatch for %s\nwant:\n%s\ngot:\n%s", goldenPath, string(want), string(data))
	}
}

func copyTree(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
