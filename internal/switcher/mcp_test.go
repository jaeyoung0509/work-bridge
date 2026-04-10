package switcher

import "testing"

func TestSummarizeMCPConfigParsesRelaxedJSON(t *testing.T) {
	t.Parallel()

	summary := summarizeMCPConfig("/tmp/opencode.json", []byte(`{
  "url": "https://example.com/config.json",
  "mcpServers": {
    "github": {
      "command": "mcp-github",
    },
  },
}`))

	if summary.Status != "parsed" {
		t.Fatalf("expected parsed status, got %q (%v)", summary.Status, summary.Warnings)
	}
	if len(summary.ServerNames) != 1 || summary.ServerNames[0] != "github" {
		t.Fatalf("expected github server, got %v", summary.ServerNames)
	}
	if len(summary.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", summary.Warnings)
	}
}
