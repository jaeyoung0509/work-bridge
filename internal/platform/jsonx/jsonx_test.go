package jsonx

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalRelaxedSupportsCommentsAndTrailingCommas(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  // comment
  "url": "https://example.com/config.json",
  "mcpServers": {
    "github": {
      "command": "mcp-github",
    },
  },
}`)

	var parsed map[string]any
	if err := UnmarshalRelaxed(raw, &parsed); err != nil {
		t.Fatalf("expected relaxed parse to succeed, got %v", err)
	}

	servers, ok := parsed["mcpServers"].(map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one MCP server, got %#v", parsed["mcpServers"])
	}
	if got, _ := parsed["url"].(string); got != "https://example.com/config.json" {
		t.Fatalf("expected URL to be preserved, got %q", got)
	}

	if _, err := json.Marshal(parsed); err != nil {
		t.Fatalf("expected sanitized content to remain valid JSON, got %v", err)
	}
}
