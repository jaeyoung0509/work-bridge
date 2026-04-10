package switcher

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/jsonx"
)

type mcpConfigSummary struct {
	Format      string
	Status      string
	ServerNames []string
	Servers     []domain.MCPServerConfig
	Warnings    []string
}

func summarizeMCPConfig(path string, data []byte) mcpConfigSummary {
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if format == "" {
		format = "text"
	}
	parsed := map[string]any{}

	switch format {
	case "toml":
		if err := toml.Unmarshal(data, &parsed); err != nil {
			return mcpConfigSummary{
				Format:   format,
				Status:   "broken",
				Warnings: []string{fmt.Sprintf("parse failed for %s: %v", path, err)},
			}
		}
	case "json", "jsonc":
		if err := jsonx.UnmarshalRelaxed(data, &parsed); err != nil {
			return mcpConfigSummary{
				Format:   format,
				Status:   "broken",
				Warnings: []string{fmt.Sprintf("parse failed for %s: %v", path, err)},
			}
		}
	default:
		return mcpConfigSummary{
			Format:   format,
			Status:   "configured",
			Warnings: []string{"unsupported config format"},
		}
	}

	servers, warnings := extractMCPServers(parsed)
	names := make([]string, 0, len(servers))
	for _, server := range servers {
		names = append(names, server.Name)
	}

	status := "configured"
	switch {
	case len(servers) > 0 && len(warnings) == 0:
		status = "parsed"
	case len(servers) > 0 || len(warnings) > 0:
		status = "degraded"
	}
	return mcpConfigSummary{
		Format:      format,
		Status:      status,
		ServerNames: names,
		Servers:     servers,
		Warnings:    warnings,
	}
}

func extractMCPServers(value any) ([]domain.MCPServerConfig, []string) {
	candidates := []map[string]any{}
	collectMCPServerCandidates(value, nil, &candidates)
	if len(candidates) == 0 {
		return nil, nil
	}
	best := candidates[0]
	servers, warnings := parseMCPServerConfigs(best)
	return servers, dedupeStrings(warnings)
}

func collectMCPServerCandidates(value any, path []string, out *[]map[string]any) {
	node, ok := value.(map[string]any)
	if !ok {
		return
	}
	for key, child := range node {
		nextPath := append(append([]string{}, path...), key)
		childMap, ok := child.(map[string]any)
		if !ok {
			continue
		}
		if scoreMCPServerCandidate(nextPath) > 0 {
			*out = append(*out, childMap)
		}
		collectMCPServerCandidates(childMap, nextPath, out)
	}
}

func parseMCPServerConfigs(values map[string]any) ([]domain.MCPServerConfig, []string) {
	servers := make([]domain.MCPServerConfig, 0, len(values))
	warnings := []string{}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, name := range keys {
		raw := values[name]
		node, ok := raw.(map[string]any)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("server %s is not an object", name))
			continue
		}
		server := domain.MCPServerConfig{
			Name:      name,
			Transport: firstNonEmpty(stringValue(node["transport"]), stringValue(node["type"])),
			Command:   stringValue(node["command"]),
			Args:      stringSliceValue(node["args"]),
			Env:       stringMapValue(node["env"]),
			Cwd:       stringValue(node["cwd"]),
			URL:       firstNonEmpty(stringValue(node["url"]), stringValue(node["endpoint"])),
		}
		if server.Transport == "" {
			switch {
			case server.URL != "":
				server.Transport = "http"
			case server.Command != "":
				server.Transport = "stdio"
			}
		}
		if server.Command == "" && server.URL == "" {
			warnings = append(warnings, fmt.Sprintf("server %s is missing command or url", name))
		}
		servers = append(servers, server)
	}
	return servers, warnings
}

func scoreMCPServerCandidate(path []string) int {
	if len(path) == 0 {
		return 0
	}
	last := normalizeMCPKey(path[len(path)-1])
	switch {
	case last == "mcpservers" || last == "mcp_servers":
		return 100 - len(path)
	case last == "servers" && sliceContains(normalizeSegments(path[:len(path)-1]), "mcp"):
		return 90 - len(path)
	case strings.HasSuffix(last, "mcp_servers"):
		return 85 - len(path)
	default:
		return 0
	}
}

func normalizeSegments(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, normalizeMCPKey(value))
	}
	return out
}

func normalizeMCPKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func sliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func inferMCPTool(name string, path string) domain.Tool {
	value := strings.ToLower(name + " " + path)
	switch {
	case strings.Contains(value, "codex"):
		return domain.ToolCodex
	case strings.Contains(value, "gemini"):
		return domain.ToolGemini
	case strings.Contains(value, "claude"):
		return domain.ToolClaude
	case strings.Contains(value, "opencode"):
		return domain.ToolOpenCode
	default:
		return ""
	}
}

func mcpScopeRank(scope string) int {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case "local":
		return 0
	case "project":
		return 1
	case "user":
		return 2
	case "global":
		return 3
	case "legacy":
		return 4
	default:
		return 5
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func stringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringValue(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func stringMapValue(value any) map[string]string {
	switch typed := value.(type) {
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, val := range typed {
			out[key] = val
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(typed))
		for key, val := range typed {
			switch rendered := val.(type) {
			case string:
				out[key] = rendered
			default:
				out[key] = fmt.Sprint(rendered)
			}
		}
		return out
	default:
		return nil
	}
}
