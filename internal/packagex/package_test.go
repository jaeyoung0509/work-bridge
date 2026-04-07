package packagex

import (
	"bytes"
	"path/filepath"
	"testing"

	"sessionport/internal/domain"
	"sessionport/internal/platform/archivex"
	"sessionport/internal/platform/fsx"
)

func TestPackAndUnpackRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	archivePath := filepath.Join(root, "bundle.spkg")
	outDir := filepath.Join(root, "out")

	bundle := domain.NewSessionBundle(domain.ToolCodex, "/workspace/repo")
	bundle.BundleID = "bundle-1"
	bundle.SourceSessionID = "session-1"
	bundle.TaskTitle = "pack bundle"
	bundle.CurrentGoal = "round-trip archive"
	bundle.Summary = "Portable bundle."
	bundle.InstructionArtifacts = append(bundle.InstructionArtifacts, domain.InstructionArtifact{
		Tool:    domain.ToolCodex,
		Kind:    "project_instruction",
		Path:    "/workspace/repo/AGENTS.md",
		Scope:   "project",
		Content: "# instructions",
	})

	manifest, err := Pack(PackOptions{
		Archive: archivex.ZIPArchive{},
		Bundle:  bundle,
		OutFile: archivePath,
	})
	if err != nil {
		t.Fatalf("Pack failed: %v", err)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("expected package manifest files, got %#v", manifest.Files)
	}

	result, err := Unpack(UnpackOptions{
		Archive: archivex.ZIPArchive{},
		FS:      fsx.OSFS{},
		File:    archivePath,
		Target:  domain.ToolClaude,
		OutDir:  outDir,
	})
	if err != nil {
		t.Fatalf("Unpack failed: %v", err)
	}

	if result.BundlePath != filepath.Join(outDir, bundleFileName) {
		t.Fatalf("unexpected bundle path: %#v", result)
	}
	if len(result.ExportManifest.Files) != 4 {
		t.Fatalf("expected export files after unpack, got %#v", result.ExportManifest.Files)
	}
}

func TestPackIsDeterministic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	first := filepath.Join(root, "first.spkg")
	second := filepath.Join(root, "second.spkg")

	bundle := domain.NewSessionBundle(domain.ToolGemini, "/workspace/repo")
	bundle.BundleID = "bundle-2"
	bundle.SourceSessionID = "session-2"
	bundle.TaskTitle = "deterministic"
	bundle.CurrentGoal = "compare archives"

	for _, path := range []string{first, second} {
		if _, err := Pack(PackOptions{
			Archive: archivex.ZIPArchive{},
			Bundle:  bundle,
			OutFile: path,
		}); err != nil {
			t.Fatalf("Pack failed: %v", err)
		}
	}

	left, err := (fsx.OSFS{}).ReadFile(first)
	if err != nil {
		t.Fatalf("read first archive failed: %v", err)
	}
	right, err := (fsx.OSFS{}).ReadFile(second)
	if err != nil {
		t.Fatalf("read second archive failed: %v", err)
	}
	if !bytes.Equal(left, right) {
		t.Fatalf("expected deterministic archives")
	}
}
