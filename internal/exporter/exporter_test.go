package exporter

import (
	"strings"
	"testing"

	"sessionport/internal/domain"
	"sessionport/internal/platform/fsx"
)

func TestExportWritesManifestAndFiles(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	bundle := domain.NewSessionBundle(domain.ToolCodex, "/workspace/repo")
	bundle.BundleID = "bundle-1"
	bundle.SourceSessionID = "session-1"
	bundle.TaskTitle = "export test"
	bundle.CurrentGoal = "generate starter artifacts"
	bundle.Summary = "Move the current task into another tool."
	bundle.SettingsSnapshot.Included["model"] = "gpt-5"
	bundle.SettingsSnapshot.ExcludedKeys = append(bundle.SettingsSnapshot.ExcludedKeys, "auth_token")

	report := domain.CompatibilityReport{
		SourceTool:         bundle.SourceTool,
		SourceSessionID:    bundle.SourceSessionID,
		ProjectRoot:        bundle.ProjectRoot,
		TargetTool:         domain.ToolCodex,
		PartialFields:      []string{"settings_snapshot"},
		UnsupportedFields:  []string{"hidden_reasoning"},
		RedactedFields:     []string{"settings.auth_token"},
		GeneratedArtifacts: []string{"AGENTS.sessionport.md", "CONFIG_HINTS.md", "STARTER_PROMPT.md"},
		Warnings:           []string{"config is partial"},
	}

	manifest, err := Export(Options{
		FS:     fsx.OSFS{},
		Bundle: bundle,
		Report: report,
		OutDir: outDir,
	})
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if len(manifest.Files) != 4 {
		t.Fatalf("expected 4 files in manifest, got %#v", manifest.Files)
	}

	data, err := (fsx.OSFS{}).ReadFile(outDir + "/AGENTS.sessionport.md")
	if err != nil {
		t.Fatalf("read exported supplement failed: %v", err)
	}
	if !strings.Contains(string(data), "sessionport Codex supplement") {
		t.Fatalf("expected codex supplement, got %q", string(data))
	}
}
