package domain

import "path/filepath"

type AssetKind string

const (
	AssetKindSession AssetKind = "session"
	AssetKindSkill   AssetKind = "skill"
)

func (k AssetKind) IsKnown() bool {
	switch k {
	case AssetKindSession, AssetKindSkill:
		return true
	default:
		return false
	}
}

func (k AssetKind) IsStable() bool {
	return k == AssetKindSession
}

type ToolPaths struct {
	Codex    string `json:"codex,omitempty" mapstructure:"codex"`
	Gemini   string `json:"gemini,omitempty" mapstructure:"gemini"`
	Claude   string `json:"claude,omitempty" mapstructure:"claude"`
	OpenCode string `json:"opencode,omitempty" mapstructure:"opencode"`
}

func (p ToolPaths) Dir(tool Tool, homeDir string) string {
	switch tool {
	case ToolCodex:
		if p.Codex != "" {
			return filepath.Clean(p.Codex)
		}
		return filepath.Join(homeDir, ".codex")
	case ToolGemini:
		if p.Gemini != "" {
			return filepath.Clean(p.Gemini)
		}
		return filepath.Join(homeDir, ".gemini")
	case ToolClaude:
		if p.Claude != "" {
			return filepath.Clean(p.Claude)
		}
		return filepath.Join(homeDir, ".claude")
	case ToolOpenCode:
		if p.OpenCode != "" {
			return filepath.Clean(p.OpenCode)
		}
		return filepath.Join(homeDir, ".local", "share", "opencode")
	default:
		return filepath.Clean(homeDir)
	}
}

type RedactionPolicy struct {
	AdditionalSensitiveKeys []string `json:"additional_sensitive_keys,omitempty" mapstructure:"additional_sensitive_keys"`
	DetectSensitiveValues   bool     `json:"detect_sensitive_values" mapstructure:"detect_sensitive_values"`
}
