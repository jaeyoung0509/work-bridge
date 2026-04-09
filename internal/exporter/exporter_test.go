package exporter

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sessionport/internal/doctor"
	"sessionport/internal/domain"
	"sessionport/internal/importer"
	"sessionport/internal/platform/fsx"
	"sessionport/internal/testutil"
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

func TestExportManifestMatchesDoctorGeneratedArtifacts(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)
	fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", "codex", "basic_latest"))

	bundle, err := importer.Import(importer.Options{
		FS:         fsx.OSFS{},
		CWD:        fixture.WorkspaceDir,
		HomeDir:    fixture.HomeDir,
		Tool:       "codex",
		Session:    "latest",
		ImportedAt: "2026-04-07T16:00:00Z",
		LookPath:   func(string) (string, error) { return "", nil },
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	for _, target := range []domain.Tool{domain.ToolCodex, domain.ToolGemini, domain.ToolClaude} {
		t.Run(string(target), func(t *testing.T) {
			report, err := doctor.Analyze(doctor.Options{
				Bundle: bundle,
				Target: target,
			})
			if err != nil {
				t.Fatalf("Analyze failed: %v", err)
			}

			outDir := t.TempDir()
			manifest, err := Export(Options{
				FS:     fsx.OSFS{},
				Bundle: bundle,
				Report: report,
				OutDir: outDir,
			})
			if err != nil {
				t.Fatalf("Export failed: %v", err)
			}

			gotArtifacts := append([]string{}, manifest.Files...)
			gotArtifacts = slices.DeleteFunc(gotArtifacts, func(value string) bool { return value == "manifest.json" })
			slices.Sort(gotArtifacts)

			wantArtifacts := append([]string{}, report.GeneratedArtifacts...)
			slices.Sort(wantArtifacts)
			if !slices.Equal(gotArtifacts, wantArtifacts) {
				t.Fatalf("generated artifacts mismatch: want=%v got=%v", wantArtifacts, gotArtifacts)
			}

			data, err := (fsx.OSFS{}).ReadFile(filepath.Join(outDir, "manifest.json"))
			if err != nil {
				t.Fatalf("read manifest failed: %v", err)
			}
			var decoded domain.ExportManifest
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("parse manifest failed: %v", err)
			}
			if decoded.AssetKind != domain.AssetKindSession {
				t.Fatalf("expected asset kind %q, got %q", domain.AssetKindSession, decoded.AssetKind)
			}
		})
	}
}
