package switcher

import (
	"path/filepath"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

type ToolLocator interface {
	InstructionPath(projectRoot string) string
	ConfigPath(projectRoot string) string
	ProjectSkillRoot(destinationRoot string) string
}

func getLocator(target domain.Tool) ToolLocator {
	switch target {
	case domain.ToolClaude:
		return &claudeLocator{}
	case domain.ToolGemini:
		return &geminiLocator{}
	case domain.ToolCodex:
		return &codexLocator{}
	case domain.ToolOpenCode:
		return &opencodeLocator{}
	default:
		return &defaultLocator{}
	}
}

type defaultLocator struct{}

func (l *defaultLocator) InstructionPath(root string) string { return filepath.Join(root, "AGENTS.md") }
func (l *defaultLocator) ConfigPath(root string) string { return "" }
func (l *defaultLocator) ProjectSkillRoot(root string) string { return "" }

type claudeLocator struct{}

func (l *claudeLocator) InstructionPath(root string) string { return filepath.Join(root, "CLAUDE.md") }
func (l *claudeLocator) ConfigPath(root string) string { return filepath.Join(root, ".claude", "settings.local.json") }
func (l *claudeLocator) ProjectSkillRoot(root string) string { return filepath.Join(root, ".claude", "skills") }

type geminiLocator struct{}

func (l *geminiLocator) InstructionPath(root string) string { return filepath.Join(root, "GEMINI.md") }
func (l *geminiLocator) ConfigPath(root string) string { return filepath.Join(root, ".gemini", "settings.json") }
func (l *geminiLocator) ProjectSkillRoot(root string) string { return filepath.Join(root, ".agents", "skills") }

type codexLocator struct{}

func (l *codexLocator) InstructionPath(root string) string { return filepath.Join(root, "AGENTS.md") }
func (l *codexLocator) ConfigPath(root string) string { return filepath.Join(root, ".codex", "config.toml") }
func (l *codexLocator) ProjectSkillRoot(root string) string { return filepath.Join(root, ".agents", "skills") }

type opencodeLocator struct{}

func (l *opencodeLocator) InstructionPath(root string) string { return filepath.Join(root, "AGENTS.md") }
func (l *opencodeLocator) ConfigPath(root string) string { return filepath.Join(root, ".opencode", "opencode.jsonc") }
func (l *opencodeLocator) ProjectSkillRoot(root string) string { return filepath.Join(root, ".opencode", "skills") }
