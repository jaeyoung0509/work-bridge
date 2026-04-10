//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func skipIfE2EDisabled(t *testing.T) {
	t.Helper()

	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("skipping E2E tests in CI environment")
	}
	if os.Getenv("WORKBRIDGE_E2E") == "" {
		t.Skip("skipping E2E tests: set WORKBRIDGE_E2E=1 to run")
	}
}

func buildWorkBridge(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	binaryName := "work-bridge"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(tmpDir, binaryName)

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/work-bridge")
	cmd.Dir = getProjectRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build work-bridge: %v\n%s", err, output)
	}

	return binaryPath
}

func getProjectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (go.mod)")
		}
		dir = parent
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to initialize git repo: %v\n%s", err, output)
	}
}
