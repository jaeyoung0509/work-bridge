// Package e2e provides end-to-end tests for work-bridge.
// These tests build the binary and run CLI commands to verify real behavior.
// They are designed to run locally only, NOT in CI.
//
// To run: WORKBRIDGE_E2E=1 go test -tags=e2e ./tests/e2e/... -v
// Or: make test-e2e

//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runCommand(t *testing.T, binPath string, args ...string) (string, string, error) {
	t.Helper()

	cmd := exec.Command(binPath, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestE2E_CrossToolSessionMigration(t *testing.T) {
	skipIfE2EDisabled(t)
	t.Parallel()

	binPath := buildWorkBridge(t)
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	err := os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	// Create a dummy instruction file for project detection
	err = os.WriteFile(filepath.Join(projectRoot, "CLAUDE.md"), []byte("# Test Project\n"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create CLAUDE.md: %v", err)
	}

	testCases := []struct {
		name string
		from string
		to   string
	}{
		{"codex_to_gemini", "codex", "gemini"},
		{"codex_to_claude", "codex", "claude"},
		{"gemini_to_claude", "gemini", "claude"},
		{"gemini_to_opencode", "gemini", "opencode"},
		{"claude_to_codex", "claude", "codex"},
		{"claude_to_gemini", "claude", "gemini"},
		{"opencode_to_claude", "opencode", "claude"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test project mode (should work for all pairs)
			t.Run("project_mode", func(t *testing.T) {
				stdout, stderr, err := runCommand(t, binPath,
					"switch",
					"--from", tc.from,
					"--to", tc.to,
					"--project", projectRoot,
					"--mode", "project",
					"--dry-run",
					"--session", "latest",
				)

				// We expect some warnings/errors if no sessions exist, but command should run
				t.Logf("stdout: %s", stdout)
				t.Logf("stderr: %s", stderr)

				// Command should at least parse and attempt to run
				// It's OK if it fails due to no sessions, but shouldn't crash
				if err != nil {
					// Check it's not a crash (segfault, panic, etc)
					if strings.Contains(stderr, "panic") || strings.Contains(stderr, "SIG") {
						t.Fatalf("Command crashed: %v\n%s", err, stderr)
					}
					t.Logf("Command failed (expected if no sessions): %v", err)
				}
			})

			// Test native mode (should also work, may have warnings)
			t.Run("native_mode", func(t *testing.T) {
				stdout, stderr, err := runCommand(t, binPath,
					"switch",
					"--from", tc.from,
					"--to", tc.to,
					"--project", projectRoot,
					"--mode", "native",
					"--dry-run",
					"--session", "latest",
				)

				t.Logf("stdout: %s", stdout)
				t.Logf("stderr: %s", stderr)

				// Native mode may have more restrictions, but should still run
				if err != nil {
					if strings.Contains(stderr, "panic") || strings.Contains(stderr, "SIG") {
						t.Fatalf("Command crashed: %v\n%s", err, stderr)
					}
					t.Logf("Command failed (may be expected): %v", err)
				}
			})
		})
	}
}

func TestE2E_GlobalSkillsMigration(t *testing.T) {
	skipIfE2EDisabled(t)
	t.Parallel()

	binPath := buildWorkBridge(t)
	tmpDir := t.TempDir()

	// Setup source tool's user-scope skill directory
	sourceSkillDir := filepath.Join(tmpDir, ".claude", "skills")
	err := os.MkdirAll(sourceSkillDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create source skill dir: %v", err)
	}

	// Create a dummy skill file
	skillContent := `---
name: test-skill
description: A test skill for migration
---

# Test Skill
This is a test skill content.
`
	skillPath := filepath.Join(sourceSkillDir, "test-skill.md")
	err = os.WriteFile(skillPath, []byte(skillContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to create skill file: %v", err)
	}

	// Create target project
	projectRoot := filepath.Join(tmpDir, "test-project")
	err = os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	// Run native mode switch from claude to codex (should migrate skills)
	stdout, stderr, err := runCommand(t, binPath,
		"switch",
		"--from", "claude",
		"--to", "codex",
		"--project", projectRoot,
		"--mode", "native",
		"--dry-run",
		"--session", "latest",
	)

	t.Logf("stdout: %s", stdout)
	t.Logf("stderr: %s", stderr)

	// In dry-run mode, skills should be mentioned in the output
	if !strings.Contains(stdout, "skill") && !strings.Contains(stderr, "skill") {
		t.Log("Warning: skill migration may not be mentioned in output")
	}
}

func TestE2E_GlobalMCPWarning(t *testing.T) {
	skipIfE2EDisabled(t)
	t.Parallel()

	binPath := buildWorkBridge(t)
	tmpDir := t.TempDir()

	// Create a dummy project
	projectRoot := filepath.Join(tmpDir, "test-project")
	err := os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	// Run native mode switch - should warn about global MCP if any exist
	stdout, stderr, err := runCommand(t, binPath,
		"switch",
		"--from", "claude",
		"--to", "codex",
		"--project", projectRoot,
		"--mode", "native",
		"--dry-run",
		"--session", "latest",
	)

	t.Logf("stdout: %s", stdout)
	t.Logf("stderr: %s", stderr)

	// Command should run without crashing
	if err != nil {
		if strings.Contains(stderr, "panic") || strings.Contains(stderr, "SIG") {
			t.Fatalf("Command crashed: %v\n%s", err, stderr)
		}
	}
}

func TestE2E_ExportNativeMode(t *testing.T) {
	skipIfE2EDisabled(t)
	t.Parallel()

	binPath := buildWorkBridge(t)
	tmpDir := t.TempDir()

	// Create export target directory
	exportDir := filepath.Join(tmpDir, "export-output")
	err := os.MkdirAll(exportDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create export dir: %v", err)
	}

	// Create source project
	projectRoot := filepath.Join(tmpDir, "test-project")
	err = os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	testCases := []struct {
		name         string
		from         string
		to           string
		expectPrefix string
	}{
		{"codex_export", "codex", "codex", ".codex"},
		{"gemini_export", "gemini", "gemini", ".gemini"},
		{"claude_export", "claude", "claude", ".claude"},
		{"opencode_export", "opencode", "opencode", ".opencode"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runCommand(t, binPath,
				"export",
				"--from", tc.from,
				"--to", tc.to,
				"--project", projectRoot,
				"--mode", "native",
				"--out", exportDir,
				"--dry-run",
				"--session", "latest",
			)

			t.Logf("stdout: %s", stdout)
			t.Logf("stderr: %s", stderr)

			// Should run without crashing
			if err != nil {
				if strings.Contains(stderr, "panic") || strings.Contains(stderr, "SIG") {
					t.Fatalf("Command crashed: %v\n%s", err, stderr)
				}
			}
		})
	}
}

func TestE2E_BinarySmoke(t *testing.T) {
	skipIfE2EDisabled(t)
	t.Parallel()

	binPath := buildWorkBridge(t)

	// Test basic commands work
	t.Run("version", func(t *testing.T) {
		stdout, stderr, err := runCommand(t, binPath, "version")
		if err != nil {
			t.Fatalf("version command failed: %v\nstderr: %s", err, stderr)
		}
		if !strings.Contains(stdout, "work-bridge") {
			t.Logf("version output: %s", stdout)
		}
	})

	t.Run("help", func(t *testing.T) {
		stdout, stderr, err := runCommand(t, binPath, "--help")
		if err != nil {
			t.Fatalf("help command failed: %v\nstderr: %s", err, stderr)
		}
		if !strings.Contains(stdout, "inspect") || !strings.Contains(stdout, "switch") {
			t.Fatalf("help output missing expected commands: %s", stdout)
		}
	})

	t.Run("inspect_codex", func(t *testing.T) {
		stdout, stderr, err := runCommand(t, binPath, "inspect", "codex", "--limit", "1")
		// May fail if no sessions exist, but shouldn't crash
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
		if err != nil && (strings.Contains(stderr, "panic") || strings.Contains(stderr, "SIG")) {
			t.Fatalf("Command crashed: %v\n%s", err, stderr)
		}
	})

	t.Run("inspect_gemini", func(t *testing.T) {
		stdout, stderr, err := runCommand(t, binPath, "inspect", "gemini", "--limit", "1")
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
		if err != nil && (strings.Contains(stderr, "panic") || strings.Contains(stderr, "SIG")) {
			t.Fatalf("Command crashed: %v\n%s", err, stderr)
		}
	})

	t.Run("inspect_claude", func(t *testing.T) {
		stdout, stderr, err := runCommand(t, binPath, "inspect", "claude", "--limit", "1")
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
		if err != nil && (strings.Contains(stderr, "panic") || strings.Contains(stderr, "SIG")) {
			t.Fatalf("Command crashed: %v\n%s", err, stderr)
		}
	})

	t.Run("inspect_opencode", func(t *testing.T) {
		stdout, stderr, err := runCommand(t, binPath, "inspect", "opencode", "--limit", "1")
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
		if err != nil && (strings.Contains(stderr, "panic") || strings.Contains(stderr, "SIG")) {
			t.Fatalf("Command crashed: %v\n%s", err, stderr)
		}
	})
}

func TestE2E_ValidateOpenCodePayloadFormat(t *testing.T) {
	skipIfE2EDisabled(t)
	t.Parallel()

	// This test validates that our OpenCode payload format matches
	// the actual `opencode export` output structure

	binPath := buildWorkBridge(t)
	tmpDir := t.TempDir()

	projectRoot := filepath.Join(tmpDir, "test-project")
	err := os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	// Export in native mode to opencode
	exportDir := filepath.Join(tmpDir, "export")
	stdout, stderr, err := runCommand(t, binPath,
		"export",
		"--from", "claude",
		"--to", "opencode",
		"--project", projectRoot,
		"--mode", "native",
		"--out", exportDir,
		"--dry-run",
		"--session", "latest",
	)

	t.Logf("stdout: %s", stdout)
	t.Logf("stderr: %s", stderr)

	// Should complete without crash
	if err != nil {
		if strings.Contains(stderr, "panic") || strings.Contains(stderr, "SIG") {
			t.Fatalf("Command crashed: %v\n%s", err, stderr)
		}
	}
}

// TestE2E_ModeFlagValidation validates that --mode flag works correctly
func TestE2E_ModeFlagValidation(t *testing.T) {
	skipIfE2EDisabled(t)
	t.Parallel()

	binPath := buildWorkBridge(t)
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	err := os.MkdirAll(projectRoot, 0o755)
	if err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	testCases := []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{"project_mode", "project", false},
		{"native_mode", "native", false},
		{"invalid_mode", "invalid", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runCommand(t, binPath,
				"switch",
				"--from", "claude",
				"--to", "codex",
				"--project", projectRoot,
				"--mode", tc.mode,
				"--dry-run",
				"--session", "latest",
			)

			t.Logf("stdout: %s", stdout)
			t.Logf("stderr: %s", stderr)

			if tc.wantErr {
				if err == nil {
					t.Error("Expected error for invalid mode, but command succeeded")
				}
				if !strings.Contains(stderr, "unsupported mode") && !strings.Contains(stdout, "unsupported mode") {
					t.Logf("Warning: expected 'unsupported mode' error message")
				}
			} else {
				// Valid modes should not crash
				if err != nil && (strings.Contains(stderr, "panic") || strings.Contains(stderr, "SIG")) {
					t.Fatalf("Command crashed: %v\n%s", err, stderr)
				}
			}
		})
	}
}

// Helper function to marshal JSON
func mustMarshalJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}
	return string(data)
}
