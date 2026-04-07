package doctor

import (
	"strings"
	"testing"

	"sessionport/internal/domain"
)

func TestAnalyzeMatrix(t *testing.T) {
	t.Parallel()

	bundle := domain.NewSessionBundle(domain.ToolCodex, "/workspace/repo")
	bundle.BundleID = "bundle-1"
	bundle.SourceSessionID = "session-1"
	bundle.TaskTitle = "doctor test"
	bundle.CurrentGoal = "render compatibility"
	bundle.Summary = "Summarize portability."
	bundle.InstructionArtifacts = append(bundle.InstructionArtifacts, domain.InstructionArtifact{
		Tool:  domain.ToolCodex,
		Kind:  "project_instruction",
		Path:  "/workspace/repo/AGENTS.md",
		Scope: "project",
	})
	bundle.SettingsSnapshot.Included["model"] = "gpt-5"
	bundle.SettingsSnapshot.ExcludedKeys = append(bundle.SettingsSnapshot.ExcludedKeys, "auth_token")
	bundle.ToolEvents = append(bundle.ToolEvents, domain.ToolEvent{Type: "tool_call", Summary: "exec_command"})
	bundle.ResumeHints = append(bundle.ResumeHints, "source_session_path=/tmp/session.jsonl")
	bundle.TokenStats["total_tokens"] = 15

	cases := []struct {
		name          string
		target        domain.Tool
		generated     string
		targetWarning string
	}{
		{
			name:          "codex target",
			target:        domain.ToolCodex,
			generated:     "AGENTS.sessionport.md",
			targetWarning: "Codex export will provide config hints only; vendor-native session resume state is not reconstructed.",
		},
		{
			name:          "gemini target",
			target:        domain.ToolGemini,
			generated:     "GEMINI.sessionport.md",
			targetWarning: "Gemini export will emit a settings patch suggestion rather than replacing the full local profile.",
		},
		{
			name:          "claude target",
			target:        domain.ToolClaude,
			generated:     "CLAUDE.sessionport.md",
			targetWarning: "Claude export will convert portable context into CLAUDE.md supplements and plain memory notes.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report, err := Analyze(Options{
				Bundle: bundle,
				Target: tc.target,
			})
			if err != nil {
				t.Fatalf("Analyze failed: %v", err)
			}

			if report.TargetTool != tc.target {
				t.Fatalf("expected target %q, got %q", tc.target, report.TargetTool)
			}
			assertContains(t, report.CompatibleFields, "instruction_artifacts")
			assertContains(t, report.PartialFields, "settings_snapshot")
			assertContains(t, report.PartialFields, "raw_transcript")
			assertContains(t, report.UnsupportedFields, "hidden_reasoning")
			assertContains(t, report.RedactedFields, "settings.auth_token")
			assertContains(t, report.GeneratedArtifacts, tc.generated)
			assertContains(t, report.Warnings, tc.targetWarning)
		})
	}
}

func TestRenderText(t *testing.T) {
	t.Parallel()

	report := domain.CompatibilityReport{
		SourceTool:         domain.ToolCodex,
		SourceSessionID:    "session-1",
		ProjectRoot:        "/workspace/repo",
		TargetTool:         domain.ToolClaude,
		CompatibleFields:   []string{"project_root", "summary"},
		PartialFields:      []string{"settings_snapshot"},
		UnsupportedFields:  []string{"hidden_reasoning"},
		RedactedFields:     []string{"settings.auth_token"},
		GeneratedArtifacts: []string{"CLAUDE.sessionport.md"},
		Warnings:           []string{"history-based import"},
	}

	output := RenderText(report)

	for _, want := range []string{"Doctor Report", "Source: codex session session-1", "Target: claude", "Compatible (2):", "Generated artifacts (1):"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output, got %q", want, output)
		}
	}
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("expected %q in %#v", want, values)
}
