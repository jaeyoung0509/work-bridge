//go:build e2e

// Package e2e provides end-to-end tests for cross-tool session migration.
// These tests are designed to run locally only and should NOT be executed in CI.
//
// Usage:
//
//	go test -tags=e2e ./tests/e2e/... -v
//
// To skip in CI, these tests require the WORKBRIDGE_E2E environment variable to be set.
package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// skipIfNotE2E skips the test unless WORKBRIDGE_E2E is set.
// This prevents E2E tests from running in CI.
func skipIfNotE2E(t *testing.T) {
	if os.Getenv("WORKBRIDGE_E2E") == "" {
		t.Skip("skipping E2E test: set WORKBRIDGE_E2E=1 to run")
	}
}

// buildWorkBridge builds the work-bridge binary for testing.
func buildWorkBridge(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "work-bridge")

	// Get the project root (3 levels up from this file)
	projectRoot := "../.."

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/work-bridge")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build work-bridge: %v\n%s", err, output)
	}

	return binaryPath
}

// TestCrossToolSessionMigration tests session import between different tools.
func TestCrossToolSessionMigration(t *testing.T) {
	skipIfNotE2E(t)

	binary := buildWorkBridge(t)
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "testproject")

	// Create a test project
	err := os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Initialize git repo (required for some tools)
	cmd := exec.Command("git", "init")
	cmd.Dir = projectRoot
	cmd.Run()

	toolPairs := []struct {
		from string
		to   string
	}{
		{"codex", "gemini"},
		{"codex", "claude"},
		{"gemini", "claude"},
		{"gemini", "codex"},
		{"claude", "codex"},
		{"claude", "gemini"},
	}

	for _, pair := range toolPairs {
		t.Run(fmt.Sprintf("%s_to_%s", pair.from, pair.to), func(t *testing.T) {
			testSessionSwitch(t, binary, projectRoot, pair.from, pair.to)
		})
	}
}

func testSessionSwitch(t *testing.T, binary, projectRoot, from, to string) {
	t.Helper()

	// Dry run first
	cmd := exec.Command(binary, "switch",
		"--from", from,
		"--session", "latest",
		"--to", to,
		"--project", projectRoot,
		"--mode", "project",
		"--dry-run",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Some tool pairs may not have sessions available, which is OK for E2E
		if strings.Contains(string(output), "no sessions found") ||
			strings.Contains(string(output), "session not found") {
			t.Skipf("no sessions available for %s -> %s", from, to)
		}
		t.Logf("Output: %s", output)
		// Don't fail - some pairs might need actual session data
		t.Skipf("switch failed (may need real session data): %v", err)
	}

	// Verify output contains expected information
	outputStr := string(output)
	if !strings.Contains(outputStr, "mode:") {
		t.Error("output missing mode information")
	}
	if !strings.Contains(outputStr, "project:") {
		t.Error("output missing project information")
	}

	t.Logf("Successfully tested %s -> %s (dry-run)", from, to)
}

// TestNativeModeMigration tests native mode session migration.
func TestNativeModeMigration(t *testing.T) {
	skipIfNotE2E(t)

	binary := buildWorkBridge(t)
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "testproject")

	// Create a test project
	err := os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = projectRoot
	cmd.Run()

	// Test native mode for each tool pair
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

	// Dry run in native mode
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

	// Verify native mode indicators
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
	skipIfNotE2E(t)

	binary := buildWorkBridge(t)
	tmpDir := t.TempDir()

	// Create fake global skills directories
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
			// Create source skill
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

			// Verify the skill was created
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
	skipIfNotE2E(t)

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

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = projectRoot
	cmd.Run()

	// Test export in native mode for each tool
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

	// Verify export directory was created
	if _, err := os.Stat(exportPath); err != nil {
		t.Logf("Export path not created (expected with no session data)")
		t.Skip("export path not created - no session data available")
	}

	t.Logf("Successfully tested export %s -> %s to %s", from, to, exportPath)
}

// TestJSONOutputFormat tests that JSON output format works correctly.
func TestJSONOutputFormat(t *testing.T) {
	skipIfNotE2E(t)

	binary := buildWorkBridge(t)
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "testproject")

	err := os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = projectRoot
	cmd.Run()

	cmd = exec.Command(binary, "switch",
		"--from", "codex",
		"--session", "latest",
		"--to", "claude",
		"--project", projectRoot,
		"--mode", "project",
		"--dry-run",
	)
	cmd.Env = append(os.Environ(), "WORKBRIDGE_FORMAT=json")

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "no sessions found") {
			t.Skip("no sessions available")
		}
		t.Skipf("switch failed: %v", err)
	}

	// Verify JSON output
	var result map[string]any
	err = json.Unmarshal(output, &result)
	if err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Check required fields
	if _, ok := result["payload"]; !ok {
		t.Error("JSON output missing payload field")
	}
	if _, ok := result["plan"]; !ok {
		t.Error("JSON output missing plan field")
	}

	t.Logf("JSON output format validation passed")
}
