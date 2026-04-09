package catalog

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
)

type SkillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Source      string `json:"source"`
	Scope       string `json:"scope,omitempty"`
	Tool        string `json:"tool,omitempty"`
}

type MCPEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Source  string `json:"source"`
	Status  string `json:"status"`
	Details string `json:"details"`
}

type ProjectEntry struct {
	Name          string   `json:"name"`
	Root          string   `json:"root"`
	WorkspaceRoot string   `json:"workspace_root"`
	Markers       []string `json:"markers"`
}

func ScanSkills(fs fsx.FS, cwd, homeDir string) ([]SkillEntry, error) {
	roots := []struct {
		Path   string
		Source string
		Scope  string
		Tool   string
	}{
		{Path: filepath.Join(cwd, ".github", "skills"), Source: "project .github/skills", Scope: "project"},
		{Path: filepath.Join(cwd, "skills"), Source: "project skills", Scope: "project"},
		{Path: filepath.Join(homeDir, ".codex", "skills"), Source: "codex user", Scope: "user", Tool: "codex"},
		{Path: filepath.Join(homeDir, ".claude", "skills"), Source: "claude user", Scope: "user", Tool: "claude"},
		{Path: filepath.Join(homeDir, ".config", "opencode", "skills"), Source: "opencode user", Scope: "user", Tool: "opencode"},
		{Path: filepath.Join(homeDir, ".local", "share", "opencode", "skills"), Source: "opencode global", Scope: "global", Tool: "opencode"},
	}

	entries := []SkillEntry{}
	for _, root := range roots {
		files, err := listMarkdownFiles(fs, root.Path, "SKILL.md")
		if err != nil {
			return nil, err
		}
		for _, path := range files {
			entry := parseSkillEntry(fs, path, root.Path, root.Source, root.Scope, root.Tool)
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
	projectRoot := nearestProjectRoot(fs, cwd)
	candidates := []MCPEntry{}
	addCandidate := func(name string, path string, source string) {
		if path == "" {
			return
		}
		candidates = append(candidates, MCPEntry{Name: name, Path: filepath.Clean(path), Source: source})
	}

	addCandidate("project claude settings", filepath.Join(projectRoot, ".claude", "settings.json"), "project")
	addCandidate("project claude local settings", filepath.Join(projectRoot, ".claude", "settings.local.json"), "local")
	addCandidate("project gemini settings", filepath.Join(projectRoot, ".gemini", "settings.json"), "project")
	addCandidate("project opencode config", filepath.Join(projectRoot, ".opencode", "opencode.jsonc"), "project")
	addCandidate("project opencode config", filepath.Join(projectRoot, ".opencode", "opencode.json"), "project")
	addCandidate("global codex config", filepath.Join(paths.Dir(domain.ToolCodex, homeDir), "config.toml"), "user")
	addCandidate("global claude settings", filepath.Join(paths.Dir(domain.ToolClaude, homeDir), "settings.json"), "user")
	addCandidate("global gemini settings", filepath.Join(paths.Dir(domain.ToolGemini, homeDir), "settings.json"), "user")
	addCandidate("global opencode config", filepath.Join(homeDir, ".config", "opencode", "opencode.jsonc"), "user")
	addCandidate("global opencode config", filepath.Join(homeDir, ".config", "opencode", "opencode.json"), "user")
	addCandidate("legacy opencode config", filepath.Join(homeDir, ".local", "share", "opencode", "opencode.jsonc"), "legacy")
	addCandidate("legacy opencode config", filepath.Join(homeDir, ".local", "share", "opencode", "opencode.json"), "legacy")

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

func ScanProjects(fs fsx.FS, roots []string) ([]ProjectEntry, error) {
	normalized := normalizeProjectRoots(fs, roots)
	entries := []ProjectEntry{}
	seen := map[string]struct{}{}
	for _, root := range normalized {
		if err := walkProjects(fs, root, root, 0, seen, &entries); err != nil {
			return nil, err
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			return entries[i].Root < entries[j].Root
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func parseSkillEntry(fs fsx.FS, path string, root string, source string, scope string, tool string) SkillEntry {
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
	if source == "" {
		source = relativeSource(root, path)
	}
	return SkillEntry{
		Name:        name,
		Description: truncate(description, 140),
		Path:        path,
		Source:      source,
		Scope:       scope,
		Tool:        tool,
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

func normalizeProjectRoots(fs fsx.FS, roots []string) []string {
	out := make([]string, 0, len(roots))
	seen := map[string]struct{}{}
	for _, root := range roots {
		root = filepath.Clean(root)
		if root == "" {
			continue
		}
		if _, ok := seen[root]; ok {
			continue
		}
		info, err := fs.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[root] = struct{}{}
		out = append(out, root)
	}
	sort.Strings(out)
	return out
}

func walkProjects(fs fsx.FS, dir string, workspaceRoot string, depth int, seen map[string]struct{}, out *[]ProjectEntry) error {
	if depth > 5 {
		return nil
	}

	entries, err := fs.ReadDir(dir)
	if err != nil {
		return nil
	}

	markers := projectMarkers(fs, dir, entries)
	if len(markers) > 0 {
		root := filepath.Clean(dir)
		if _, ok := seen[root]; !ok {
			seen[root] = struct{}{}
			*out = append(*out, ProjectEntry{
				Name:          filepath.Base(root),
				Root:          root,
				WorkspaceRoot: filepath.Clean(workspaceRoot),
				Markers:       markers,
			})
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if shouldSkipProjectWalkDir(entry.Name()) {
			continue
		}
		if err := walkProjects(fs, filepath.Join(dir, entry.Name()), workspaceRoot, depth+1, seen, out); err != nil {
			return err
		}
	}
	return nil
}

func projectMarkers(fs fsx.FS, dir string, entries []fs.DirEntry) []string {
	markers := []string{}
	for _, entry := range entries {
		name := entry.Name()
		switch {
		case entry.IsDir() && name == ".git":
			markers = append(markers, "git")
		case !entry.IsDir() && name == "AGENTS.md":
			markers = append(markers, "codex")
		case !entry.IsDir() && name == "GEMINI.md":
			markers = append(markers, "gemini")
		case !entry.IsDir() && name == "CLAUDE.md":
			markers = append(markers, "claude")
		case entry.IsDir() && name == ".claude":
			markers = append(markers, "claude")
		case entry.IsDir() && name == ".gemini":
			markers = append(markers, "gemini")
		case entry.IsDir() && name == ".opencode":
			markers = append(markers, "opencode")
		case entry.IsDir() && name == "skills":
			markers = append(markers, "skills")
		}
	}
	if info, err := fs.Stat(filepath.Join(dir, ".github", "skills")); err == nil && info.IsDir() {
		markers = append(markers, "skills")
	}
	sort.Strings(markers)
	return dedupeStrings(markers)
}

func shouldSkipProjectWalkDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", ".jj", "node_modules", "vendor", "dist", "build", "target", ".next", ".turbo", ".cache":
		return true
	default:
		return strings.HasPrefix(name, ".work-bridge")
	}
}

func nearestProjectRoot(fs fsx.FS, cwd string) string {
	current := filepath.Clean(cwd)
	for {
		if info, err := fs.Stat(filepath.Join(current, ".git")); err == nil && info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(cwd)
		}
		current = parent
	}
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
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
