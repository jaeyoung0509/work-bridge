package switcher

import (
	"path/filepath"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/pathpatch"
)

// applyProjectPatches performs project-safe post-apply normalization for the
// managed handoff artifacts written by applyPlan. These patches only touch
// files inside the project or export destination tree.
func (a *projectAdapter) applyProjectPatches(payload domain.SwitchPayload, plan domain.SwitchPlan) []string {
	switch a.target {
	case domain.ToolCodex:
		return a.projectPatchCodex(payload, plan)
	case domain.ToolGemini:
		return a.projectPatchGemini(payload, plan)
	case domain.ToolClaude:
		return a.projectPatchClaude(payload, plan)
	case domain.ToolOpenCode:
		return a.projectPatchOpenCode(payload, plan)
	default:
		return nil
	}
}

// applyNativePatches is kept as a narrow compatibility shim for tests and
// callers that still expect the old name. It now performs project-safe
// artifact patching only.
func (a *projectAdapter) applyNativePatches(payload domain.SwitchPayload, plan domain.SwitchPlan) []string {
	return a.applyProjectPatches(payload, plan)
}

// applyNativeRegistrations performs native-only post-apply steps that touch
// tool home directories or refresh native indexes.
func (a *projectAdapter) applyNativeRegistrations(plan domain.SwitchPlan) []string {
	switch a.target {
	case domain.ToolClaude:
		return a.clearClaudeSessionsIndex(plan.ProjectRoot)
	default:
		return nil
	}
}

func (a *projectAdapter) projectPatchCodex(payload domain.SwitchPayload, plan domain.SwitchPlan) []string {
	srcRoot := payload.Bundle.ProjectRoot
	dstRoot := plan.ProjectRoot
	if srcRoot == "" || srcRoot == dstRoot {
		return nil
	}

	warnings := []string{}
	for _, f := range plan.Session.Files {
		if filepath.Base(f) == "manifest.json" || strings.ToLower(filepath.Ext(f)) != ".jsonl" {
			continue
		}
		data, err := a.fs.ReadFile(f)
		if err != nil {
			warnings = append(warnings, "codex cwd patch: cannot read "+f+": "+err.Error())
			continue
		}
		patched, ok := pathpatch.PatchCodexSessionMetaCWD(data, dstRoot)
		if !ok {
			continue
		}
		patched = pathpatch.PatchJSONLBytes(patched, srcRoot, dstRoot)
		if err := a.fs.WriteFile(f, patched, 0o644); err != nil {
			warnings = append(warnings, "codex cwd patch: cannot write "+f+": "+err.Error())
		}
	}
	return warnings
}

func (a *projectAdapter) projectPatchGemini(payload domain.SwitchPayload, plan domain.SwitchPlan) []string {
	warnings := []string{}

	projectRootFile := filepath.Join(plan.ManagedRoot, ".project_root")
	if err := a.fs.MkdirAll(plan.ManagedRoot, 0o755); err == nil {
		if err := a.fs.WriteFile(projectRootFile, []byte(plan.ProjectRoot+"\n"), 0o644); err != nil {
			warnings = append(warnings, "gemini .project_root patch: "+err.Error())
		}
	}

	srcRoot := payload.Bundle.ProjectRoot
	dstRoot := plan.ProjectRoot
	if srcRoot == "" || srcRoot == dstRoot {
		return warnings
	}
	for _, f := range plan.Session.Files {
		if filepath.Base(f) == "manifest.json" || strings.ToLower(filepath.Ext(f)) != ".json" {
			continue
		}
		data, err := a.fs.ReadFile(f)
		if err != nil {
			continue
		}
		patched, err := pathpatch.PatchJSONBytes(data, srcRoot, dstRoot)
		if err != nil {
			continue
		}
		_ = a.fs.WriteFile(f, patched, 0o644)
	}
	return warnings
}

func (a *projectAdapter) projectPatchClaude(payload domain.SwitchPayload, plan domain.SwitchPlan) []string {
	srcRoot := payload.Bundle.ProjectRoot
	dstRoot := plan.ProjectRoot
	if srcRoot == "" || srcRoot == dstRoot {
		return nil
	}
	for _, f := range plan.Session.Files {
		if filepath.Base(f) == "manifest.json" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f))
		if ext != ".jsonl" && ext != ".json" {
			continue
		}
		data, err := a.fs.ReadFile(f)
		if err != nil {
			continue
		}
		var patched []byte
		if ext == ".jsonl" {
			patched = pathpatch.PatchJSONLBytes(data, srcRoot, dstRoot)
		} else {
			patched, err = pathpatch.PatchJSONBytes(data, srcRoot, dstRoot)
			if err != nil {
				continue
			}
		}
		_ = a.fs.WriteFile(f, patched, 0o644)
	}
	return nil
}

func (a *projectAdapter) projectPatchOpenCode(payload domain.SwitchPayload, plan domain.SwitchPlan) []string {
	srcRoot := payload.Bundle.ProjectRoot
	dstRoot := plan.ProjectRoot
	if srcRoot == "" || srcRoot == dstRoot {
		return nil
	}
	for _, f := range plan.Session.Files {
		if filepath.Base(f) == "manifest.json" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f))
		if ext != ".json" && ext != ".jsonl" && ext != ".jsonc" {
			continue
		}
		data, err := a.fs.ReadFile(f)
		if err != nil {
			continue
		}
		var patched []byte
		switch ext {
		case ".jsonl":
			patched = pathpatch.PatchJSONLBytes(data, srcRoot, dstRoot)
		default:
			patched, err = pathpatch.PatchJSONBytes(data, srcRoot, dstRoot)
			if err != nil {
				continue
			}
		}
		_ = a.fs.WriteFile(f, patched, 0o644)
	}
	return nil
}

func (a *projectAdapter) clearClaudeSessionsIndex(projectRoot string) []string {
	claudeHome := a.toolPaths.Dir(domain.ToolClaude, a.homeDir)
	indexPath := filepath.Join(claudeHome, "projects", pathpatch.ClaudeProjectDirName(projectRoot), "sessions-index.json")
	if _, err := a.fs.Stat(indexPath); err != nil {
		return nil
	}
	if err := a.fs.Remove(indexPath); err != nil {
		return []string{"claude sessions-index removal: " + err.Error()}
	}
	return nil
}
