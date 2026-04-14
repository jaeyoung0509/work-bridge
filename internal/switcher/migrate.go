package switcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/catalog"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

// MigrateMCP reads the MCP servers from the given entry's config path and patches them into the target LLM's config.
func (s *Service) MigrateMCP(ctx context.Context, entry catalog.MCPEntry, target domain.Tool, projectRoot string) error {
	projectRoot, err := s.resolveProjectRoot(projectRoot)
	if err != nil {
		return err
	}

	data, err := s.fs.ReadFile(entry.Path)
	if err != nil {
		return fmt.Errorf("failed to read source MCP config: %w", err)
	}

	summary := summarizeMCPConfig(entry.Path, data)
	if summary.Status == "broken" {
		return fmt.Errorf("invalid MCP config: %s", strings.Join(summary.Warnings, ", "))
	}

	if len(summary.Servers) == 0 {
		return fmt.Errorf("no MCP servers found in %s", entry.Path)
	}

	serversMap := sliceToMCPServerMap(summary.Servers)

	adapter, err := s.adapterFor(target)
	if err != nil {
		return err
	}

	// Try project-local config, otherwise fall back to global config
	configPath := getLocator(target).ConfigPath(projectRoot)
	if configPath == "" {
		return fmt.Errorf("tool %s does not support a standard MCP config path", target)
	}

	content, _, err := adapter.(*projectAdapter).renderMergedTargetConfig(configPath, serversMap)
	if err != nil {
		return fmt.Errorf("failed to merge MCP config: %w", err)
	}

	_, _, err = adapter.(*projectAdapter).writeFile(configPath, content)
	if err != nil {
		return fmt.Errorf("failed to write patched MCP config: %w", err)
	}

	return nil
}

// MigrateSkill copies a skill bundle from its source into the target LLM's skills directory.
// It prefers the global (home-dir) destination so the skill is available across all projects.
func (s *Service) MigrateSkill(ctx context.Context, entry catalog.SkillEntry, target domain.Tool, projectRoot string) error {
	// Pick the destination: global first, then project-local fallback
	skillsRoot := s.globalSkillRoot(target)
	if skillsRoot == "" {
		// Fall back to project-local
		resolved, err := s.resolveProjectRoot(projectRoot)
		if err != nil {
			return err
		}
		skillsRoot = getLocator(target).ProjectSkillRoot(resolved)
	}
	if skillsRoot == "" {
		return fmt.Errorf("tool %s does not have a known skills directory", target)
	}

	slug := sanitizeSkillName(entry.Name)
	if slug == "" {
		slug = "skill"
	}

	targetDir := filepath.Join(skillsRoot, slug)

	if _, err := s.fs.Stat(targetDir); err == nil {
		return fmt.Errorf("skill %q already installed at %s", slug, targetDir)
	} else if !os.IsNotExist(err) {
		return err
	}

	// entry.Files is the authoritative list of all files in the bundle
	skillFiles := entry.Files
	if len(skillFiles) == 0 && entry.EntryPath != "" {
		skillFiles = []string{entry.EntryPath}
	}
	if len(skillFiles) == 0 {
		return fmt.Errorf("skill bundle %q has no files to copy", entry.Name)
	}

	// sourceDir is the bundle root — the containing directory of SKILL.md
	sourceDir := entry.RootPath

	for _, src := range skillFiles {
		rel, err := filepath.Rel(sourceDir, src)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}

		dst := filepath.Join(targetDir, rel)
		data, err := s.fs.ReadFile(src)
		if err != nil {
			return fmt.Errorf("failed reading skill file %s: %w", src, err)
		}

		if err := s.fs.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}

		if err := s.fs.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("failed writing skill file %s: %w", dst, err)
		}
	}

	return nil
}

// globalSkillRoot returns the home-directory based skill root for the given tool,
// so installed skills are available across all projects.
func (s *Service) globalSkillRoot(target domain.Tool) string {
	switch target {
	case domain.ToolGemini:
		return filepath.Join(s.homeDir, ".gemini", "skills")
	case domain.ToolClaude:
		return filepath.Join(s.homeDir, ".claude", "skills")
	case domain.ToolCodex:
		return filepath.Join(s.homeDir, ".codex", "skills")
	case domain.ToolOpenCode:
		return filepath.Join(s.homeDir, ".config", "opencode", "skills")
	default:
		return ""
	}
}
