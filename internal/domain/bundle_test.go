package domain

import (
	"encoding/json"
	"testing"
)

func TestNewSessionBundleProducesValidDefaults(t *testing.T) {
	t.Parallel()

	bundle := NewSessionBundle(ToolCodex, "/workspace/repo")

	if err := bundle.Validate(); err != nil {
		t.Fatalf("expected valid bundle, got %v", err)
	}
}

func TestSessionBundleValidateRejectsMissingProjectRoot(t *testing.T) {
	t.Parallel()

	bundle := NewSessionBundle(ToolGemini, "")

	if err := bundle.Validate(); err == nil {
		t.Fatal("expected validation error for missing project_root")
	}
}

func TestSessionBundleJSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := NewSessionBundle(ToolClaude, "/workspace/repo")
	original.TaskTitle = "hydrate starter artifact"
	original.SettingsSnapshot.Included["mode"] = "safe"
	original.Warnings = append(original.Warnings, "partial import")

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded SessionBundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.SourceTool != original.SourceTool {
		t.Fatalf("expected source tool %q, got %q", original.SourceTool, decoded.SourceTool)
	}
	if decoded.TaskTitle != original.TaskTitle {
		t.Fatalf("expected task title %q, got %q", original.TaskTitle, decoded.TaskTitle)
	}
	if got := decoded.SettingsSnapshot.Included["mode"]; got != "safe" {
		t.Fatalf("expected settings mode %q, got %#v", "safe", got)
	}
	if len(decoded.Warnings) != 1 || decoded.Warnings[0] != "partial import" {
		t.Fatalf("expected warnings to round-trip, got %#v", decoded.Warnings)
	}
}
