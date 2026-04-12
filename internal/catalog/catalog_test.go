package catalog

import (
	"path/filepath"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
)

func TestScanProjectsFindsProjectsUnderWorkspaceRoots(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workspaceRoot := filepath.Join(root, "workspaces")
	projectA := filepath.Join(workspaceRoot, "alpha")
	projectB := filepath.Join(workspaceRoot, "nested", "beta")

	mkdirAll(t, filepath.Join(projectA, ".git"))
	writeFile(t, filepath.Join(projectA, "AGENTS.md"), "# codex")
	mkdirAll(t, filepath.Join(projectB, ".claude"))
	writeFile(t, filepath.Join(projectB, "CLAUDE.md"), "# claude")
	mkdirAll(t, filepath.Join(workspaceRoot, "node_modules", "ignored", ".git"))

	projects, err := ScanProjects(fsx.OSFS{}, []string{workspaceRoot})
	if err != nil {
		t.Fatalf("scan projects failed: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %#v", projects)
	}

	first := projects[0]
	second := projects[1]
	if first.WorkspaceRoot != workspaceRoot || second.WorkspaceRoot != workspaceRoot {
		t.Fatalf("expected workspace root propagation, got %#v", projects)
	}
	if !containsProject(projects, projectA, "git", "codex") {
		t.Fatalf("expected git/codex markers for alpha, got %#v", projects)
	}
	if !containsProject(projects, projectB, "claude") {
		t.Fatalf("expected claude markers for beta, got %#v", projects)
	}
}

func TestScanSkillsClassifiesScopeAndTool(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo")

	writeFile(t, filepath.Join(cwd, ".agents", "skills", "project-helper", "SKILL.md"), "---\nname: project-helper\ndescription: Project helper\n---\n\n# project-helper")
	writeFile(t, filepath.Join(cwd, ".gemini", "skills", "gemini-helper", "SKILL.md"), "---\nname: gemini-helper\ndescription: Gemini helper\n---\n\n# gemini-helper")
	writeFile(t, filepath.Join(homeDir, ".agents", "skills", "frontend-design", "SKILL.md"), "---\nname: frontend-design\ndescription: Frontend design\n---\n\n# frontend-design")
	writeFile(t, filepath.Join(homeDir, ".claude", "skills", "reviewer", "SKILL.md"), "---\nname: reviewer\ndescription: Review code\n---\n\n# reviewer")
	writeFile(t, filepath.Join(homeDir, ".config", "opencode", "skills", "legacy-helper", "SKILL.md"), "---\nname: legacy-helper\ndescription: OpenCode helper\n---\n\n# legacy-helper")

	skills, err := ScanSkills(fsx.OSFS{}, cwd, homeDir)
	if err != nil {
		t.Fatalf("scan skills failed: %v", err)
	}

	if !containsSkill(skills, "project-helper", "project", "") {
		t.Fatalf("expected project scope skill, got %#v", skills)
	}
	if !containsSkill(skills, "gemini-helper", "project", "gemini") {
		t.Fatalf("expected gemini workspace skill, got %#v", skills)
	}
	if !containsSkill(skills, "frontend-design", "user", "") {
		t.Fatalf("expected generic user skill, got %#v", skills)
	}
	if !containsSkill(skills, "reviewer", "user", "claude") {
		t.Fatalf("expected claude user skill, got %#v", skills)
	}
	if !containsSkill(skills, "legacy-helper", "user", "opencode") {
		t.Fatalf("expected opencode user skill, got %#v", skills)
	}
}

func containsProject(projects []ProjectEntry, root string, markers ...string) bool {
	for _, project := range projects {
		if project.Root != root {
			continue
		}
		for _, marker := range markers {
			if !containsString(project.Markers, marker) {
				return false
			}
		}
		return true
	}
	return false
}

func containsSkill(skills []SkillEntry, name string, scope string, tool string) bool {
	for _, skill := range skills {
		if skill.Name == name && skill.Scope == scope && skill.Tool == tool {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := (fsx.OSFS{}).MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %q failed: %v", path, err)
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := (fsx.OSFS{}).WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %q failed: %v", path, err)
	}
}
