package catalog

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
	"github.com/jaeyoung0509/work-bridge/internal/platform/stringx"
)

type SkillEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	RootPath    string   `json:"root_path"`
	EntryPath   string   `json:"entry_path"`
	Path        string   `json:"path"`
	Files       []string `json:"files,omitempty"`
	Source      string   `json:"source"`
	Scope       string   `json:"scope,omitempty"`
	Tool        string   `json:"tool,omitempty"`
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
	repoRoot := nearestProjectRoot(fs, cwd)
	projectDirs := skillDiscoveryWalk(cwd, repoRoot)
	roots := []struct {
		Path   string
		Source string
		Scope  string
		Tool   string
	}{}
	for _, dir := range projectDirs {
		roots = append(roots,
			struct {
				Path   string
				Source string
				Scope  string
				Tool   string
			}{Path: filepath.Join(dir, ".agents", "skills"), Source: "project .agents/skills", Scope: "project"},
			struct {
				Path   string
				Source string
				Scope  string
				Tool   string
			}{Path: filepath.Join(dir, ".gemini", "skills"), Source: "project .gemini/skills", Scope: "project", Tool: "gemini"},
			struct {
				Path   string
				Source string
				Scope  string
				Tool   string
			}{Path: filepath.Join(dir, ".claude", "skills"), Source: "project .claude/skills", Scope: "project", Tool: "claude"},
			struct {
				Path   string
				Source string
				Scope  string
				Tool   string
			}{Path: filepath.Join(dir, ".opencode", "skills"), Source: "project .opencode/skills", Scope: "project", Tool: "opencode"},
		)
	}
	roots = append(roots,
		struct {
			Path   string
			Source string
			Scope  string
			Tool   string
		}{Path: filepath.Join(homeDir, ".agents", "skills"), Source: "user .agents/skills", Scope: "user"},
		struct {
			Path   string
			Source string
			Scope  string
			Tool   string
		}{Path: filepath.Join(homeDir, ".gemini", "skills"), Source: "user .gemini/skills", Scope: "user", Tool: "gemini"},
		struct {
			Path   string
			Source string
			Scope  string
			Tool   string
		}{Path: filepath.Join(homeDir, ".claude", "skills"), Source: "user .claude/skills", Scope: "user", Tool: "claude"},
		struct {
			Path   string
			Source string
			Scope  string
			Tool   string
		}{Path: filepath.Join(homeDir, ".config", "opencode", "skills"), Source: "user opencode skills", Scope: "user", Tool: "opencode"},
		struct {
			Path   string
			Source string
			Scope  string
			Tool   string
		}{Path: filepath.Join(string(filepath.Separator), "etc", "codex", "skills"), Source: "admin codex skills", Scope: "admin", Tool: "codex"},
	)

	entries := []SkillEntry{}
	for _, root := range roots {
		bundles, err := listSkillBundles(fs, root.Path)
		if err != nil {
			return nil, err
		}
		for _, bundle := range bundles {
			entry := parseSkillEntry(fs, bundle, root.Path, root.Source, root.Scope, root.Tool)
			if entry.Name == "" {
				continue
			}
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name == entries[j].Name {
			return entries[i].EntryPath < entries[j].EntryPath
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

type skillBundle struct {
	RootPath  string
	EntryPath string
	Files     []string
}

func parseSkillEntry(fs fsx.FS, bundle skillBundle, root string, source string, scope string, tool string) SkillEntry {
	data, err := fs.ReadFile(bundle.EntryPath)
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
		name = filepath.Base(bundle.RootPath)
	}
	if description == "" {
		description = firstParagraph(content)
	}
	if source == "" {
		source = relativeSource(root, bundle.RootPath)
	}
	return SkillEntry{
		Name:        name,
		Description: truncate(description, 140),
		RootPath:    bundle.RootPath,
		EntryPath:   bundle.EntryPath,
		Path:        bundle.EntryPath,
		Files:       append([]string{}, bundle.Files...),
		Source:      source,
		Scope:       scope,
		Tool:        tool,
	}
}

func listSkillBundles(fs fsx.FS, root string) ([]skillBundle, error) {
	info, err := fs.Stat(root)
	if err != nil {
		return nil, nil
	}
	if !info.IsDir() {
		return nil, nil
	}

	entryPath := filepath.Join(root, "SKILL.md")
	if info, err := fs.Stat(entryPath); err == nil && !info.IsDir() {
		files, err := listBundleFiles(fs, root)
		if err != nil {
			return nil, err
		}
		return []skillBundle{{
			RootPath:  root,
			EntryPath: entryPath,
			Files:     files,
		}}, nil
	}

	bundles := []skillBundle{}
	entries, err := fs.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if !entry.IsDir() {
			continue
		}
		nested, err := listSkillBundles(fs, path)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, nested...)
	}
	return bundles, nil
}

func listBundleFiles(fs fsx.FS, root string) ([]string, error) {
	entries, err := fs.ReadDir(root)
	if err != nil {
		return nil, err
	}
	files := []string{}
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			nested, err := listBundleFiles(fs, path)
			if err != nil {
				return nil, err
			}
			files = append(files, nested...)
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}

func dedupeSkills(entries []SkillEntry) []SkillEntry {
	selected := map[string]SkillEntry{}
	for _, entry := range entries {
		key := strings.ToLower(entry.Scope) + "|" + strings.ToLower(entry.Name)
		current, ok := selected[key]
		if !ok || skillEntryPriority(entry) < skillEntryPriority(current) || (skillEntryPriority(entry) == skillEntryPriority(current) && entry.EntryPath < current.EntryPath) {
			selected[key] = entry
		}
	}
	out := make([]SkillEntry, 0, len(selected))
	for _, entry := range selected {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].EntryPath < out[j].EntryPath
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func skillEntryPriority(entry SkillEntry) int {
	switch {
	case strings.Contains(entry.RootPath, string(filepath.Separator)+".agents"+string(filepath.Separator)+"skills"):
		if entry.Scope == "project" {
			return 0
		}
		return 10
	case strings.Contains(entry.RootPath, string(filepath.Separator)+".gemini"+string(filepath.Separator)+"skills"):
		if entry.Scope == "project" {
			return 1
		}
		return 11
	case strings.Contains(entry.RootPath, string(filepath.Separator)+".claude"+string(filepath.Separator)+"skills"):
		if entry.Scope == "project" {
			return 2
		}
		return 12
	case strings.Contains(entry.RootPath, string(filepath.Separator)+".opencode"+string(filepath.Separator)+"skills"):
		if entry.Scope == "project" {
			return 3
		}
		return 13
	case entry.Scope == "admin":
		return 20
	default:
		return 30
	}
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
		case entry.IsDir() && name == ".agents":
			markers = append(markers, "skills")
		}
	}
	for _, skillPath := range []string{
		filepath.Join(dir, ".agents", "skills"),
		filepath.Join(dir, ".gemini", "skills"),
		filepath.Join(dir, ".claude", "skills"),
		filepath.Join(dir, ".opencode", "skills"),
	} {
		if info, err := fs.Stat(skillPath); err == nil && info.IsDir() {
			markers = append(markers, "skills")
			break
		}
	}
	sort.Strings(markers)
	return stringx.Dedupe(markers)
}

func skillDiscoveryWalk(cwd string, repoRoot string) []string {
	current := filepath.Clean(cwd)
	repoRoot = filepath.Clean(repoRoot)
	roots := []string{}
	seen := map[string]struct{}{}
	for {
		if _, ok := seen[current]; !ok {
			seen[current] = struct{}{}
			roots = append(roots, current)
		}
		if current == repoRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return roots
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
	return stringx.Truncate(value, n)
}
