package switcher

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

func TestBuildOpenCodeImportPayloadIncludesImportRequiredFields(t *testing.T) {
	now := time.Date(2026, 4, 10, 15, 13, 56, 0, time.UTC)
	sourceRoot := "/Users/source/project"
	targetRoot := "/Users/target/project"

	bundle := domain.NewSessionBundle(domain.ToolClaude, sourceRoot)
	bundle.SourceSessionID = "claude-session-123"
	bundle.TaskTitle = "Port native mode support"
	bundle.CurrentGoal = "Check " + filepath.Join(sourceRoot, "README.md")
	bundle.Summary = "Updated " + filepath.Join(sourceRoot, "internal", "switcher", "native_opencode.go")

	payload := buildOpenCodeImportPayload(bundle, targetRoot, now)

	model, ok := payload["model"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level model, got %#v", payload["model"])
	}
	if model["providerID"] != openCodeDefaultProviderID {
		t.Fatalf("expected providerID %q, got %#v", openCodeDefaultProviderID, model["providerID"])
	}
	if model["modelID"] != openCodeDefaultModelID {
		t.Fatalf("expected modelID %q, got %#v", openCodeDefaultModelID, model["modelID"])
	}

	info := payload["info"].(map[string]any)
	if info["directory"] != targetRoot {
		t.Fatalf("expected directory %q, got %#v", targetRoot, info["directory"])
	}
	if info["version"] != openCodePayloadVersion {
		t.Fatalf("expected version %q, got %#v", openCodePayloadVersion, info["version"])
	}

	messages, ok := payload["messages"].([]map[string]any)
	if !ok {
		t.Fatalf("expected messages slice, got %#v", payload["messages"])
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	userInfo := messages[0]["info"].(map[string]any)
	if userInfo["role"] != "user" {
		t.Fatalf("expected first role user, got %#v", userInfo["role"])
	}
	if userInfo["agent"] != openCodeDefaultAgent {
		t.Fatalf("expected user agent %q, got %#v", openCodeDefaultAgent, userInfo["agent"])
	}
	if _, ok := userInfo["model"].(map[string]any); !ok {
		t.Fatalf("expected user model block, got %#v", userInfo["model"])
	}

	userParts := messages[0]["parts"].([]map[string]any)
	if len(userParts) != 1 || userParts[0]["type"] != "text" {
		t.Fatalf("expected one user text part, got %#v", userParts)
	}
	if text := userParts[0]["text"]; text != "Check "+filepath.Join(targetRoot, "README.md") {
		t.Fatalf("expected patched user text, got %#v", text)
	}

	assistantInfo := messages[1]["info"].(map[string]any)
	if assistantInfo["role"] != "assistant" {
		t.Fatalf("expected second role assistant, got %#v", assistantInfo["role"])
	}
	if assistantInfo["parentID"] != userInfo["id"] {
		t.Fatalf("expected assistant parentID %#v, got %#v", userInfo["id"], assistantInfo["parentID"])
	}
	if assistantInfo["providerID"] != openCodeDefaultProviderID {
		t.Fatalf("expected assistant providerID %q, got %#v", openCodeDefaultProviderID, assistantInfo["providerID"])
	}
	if assistantInfo["modelID"] != openCodeDefaultModelID {
		t.Fatalf("expected assistant modelID %q, got %#v", openCodeDefaultModelID, assistantInfo["modelID"])
	}
	if assistantInfo["finish"] != "stop" {
		t.Fatalf("expected finish stop, got %#v", assistantInfo["finish"])
	}

	pathInfo := assistantInfo["path"].(map[string]any)
	if pathInfo["cwd"] != targetRoot || pathInfo["root"] != targetRoot {
		t.Fatalf("expected assistant paths patched to %q, got %#v", targetRoot, pathInfo)
	}

	assistantParts := messages[1]["parts"].([]map[string]any)
	if len(assistantParts) != 3 {
		t.Fatalf("expected 3 assistant parts, got %#v", assistantParts)
	}
	if assistantParts[0]["type"] != "step-start" || assistantParts[1]["type"] != "text" || assistantParts[2]["type"] != "step-finish" {
		t.Fatalf("unexpected assistant parts %#v", assistantParts)
	}
	if text := assistantParts[1]["text"]; text != "Updated "+filepath.Join(targetRoot, "internal", "switcher", "native_opencode.go") {
		t.Fatalf("expected patched assistant text, got %#v", text)
	}
}

func TestBuildOpenCodeImportPayloadFallsBackToMinimalUserMessage(t *testing.T) {
	now := time.Date(2026, 4, 10, 15, 13, 56, 0, time.UTC)
	projectRoot := "/Users/target/project"

	bundle := domain.NewSessionBundle(domain.ToolCodex, "/Users/source/project")
	payload := buildOpenCodeImportPayload(bundle, projectRoot, now)

	messages := payload["messages"].([]map[string]any)
	if len(messages) != 1 {
		t.Fatalf("expected 1 fallback message, got %d", len(messages))
	}

	info := messages[0]["info"].(map[string]any)
	if info["role"] != "user" {
		t.Fatalf("expected fallback role user, got %#v", info["role"])
	}
	if _, ok := info["model"].(map[string]any); !ok {
		t.Fatalf("expected fallback model block, got %#v", info["model"])
	}

	parts := messages[0]["parts"].([]map[string]any)
	if len(parts) != 1 || parts[0]["type"] != "text" {
		t.Fatalf("expected one fallback text part, got %#v", parts)
	}
	if parts[0]["text"] != "Imported session via work-bridge" {
		t.Fatalf("unexpected fallback text %#v", parts[0]["text"])
	}
}
