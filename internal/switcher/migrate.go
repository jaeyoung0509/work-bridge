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

	configPath := getLocator(target).ConfigPath(projectRoot)
	if configPath == "" {
		return fmt.Errorf("tool %s does not support a standard MCP config path for this context", target)
	}

	content, warnings, err := adapter.(*projectAdapter).renderMergedTargetConfig(configPath, serversMap)
	if err != nil {
		return fmt.Errorf("failed to merge MCP config: %w", err)
	}
	
	if len(warnings) > 0 {
		// Just log them internally if needed. But for the user, it means some servers had issues.
	}

	_, _, err = adapter.(*projectAdapter).writeFile(configPath, content)
	if err != nil {
		return fmt.Errorf("failed to write patched MCP config: %w", err)
	}

	return nil
}

// MigrateSkill copies a skill bundle from its source into the target LLM's skills directory.
func (s *Service) MigrateSkill(ctx context.Context, entry catalog.SkillEntry, target domain.Tool, projectRoot string) error {
	projectRoot, err := s.resolveProjectRoot(projectRoot)
	if err != nil {
		return err
	}

	skillsRoot := getLocator(target).ProjectSkillRoot(projectRoot)
	if skillsRoot == "" {
		return fmt.Errorf("tool %s does not support a standard skills directory", target)
	}

	slug := sanitizeSkillName(entry.Name)
	if slug == "" {
		slug = "skill"
	}
	
	targetDir := filepath.Join(skillsRoot, slug)
	
	if _, err := s.fs.Stat(targetDir); err == nil {
		return fmt.Errorf("skill '%s' already exists at %s", slug, targetDir)
	} else if !os.IsNotExist(err) {
		return err
	}
	
	skillFiles := entry.Files
	if len(skillFiles) == 0 {
		skillFiles = []string{entry.EntryPath}
	}
	
	for _, src := range skillFiles {
		rel, err := filepath.Rel(entry.RootPath, src)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
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
