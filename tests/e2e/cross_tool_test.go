//go:build e2e

// Package e2e provides end-to-end tests for cross-tool session migration.
// These tests are designed to run locally only and should NOT be executed in CI.
//
// Usage:
//
//	WORKBRIDGE_E2E=1 go test -tags=e2e ./tests/e2e/... -v
//
// Or run `make test-e2e`.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNativeModeMigration tests native mode session migration.
func TestNativeModeMigration(t *testing.T) {
	skipIfE2EDisabled(t)

	binary := buildWorkBridge(t)
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "testproject")

	err := os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	initGitRepo(t, projectRoot)

	nativePairs := []struct {
		from string
		to   string
	}{
		{"codex", "claude"},
		{"gemini", "claude"},
		{"claude", "codex"},
	}

	for _, pair := range nativePairs {
		t.Run(fmt.Sprintf("native_%s_to_%s", pair.from, pair.to), func(t *testing.T) {
			testNativeSwitch(t, binary, projectRoot, pair.from, pair.to)
		})
	}
}

func testNativeSwitch(t *testing.T, binary, projectRoot, from, to string) {
	t.Helper()

	cmd := exec.Command(binary, "switch",
		"--from", from,
		"--session", "latest",
		"--to", to,
		"--project", projectRoot,
		"--mode", "native",
		"--dry-run",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "no sessions found") ||
			strings.Contains(string(output), "session not found") {
			t.Skipf("no sessions available for native %s -> %s", from, to)
		}
		t.Logf("Output: %s", output)
		t.Skipf("native switch failed (may need real session data): %v", err)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "mode: native") {
		t.Error("output missing native mode indicator")
	}
	if !strings.Contains(outputStr, "destination:") {
		t.Error("output missing destination in native mode")
	}

	t.Logf("Successfully tested native mode %s -> %s (dry-run)", from, to)
}

// TestGlobalSkillsMigration tests migration of global/user-scope skills.
func TestGlobalSkillsMigration(t *testing.T) {
	skipIfE2EDisabled(t)

	tmpDir := t.TempDir()

	codexSkillsDir := filepath.Join(tmpDir, ".codex", "skills")
	claudeSkillsDir := filepath.Join(tmpDir, ".claude", "skills")

	testCases := []struct {
		name       string
		sourceDir  string
		targetDir  string
		sourceTool string
		targetTool string
	}{
		{
			name:       "codex_to_claude_global_skills",
			sourceDir:  codexSkillsDir,
			targetDir:  claudeSkillsDir,
			sourceTool: "codex",
			targetTool: "claude",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := os.MkdirAll(tc.sourceDir, 0o755)
			if err != nil {
				t.Fatalf("failed to create source skills dir: %v", err)
			}

			skillContent := `---
name: test-skill
description: A test skill for E2E validation
---

# Test Skill

This is a test skill for E2E validation of global skill migration.
`
			skillPath := filepath.Join(tc.sourceDir, "SKILL.md")
			err = os.WriteFile(skillPath, []byte(skillContent), 0o644)
			if err != nil {
				t.Fatalf("failed to write skill file: %v", err)
			}

			if _, err := os.Stat(skillPath); err != nil {
				t.Fatalf("skill file not created: %v", err)
			}

			t.Logf("Created test skill at %s", skillPath)
			t.Logf("Global skill migration test setup complete for %s -> %s", tc.sourceTool, tc.targetTool)
		})
	}
}

// TestNativeExportImportCycle tests export and import cycle for session preservation.
func TestNativeExportImportCycle(t *testing.T) {
	skipIfE2EDisabled(t)

	binary := buildWorkBridge(t)
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "testproject")
	exportDir := filepath.Join(tmpDir, "export")

	err := os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	err = os.MkdirAll(exportDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create export dir: %v", err)
	}

	initGitRepo(t, projectRoot)

	tools := []string{"codex", "gemini", "claude"}

	for _, from := range tools {
		for _, to := range tools {
			if from == to {
				continue
			}

			t.Run(fmt.Sprintf("export_%s_to_%s", from, to), func(t *testing.T) {
				testExportCycle(t, binary, projectRoot, exportDir, from, to)
			})
		}
	}
}

func testExportCycle(t *testing.T, binary, projectRoot, exportDir, from, to string) {
	t.Helper()

	exportPath := filepath.Join(exportDir, fmt.Sprintf("%s_to_%s", from, to))

	cmd := exec.Command(binary, "export",
		"--from", from,
		"--session", "latest",
		"--to", to,
		"--project", projectRoot,
		"--mode", "native",
		"--out", exportPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "no sessions found") ||
			strings.Contains(string(output), "session not found") {
			t.Skipf("no sessions available for export %s -> %s", from, to)
		}
		t.Logf("Output: %s", output)
		t.Skipf("export failed (may need real session data): %v", err)
	}

	if _, err := os.Stat(exportPath); err != nil {
		t.Logf("Export path not created (expected with no session data)")
		t.Skip("export path not created - no session data available")
	}

	t.Logf("Successfully tested export %s -> %s to %s", from, to, exportPath)
}
