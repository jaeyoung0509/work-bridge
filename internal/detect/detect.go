package detect

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sessionport/internal/domain"
	"sessionport/internal/platform/fsx"
)

type Report struct {
	CWD         string       `json:"cwd"`
	ProjectRoot string       `json:"project_root"`
	Tools       []ToolReport `json:"tools"`
}

type ToolReport struct {
	Tool      string          `json:"tool"`
	Installed bool            `json:"installed"`
	Binary    BinaryStatus    `json:"binary"`
	Artifacts []ArtifactProbe `json:"artifacts"`
	Notes     []string        `json:"notes,omitempty"`
}

type BinaryStatus struct {
	Name  string `json:"name"`
	Found bool   `json:"found"`
	Path  string `json:"path,omitempty"`
}

type ArtifactProbe struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Scope string `json:"scope"`
	Path  string `json:"path"`
	Found bool   `json:"found"`
}

type Options struct {
	FS        fsx.FS
	CWD       string
	HomeDir   string
	ToolPaths domain.ToolPaths
	LookPath  func(string) (string, error)
}

func Run(opts Options) (Report, error) {
	if opts.FS == nil {
		return Report{}, errors.New("fs is required")
	}
	if opts.CWD == "" {
		return Report{}, errors.New("cwd is required")
	}
	if opts.HomeDir == "" {
		return Report{}, errors.New("home_dir is required")
	}

	projectRoot := findProjectRoot(opts.FS, opts.CWD)
	ancestors := ancestorDirs(projectRoot, opts.CWD)

	report := Report{
		CWD:         opts.CWD,
		ProjectRoot: projectRoot,
		Tools: []ToolReport{
			detectCodex(opts, ancestors),
			detectGemini(opts, projectRoot, ancestors),
			detectClaude(opts, projectRoot, ancestors),
		},
	}

	return report, nil
}

func detectCodex(opts Options, ancestors []string) ToolReport {
	codexDir := opts.ToolPaths.Dir(domain.ToolCodex, opts.HomeDir)
	artifacts := []ArtifactProbe{
		probeFile(opts.FS, "config.toml", "config", "user", filepath.Join(codexDir, "config.toml")),
		probeFile(opts.FS, "AGENTS.md", "instruction", "user", filepath.Join(codexDir, "AGENTS.md")),
		probeFile(opts.FS, "AGENTS.override.md", "instruction", "user", filepath.Join(codexDir, "AGENTS.override.md")),
	}

	for _, dir := range ancestors {
		artifacts = append(artifacts,
			probeFile(opts.FS, "AGENTS.md", "instruction", "project", filepath.Join(dir, "AGENTS.md")),
			probeFile(opts.FS, "AGENTS.override.md", "instruction", "project", filepath.Join(dir, "AGENTS.override.md")),
		)
	}

	return newToolReport("codex", "codex", opts.LookPath, artifacts, []string{
		"Detects default Codex config and AGENTS files. project_doc_fallback_filenames are not resolved yet.",
	})
}

func detectGemini(opts Options, projectRoot string, ancestors []string) ToolReport {
	geminiDir := opts.ToolPaths.Dir(domain.ToolGemini, opts.HomeDir)
	artifacts := []ArtifactProbe{
		probeFile(opts.FS, "settings.json", "config", "user", filepath.Join(geminiDir, "settings.json")),
		probeFile(opts.FS, "GEMINI.md", "instruction", "user", filepath.Join(geminiDir, "GEMINI.md")),
		probeFile(opts.FS, "settings.json", "config", "project", filepath.Join(projectRoot, ".gemini", "settings.json")),
	}

	for _, dir := range ancestors {
		artifacts = append(artifacts, probeFile(opts.FS, "GEMINI.md", "instruction", "project", filepath.Join(dir, "GEMINI.md")))
	}

	return newToolReport("gemini", "gemini", opts.LookPath, artifacts, []string{
		"Detects default GEMINI.md and settings.json locations only. Custom context.fileName values are not resolved yet.",
	})
}

func detectClaude(opts Options, projectRoot string, ancestors []string) ToolReport {
	claudeDir := opts.ToolPaths.Dir(domain.ToolClaude, opts.HomeDir)
	artifacts := []ArtifactProbe{
		probeFile(opts.FS, "settings.json", "config", "user", filepath.Join(claudeDir, "settings.json")),
		probeFile(opts.FS, "CLAUDE.md", "instruction", "user", filepath.Join(claudeDir, "CLAUDE.md")),
		probeFile(opts.FS, "settings.json", "config", "project", filepath.Join(projectRoot, ".claude", "settings.json")),
		probeFile(opts.FS, "settings.local.json", "config", "local", filepath.Join(projectRoot, ".claude", "settings.local.json")),
	}

	for _, dir := range ancestors {
		artifacts = append(artifacts,
			probeFile(opts.FS, "CLAUDE.md", "instruction", "project", filepath.Join(dir, "CLAUDE.md")),
			probeFile(opts.FS, "CLAUDE.md", "instruction", "project", filepath.Join(dir, ".claude", "CLAUDE.md")),
			probeFile(opts.FS, "CLAUDE.local.md", "instruction", "local", filepath.Join(dir, "CLAUDE.local.md")),
		)
	}

	return newToolReport("claude", "claude", opts.LookPath, artifacts, []string{
		"Detects documented CLAUDE.md and settings.json locations. Managed settings and MCP config files are not included yet.",
	})
}

func newToolReport(tool string, binaryName string, lookPath func(string) (string, error), artifacts []ArtifactProbe, notes []string) ToolReport {
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].Path == artifacts[j].Path {
			if artifacts[i].Scope == artifacts[j].Scope {
				return artifacts[i].Name < artifacts[j].Name
			}
			return artifacts[i].Scope < artifacts[j].Scope
		}
		return artifacts[i].Path < artifacts[j].Path
	})

	binary := BinaryStatus{Name: binaryName}
	if lookPath != nil {
		if path, err := lookPath(binaryName); err == nil && path != "" {
			binary.Found = true
			binary.Path = path
		}
	}

	foundArtifacts := 0
	for _, artifact := range artifacts {
		if artifact.Found {
			foundArtifacts++
		}
	}

	return ToolReport{
		Tool:      tool,
		Installed: binary.Found || foundArtifacts > 0,
		Binary:    binary,
		Artifacts: artifacts,
		Notes:     notes,
	}
}

func probeFile(fs fsx.FS, name string, kind string, scope string, path string) ArtifactProbe {
	return ArtifactProbe{
		Name:  name,
		Kind:  kind,
		Scope: scope,
		Path:  path,
		Found: fileExists(fs, path),
	}
}

func fileExists(fs fsx.FS, path string) bool {
	info, err := fs.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func findProjectRoot(fs fsx.FS, cwd string) string {
	current := filepath.Clean(cwd)
	for {
		if _, err := fs.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(cwd)
		}
		current = parent
	}
}

func ancestorDirs(root string, cwd string) []string {
	root = filepath.Clean(root)
	cwd = filepath.Clean(cwd)

	rel, err := filepath.Rel(root, cwd)
	if err != nil || strings.HasPrefix(rel, "..") {
		return []string{cwd}
	}

	parts := []string{}
	if rel != "." {
		parts = strings.Split(rel, string(os.PathSeparator))
	}

	dirs := make([]string, 0, len(parts)+1)
	current := root
	dirs = append(dirs, current)
	for _, part := range parts {
		current = filepath.Join(current, part)
		dirs = append(dirs, current)
	}
	return dirs
}
