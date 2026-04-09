package catalog

import (
	"path/filepath"
	"sort"
	"strings"

	"sessionport/internal/domain"
	"sessionport/internal/platform/fsx"
)

type SkillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Source      string `json:"source"`
}

type MCPEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Source  string `json:"source"`
	Status  string `json:"status"`
	Details string `json:"details"`
}

func ScanSkills(fs fsx.FS, cwd, homeDir string) ([]SkillEntry, error) {
	roots := []string{
		filepath.Join(cwd, ".github", "skills"),
		filepath.Join(cwd, "skills"),
		filepath.Join(homeDir, ".codex", "skills"),
		filepath.Join(homeDir, ".claude", "skills"),
		filepath.Join(homeDir, ".config", "opencode", "skills"),
		filepath.Join(homeDir, ".local", "share", "opencode", "skills"),
	}

	entries := []SkillEntry{}
	for _, root := range roots {
		files, err := listMarkdownFiles(fs, root, "SKILL.md")
		if err != nil {
			return nil, err
		}
		for _, path := range files {
			entry := parseSkillEntry(fs, path, root)
			if entry.Name == "" {
				continue
			}
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Name < entries[j].Name
	})
	return dedupeSkills(entries), nil
}

func ScanMCP(fs fsx.FS, cwd, homeDir string, paths domain.ToolPaths) ([]MCPEntry, error) {
	candidates := []MCPEntry{
		{Name: "project opencode config", Path: filepath.Join(cwd, ".opencode", "opencode.jsonc"), Source: "project"},
		{Name: "project opencode config", Path: filepath.Join(cwd, ".opencode", "opencode.json"), Source: "project"},
		{Name: "global opencode config", Path: filepath.Join(homeDir, ".config", "opencode", "opencode.jsonc"), Source: "user"},
		{Name: "global opencode config", Path: filepath.Join(homeDir, ".config", "opencode", "opencode.json"), Source: "user"},
		{Name: "legacy opencode config", Path: filepath.Join(homeDir, ".local", "share", "opencode", "opencode.jsonc"), Source: "legacy"},
		{Name: "claude settings", Path: filepath.Join(paths.Dir(domain.ToolClaude, homeDir), "settings.json"), Source: "user"},
		{Name: "gemini settings", Path: filepath.Join(paths.Dir(domain.ToolGemini, homeDir), "settings.json"), Source: "user"},
		{Name: "codex config", Path: filepath.Join(paths.Dir(domain.ToolCodex, homeDir), "config.toml"), Source: "user"},
	}

	entries := make([]MCPEntry, 0, len(candidates))
	for _, item := range candidates {
		if exists(fs, item.Path) {
			item.Status = "present"
			if item.Details == "" {
				item.Details = filepath.Base(item.Path)
			}
			entries = append(entries, item)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func parseSkillEntry(fs fsx.FS, path string, root string) SkillEntry {
	data, err := fs.ReadFile(path)
	if err != nil {
		return SkillEntry{}
	}

	content := strings.TrimSpace(string(data))
	name := ""
	description := ""

	if strings.HasPrefix(content, "---") {
		if end := strings.Index(content[3:], "\n---"); end >= 0 {
			frontmatter := content[3 : 3+end]
			for _, line := range strings.Split(frontmatter, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				}
				if strings.HasPrefix(line, "description:") {
					description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				}
			}
		}
	}

	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	if description == "" {
		description = firstParagraph(content)
	}
	return SkillEntry{
		Name:        name,
		Description: truncate(description, 140),
		Path:        path,
		Source:      relativeSource(root, path),
	}
}

func listMarkdownFiles(fs fsx.FS, root string, fileName string) ([]string, error) {
	info, err := fs.Stat(root)
	if err != nil {
		return nil, nil
	}
	if !info.IsDir() {
		if filepath.Base(root) == fileName {
			return []string{root}, nil
		}
		return nil, nil
	}

	files := []string{}
	entries, err := fs.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			nested, err := listMarkdownFiles(fs, path, fileName)
			if err != nil {
				return nil, err
			}
			files = append(files, nested...)
			continue
		}
		if entry.Name() == fileName {
			files = append(files, path)
		}
	}
	return files, nil
}

func dedupeSkills(entries []SkillEntry) []SkillEntry {
	seen := map[string]struct{}{}
	out := make([]SkillEntry, 0, len(entries))
	for _, entry := range entries {
		key := strings.ToLower(entry.Name) + "|" + entry.Path
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func exists(fs fsx.FS, path string) bool {
	info, err := fs.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func relativeSource(root, path string) string {
	if root == "" {
		return "project"
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "project"
	}
	return rel
}

func firstParagraph(content string) string {
	for _, block := range strings.Split(content, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, "---") {
			continue
		}
		block = strings.TrimPrefix(block, "#")
		return strings.TrimSpace(block)
	}
	return ""
}

func truncate(value string, n int) string {
	value = strings.TrimSpace(value)
	if len(value) <= n {
		return value
	}
	if n <= 3 {
		return value[:n]
	}
	return value[:n-3] + "..."
}
