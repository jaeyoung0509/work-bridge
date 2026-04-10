package switcher

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/pathpatch"
)

// previewNativeGemini provides a plan for Gemini native mode.
func (a *projectAdapter) previewNativeGemini(payload domain.SwitchPayload, projectRoot string, destinationOverride string) (domain.SwitchPlan, error) {
	plan, err := a.previewProject(payload, projectRoot, destinationOverride)
	if err != nil {
		return plan, err
	}
	plan.Mode = domain.SwitchModeNative
	plan.DestinationRoot = a.toolPaths.Dir(domain.ToolGemini, a.homeDir)
	if strings.TrimSpace(destinationOverride) != "" {
		plan.DestinationRoot = destinationOverride
	}
	return plan, nil
}

// applyNativeGemini applies the imported session natively to the Gemini storage.
func (a *projectAdapter) applyNativeGemini(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeNative)

	now := a.now().UTC()
	geminiHome := plan.DestinationRoot

	// read projects.json to determine slug
	projectsPath := filepath.Join(geminiHome, "projects.json")
	var projects map[string]string
	data, err := a.fs.ReadFile(projectsPath)
	if err == nil {
		var pf struct {
			Projects map[string]string `json:"projects"`
		}
		if json.Unmarshal(data, &pf) == nil && pf.Projects != nil {
			projects = pf.Projects
		}
	}
	if projects == nil {
		projects = make(map[string]string)
	}

	slug := pathpatch.GeminiProjectSlug(plan.ProjectRoot, projects)
	if existing, ok := projects[plan.ProjectRoot]; !ok || existing != slug {
		projects[plan.ProjectRoot] = slug
		pf := struct {
			Projects map[string]string `json:"projects"`
		}{Projects: projects}
		if out, err := json.MarshalIndent(pf, "", "  "); err == nil {
			if err := a.fs.MkdirAll(filepath.Dir(projectsPath), 0o755); err == nil {
				_ = a.fs.WriteFile(projectsPath, append(out, '\n'), 0o644)
				report.FilesUpdated = append(report.FilesUpdated, projectsPath)
			}
		}
	}

	projectRootFile := filepath.Join(geminiHome, "tmp", slug, ".project_root")
	if err := a.fs.MkdirAll(filepath.Dir(projectRootFile), 0o755); err == nil {
		if err := a.fs.WriteFile(projectRootFile, []byte(plan.ProjectRoot+"\n"), 0o644); err == nil {
			report.FilesUpdated = append(report.FilesUpdated, projectRootFile)
		}
	}

	chatDir := filepath.Join(geminiHome, "tmp", slug, "chats")
	if err := a.fs.MkdirAll(chatDir, 0o755); err != nil {
		return report, err
	}
	id := payload.Bundle.SourceSessionID
	if id == "" {
		id = fmt.Sprintf("session-%s", now.Format("20060102150405"))
	}
	chatPath := filepath.Join(chatDir, fmt.Sprintf("session-%s.json", now.Format("2006-01-02T15-04-05")))

	// Build chat content with path patching
	chatContent := buildGeminiChat(payload.Bundle, id, now)
	chatContent = pathpatch.ReplacePathsInText(chatContent, payload.Bundle.ProjectRoot, plan.ProjectRoot)

	if err := a.fs.WriteFile(chatPath, []byte(chatContent), 0o644); err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, chatPath)
	report.Session.Files = append(report.Session.Files, chatPath)

	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	return a.applyNativeGlobalArtifacts(payload, report)
}

// exportNativeGemini exports the imported session natively to the Gemini storage layout.
// Note: export does NOT apply global skills/MCP - only apply does that.
func (a *projectAdapter) exportNativeGemini(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeNative)

	now := a.now().UTC()
	geminiHome := filepath.Join(plan.DestinationRoot, ".gemini")

	// read projects.json to determine slug
	projectsPath := filepath.Join(geminiHome, "projects.json")
	var projects map[string]string
	data, err := a.fs.ReadFile(projectsPath)
	if err == nil {
		var pf struct {
			Projects map[string]string `json:"projects"`
		}
		if json.Unmarshal(data, &pf) == nil && pf.Projects != nil {
			projects = pf.Projects
		}
	}
	if projects == nil {
		projects = make(map[string]string)
	}

	slug := pathpatch.GeminiProjectSlug(plan.ProjectRoot, projects)
	if existing, ok := projects[plan.ProjectRoot]; !ok || existing != slug {
		projects[plan.ProjectRoot] = slug
		pf := struct {
			Projects map[string]string `json:"projects"`
		}{Projects: projects}
		if out, err := json.MarshalIndent(pf, "", "  "); err == nil {
			if err := a.fs.MkdirAll(filepath.Dir(projectsPath), 0o755); err == nil {
				_ = a.fs.WriteFile(projectsPath, append(out, '\n'), 0o644)
				report.FilesUpdated = append(report.FilesUpdated, projectsPath)
			}
		}
	}

	projectRootFile := filepath.Join(geminiHome, "tmp", slug, ".project_root")
	if err := a.fs.MkdirAll(filepath.Dir(projectRootFile), 0o755); err == nil {
		if err := a.fs.WriteFile(projectRootFile, []byte(plan.ProjectRoot+"\n"), 0o644); err == nil {
			report.FilesUpdated = append(report.FilesUpdated, projectRootFile)
		}
	}

	chatDir := filepath.Join(geminiHome, "tmp", slug, "chats")
	if err := a.fs.MkdirAll(chatDir, 0o755); err != nil {
		return report, err
	}
	id := payload.Bundle.SourceSessionID
	if id == "" {
		id = fmt.Sprintf("session-%s", now.Format("20060102150405"))
	}
	chatPath := filepath.Join(chatDir, fmt.Sprintf("session-%s.json", now.Format("2006-01-02T15-04-05")))

	// Build chat content with path patching
	chatContent := buildGeminiChat(payload.Bundle, id, now)
	chatContent = pathpatch.ReplacePathsInText(chatContent, payload.Bundle.ProjectRoot, plan.ProjectRoot)

	if err := a.fs.WriteFile(chatPath, []byte(chatContent), 0o644); err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, chatPath)
	report.Session.Files = append(report.Session.Files, chatPath)

	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	return report, nil
}

func buildGeminiChat(bundle domain.SessionBundle, id string, now time.Time) string {
	ts := now.Format(time.RFC3339)
	payload := map[string]any{
		"sessionId":   id,
		"startTime":   ts,
		"lastUpdated": ts,
	}
	var messages []map[string]any
	if bundle.CurrentGoal != "" {
		messages = append(messages, map[string]any{
			"type":      "user",
			"timestamp": ts,
			"content": []map[string]any{
				{
					"text": bundle.CurrentGoal,
				},
			},
		})
	}
	if bundle.Summary != "" {
		messages = append(messages, map[string]any{
			"type":      "gemini",
			"timestamp": ts,
			"content":   bundle.Summary,
		})
	}
	if len(messages) > 0 {
		payload["messages"] = messages
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data) + "\n"
}
