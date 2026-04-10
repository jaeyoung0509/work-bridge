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

// previewNativeClaude provides a plan for Claude native mode.
func (a *projectAdapter) previewNativeClaude(payload domain.SwitchPayload, projectRoot string, destinationOverride string) (domain.SwitchPlan, error) {
	plan, err := a.previewProject(payload, projectRoot, destinationOverride)
	if err != nil {
		return plan, err
	}
	plan.Mode = domain.SwitchModeNative
	plan.DestinationRoot = a.toolPaths.Dir(domain.ToolClaude, a.homeDir)
	if strings.TrimSpace(destinationOverride) != "" {
		plan.DestinationRoot = destinationOverride
	}
	return plan, nil
}

// applyNativeClaude applies the imported session natively to the Claude storage.
func (a *projectAdapter) applyNativeClaude(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeNative)

	now := a.now().UTC()
	claudeHome := plan.DestinationRoot
	encodedDir := pathpatch.ClaudeProjectDirName(plan.ProjectRoot)
	projectDir := filepath.Join(claudeHome, "projects", encodedDir)

	id := payload.Bundle.SourceSessionID
	if id == "" {
		id = fmt.Sprintf("session-%s", now.Format("20060102150405"))
	}
	sessionFilename := id + ".jsonl"
	sessionPath := filepath.Join(projectDir, sessionFilename)

	// Build session content with path patching
	sessionContent := buildClaudeRollout(payload.Bundle, plan.ProjectRoot, now)
	sessionContent = pathpatchClaudePatchPaths(sessionContent, payload.Bundle.ProjectRoot, plan.ProjectRoot)

	if err := a.fs.MkdirAll(projectDir, 0o755); err != nil {
		return report, err
	}
	if err := a.fs.WriteFile(sessionPath, []byte(sessionContent), 0o644); err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, sessionPath)
	report.Session.Files = append(report.Session.Files, sessionPath)

	historyPath := filepath.Join(claudeHome, "history.jsonl")
	title := payload.Bundle.TaskTitle
	if title == "" {
		title = "Imported Claude session"
	}
	indexLine := buildClaudeIndexLine(id, title, plan.ProjectRoot, now)

	existingIndex, _ := a.fs.ReadFile(historyPath)
	newIndex := append(existingIndex, []byte(indexLine+"\n")...)
	if err := a.fs.WriteFile(historyPath, newIndex, 0o644); err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, historyPath)

	report.FilesUpdated = dedupeStrings(report.FilesUpdated)

	// Apply global skills and MCP
	report, _ = a.applyGlobalSkills(payload, report)
	report, _ = a.applyGlobalMCP(payload, report)

	return report, nil
}

// exportNativeClaude exports the imported session natively to the Claude storage layout.
func (a *projectAdapter) exportNativeClaude(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeNative)

	now := a.now().UTC()
	claudeHome := filepath.Join(plan.DestinationRoot, ".claude")
	encodedDir := pathpatch.ClaudeProjectDirName(plan.ProjectRoot)
	projectDir := filepath.Join(claudeHome, "projects", encodedDir)

	id := payload.Bundle.SourceSessionID
	if id == "" {
		id = fmt.Sprintf("session-%s", now.Format("20060102150405"))
	}
	sessionFilename := id + ".jsonl"
	sessionPath := filepath.Join(projectDir, sessionFilename)

	// Build session content with path patching
	sessionContent := buildClaudeRollout(payload.Bundle, plan.ProjectRoot, now)
	sessionContent = pathpatchClaudePatchPaths(sessionContent, payload.Bundle.ProjectRoot, plan.ProjectRoot)

	if err := a.fs.MkdirAll(projectDir, 0o755); err != nil {
		return report, err
	}
	if err := a.fs.WriteFile(sessionPath, []byte(sessionContent), 0o644); err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, sessionPath)
	report.Session.Files = append(report.Session.Files, sessionPath)

	historyPath := filepath.Join(claudeHome, "history.jsonl")
	title := payload.Bundle.TaskTitle
	if title == "" {
		title = "Imported Claude session"
	}
	indexLine := buildClaudeIndexLine(id, title, plan.ProjectRoot, now)

	existingIndex, _ := a.fs.ReadFile(historyPath)
	newIndex := append(existingIndex, []byte(indexLine+"\n")...)
	if err := a.fs.WriteFile(historyPath, newIndex, 0o644); err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, historyPath)

	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	return report, nil
}

func buildClaudeIndexLine(id, title, projectRoot string, now time.Time) string {
	indexRecord := map[string]any{
		"sessionId": id,
		"display":   title,
		"timestamp": now.UnixMilli(),
		"project":   projectRoot,
	}
	data, _ := json.Marshal(indexRecord)
	return string(data)
}

func buildClaudeRollout(bundle domain.SessionBundle, projectRoot string, now time.Time) string {
	var b strings.Builder
	id := bundle.SourceSessionID
	if id == "" {
		id = fmt.Sprintf("session-%s", now.Format("20060102150405"))
	}
	metaPayload := map[string]any{
		"sessionId": id,
		"timestamp": now.UnixMilli(),
		"project":   projectRoot,
	}
	metaData, _ := json.Marshal(metaPayload)
	b.WriteString(string(metaData) + "\n")

	if bundle.Summary != "" {
		msgPayload := map[string]any{
			"type":      "message",
			"text":      bundle.Summary,
			"timestamp": now.UnixMilli(),
		}
		msgData, _ := json.Marshal(msgPayload)
		b.WriteString(string(msgData) + "\n")
	}
	return b.String()
}

// pathpatchClaudePatchPaths replaces source project root paths with target paths
// in Claude session JSONL content. This handles absolute paths in tool results
// and other text content.
func pathpatchClaudePatchPaths(content, srcPath, dstPath string) string {
	if srcPath == "" || srcPath == dstPath {
		return content
	}
	return pathpatch.ReplacePathsInText(content, srcPath, dstPath)
}
