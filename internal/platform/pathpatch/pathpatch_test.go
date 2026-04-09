package pathpatch_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/platform/pathpatch"
)

// ---------------------------------------------------------------------------
// ClaudeProjectDirName
// ---------------------------------------------------------------------------

func TestClaudeProjectDirName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/Users/alice/myproject", "-Users-alice-myproject"},
		{"/Users/bob/work-bridge", "-Users-bob-work-bridge"},
		{"/home/user/project", "-home-user-project"},
		{"", ""},
		{"/", "-"},
		{"/Users/alice/my project", "-Users-alice-my-project"},
		{"/Users/alice/project123", "-Users-alice-project123"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := pathpatch.ClaudeProjectDirName(tt.input)
			if got != tt.want {
				t.Errorf("ClaudeProjectDirName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// The encoded dir name must never contain double-hyphens (they arise from //
// paths or consecutive special characters).
func TestClaudeProjectDirName_NoConsecutiveHyphens(t *testing.T) {
	result := pathpatch.ClaudeProjectDirName("/Users/alice//project")
	if strings.Contains(result, "--") {
		t.Errorf("ClaudeProjectDirName produced consecutive hyphens: %q", result)
	}
}

// ---------------------------------------------------------------------------
// GeminiProjectHashLegacy
// ---------------------------------------------------------------------------

func TestGeminiProjectHashLegacy(t *testing.T) {
	// Hash must be 64 hex characters (256 bits).
	hash := pathpatch.GeminiProjectHashLegacy("/Users/alice/myproject")
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex, got %d chars: %q", len(hash), hash)
	}
	// Deterministic.
	hash2 := pathpatch.GeminiProjectHashLegacy("/Users/alice/myproject")
	if hash != hash2 {
		t.Error("hash is not deterministic")
	}
	// Different paths must produce different hashes (collision resistance).
	other := pathpatch.GeminiProjectHashLegacy("/Users/bob/myproject")
	if hash == other {
		t.Error("different paths produced identical hashes")
	}
}

// ---------------------------------------------------------------------------
// GeminiProjectSlug
// ---------------------------------------------------------------------------

func TestGeminiProjectSlug_NewProject(t *testing.T) {
	slug := pathpatch.GeminiProjectSlug("/Users/alice/my-project", map[string]string{})
	if slug != "my-project" {
		t.Errorf("expected 'my-project', got %q", slug)
	}
}

func TestGeminiProjectSlug_ExistingMapping(t *testing.T) {
	mapping := map[string]string{"/Users/alice/proj": "proj"}
	slug := pathpatch.GeminiProjectSlug("/Users/alice/proj", mapping)
	if slug != "proj" {
		t.Errorf("expected existing slug 'proj', got %q", slug)
	}
}

func TestGeminiProjectSlug_Collision(t *testing.T) {
	// If 'myproject' already taken, should return 'myproject-1'.
	mapping := map[string]string{"/other/path": "myproject"}
	slug := pathpatch.GeminiProjectSlug("/Users/alice/myproject", mapping)
	if slug != "myproject-1" {
		t.Errorf("expected 'myproject-1' due to collision, got %q", slug)
	}
}

func TestGeminiProjectSlug_MultipleCollisions(t *testing.T) {
	mapping := map[string]string{
		"/other/path1": "myproject",
		"/other/path2": "myproject-1",
	}
	slug := pathpatch.GeminiProjectSlug("/Users/alice/myproject", mapping)
	if slug != "myproject-2" {
		t.Errorf("expected 'myproject-2', got %q", slug)
	}
}

// ---------------------------------------------------------------------------
// PatchJSONBytes
// ---------------------------------------------------------------------------

func TestPatchJSONBytes_StringValue(t *testing.T) {
	input := `{"path": "/Users/alice/project", "name": "test"}`
	out, err := pathpatch.PatchJSONBytes([]byte(input), "/Users/alice", "/Users/bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if result["path"] != "/Users/bob/project" {
		t.Errorf("expected patched path, got %v", result["path"])
	}
	if result["name"] != "test" {
		t.Errorf("name should be unchanged, got %v", result["name"])
	}
}

func TestPatchJSONBytes_NestedObject(t *testing.T) {
	input := `{"outer": {"inner": "/Users/alice/project/file.go"}}`
	out, err := pathpatch.PatchJSONBytes([]byte(input), "/Users/alice", "/Users/bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	outer := result["outer"].(map[string]any)
	if outer["inner"] != "/Users/bob/project/file.go" {
		t.Errorf("nested path not patched, got %v", outer["inner"])
	}
}

func TestPatchJSONBytes_ArrayValues(t *testing.T) {
	input := `{"files": ["/Users/alice/a.go", "/Users/alice/b.go"]}`
	out, err := pathpatch.PatchJSONBytes([]byte(input), "/Users/alice", "/Users/bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	files := result["files"].([]any)
	if files[0] != "/Users/bob/a.go" || files[1] != "/Users/bob/b.go" {
		t.Errorf("array values not patched: %v", files)
	}
}

func TestPatchJSONBytes_SamePath(t *testing.T) {
	input := `{"path": "/Users/alice"}`
	out, err := pathpatch.PatchJSONBytes([]byte(input), "/Users/alice", "/Users/alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) == "" {
		t.Error("output should not be empty")
	}
}

func TestPatchJSONBytes_InvalidJSON(t *testing.T) {
	input := []byte(`not json`)
	out, err := pathpatch.PatchJSONBytes(input, "/Users/alice", "/Users/bob")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if string(out) != string(input) {
		t.Error("should return original on error")
	}
}

// ---------------------------------------------------------------------------
// PatchJSONLBytes
// ---------------------------------------------------------------------------

func TestPatchJSONLBytes(t *testing.T) {
	input := `{"cwd": "/Users/alice/project"}
{"path": "/Users/alice/project/main.go"}
not json
{"other": "value"}
`
	out := pathpatch.PatchJSONLBytes([]byte(input), "/Users/alice", "/Users/bob")
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")

	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line 0 invalid JSON: %v", err)
	}
	if first["cwd"] != "/Users/bob/project" {
		t.Errorf("line 0 cwd not patched: %v", first["cwd"])
	}

	var second map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("line 1 invalid JSON: %v", err)
	}
	if second["path"] != "/Users/bob/project/main.go" {
		t.Errorf("line 1 path not patched: %v", second["path"])
	}

	// unparseable line should pass through
	if lines[2] != "not json" {
		t.Errorf("non-JSON line was modified: %q", lines[2])
	}
}

// ---------------------------------------------------------------------------
// PatchCodexSessionMetaCWD
// ---------------------------------------------------------------------------

func TestPatchCodexSessionMetaCWD_Basic(t *testing.T) {
	input := `{"type":"session_meta","payload":{"id":"abc","cwd":"/Users/alice/project","timestamp":"2026-01-01T00:00:00Z"}}
{"type":"response_item","payload":{"type":"message","content":"hello"}}
`
	out, ok := pathpatch.PatchCodexSessionMetaCWD([]byte(input), "/Users/bob/project")
	if !ok {
		t.Fatal("expected patch to succeed")
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}

	var meta struct {
		Type    string `json:"type"`
		Payload struct {
			CWD string `json:"cwd"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &meta); err != nil {
		t.Fatalf("cannot parse first line: %v", err)
	}
	if meta.Payload.CWD != "/Users/bob/project" {
		t.Errorf("expected cwd '/Users/bob/project', got %q", meta.Payload.CWD)
	}

	// Second line must be unchanged.
	var second map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("cannot parse second line: %v", err)
	}
	if second["type"] != "response_item" {
		t.Errorf("second line type changed: %v", second["type"])
	}
}

func TestPatchCodexSessionMetaCWD_AlreadyCorrect(t *testing.T) {
	input := `{"type":"session_meta","payload":{"id":"abc","cwd":"/Users/bob/project"}}
`
	_, ok := pathpatch.PatchCodexSessionMetaCWD([]byte(input), "/Users/bob/project")
	if ok {
		t.Error("should not report patched when cwd already correct")
	}
}

func TestPatchCodexSessionMetaCWD_EmptyNewCWD(t *testing.T) {
	input := `{"type":"session_meta","payload":{"cwd":"/Users/alice"}}
`
	_, ok := pathpatch.PatchCodexSessionMetaCWD([]byte(input), "")
	if ok {
		t.Error("empty newCWD should not patch")
	}
}

func TestPatchCodexSessionMetaCWD_NotSessionMeta(t *testing.T) {
	input := `{"type":"other","payload":{}}
`
	_, ok := pathpatch.PatchCodexSessionMetaCWD([]byte(input), "/Users/bob/project")
	if ok {
		t.Error("non-session_meta first line should not be patched")
	}
}

// ---------------------------------------------------------------------------
// ReplacePathsInText
// ---------------------------------------------------------------------------

func TestReplacePathsInText(t *testing.T) {
	text := "error: file /Users/alice/project/main.go not found"
	got := pathpatch.ReplacePathsInText(text, "/Users/alice", "/Users/bob")
	want := "error: file /Users/bob/project/main.go not found"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReplacePathsInText_EmptySrc(t *testing.T) {
	text := "no change"
	got := pathpatch.ReplacePathsInText(text, "", "/Users/bob")
	if got != text {
		t.Errorf("empty src should not modify text, got %q", got)
	}
}
