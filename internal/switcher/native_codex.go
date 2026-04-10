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

// previewNativeCodex provides a plan for Codex native mode.
func (a *projectAdapter) previewNativeCodex(payload domain.SwitchPayload, projectRoot string, destinationOverride string) (domain.SwitchPlan, error) {
	plan, err := a.previewProject(payload, projectRoot, destinationOverride)
	if err != nil {
		return plan, err
	}
	plan.Mode = domain.SwitchModeNative
	plan.DestinationRoot = a.toolPaths.Dir(domain.ToolCodex, a.homeDir)
	if strings.TrimSpace(destinationOverride) != "" {
		plan.DestinationRoot = destinationOverride
	}
	return plan, nil
}

// applyNativeCodex applies the imported session natively to the Codex storage.
func (a *projectAdapter) applyNativeCodex(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeNative)

	now := a.now().UTC()
	codexHome := plan.DestinationRoot
	codexSessionsDir := filepath.Join(codexHome, "sessions", fmt.Sprintf("%04d", now.Year()), fmt.Sprintf("%02d", now.Month()), fmt.Sprintf("%02d", now.Day()))
	rolloutFilename := fmt.Sprintf("rollout-%s.jsonl", payload.Bundle.SourceSessionID)
	if payload.Bundle.SourceSessionID == "" {
		rolloutFilename = fmt.Sprintf("rollout-%s.jsonl", now.Format("2006-01-02T15-04-05"))
	}
	rolloutPath := filepath.Join(codexSessionsDir, rolloutFilename)

	// Build rollout content with path patching
	rolloutContent := buildCodexRollout(payload.Bundle, plan.ProjectRoot, now)
	rolloutContent = pathpatch.ReplacePathsInText(rolloutContent, payload.Bundle.ProjectRoot, plan.ProjectRoot)

	if err := a.fs.MkdirAll(codexSessionsDir, 0o755); err != nil {
		return report, err
	}
	if err := a.fs.WriteFile(rolloutPath, []byte(rolloutContent), 0o644); err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, rolloutPath)
	report.Session.Files = append(report.Session.Files, rolloutPath)

	indexPath := filepath.Join(codexHome, "session_index.jsonl")
	id := payload.Bundle.SourceSessionID
	if id == "" {
		id = fmt.Sprintf("session-%s", now.Format("20060102150405"))
	}
	title := payload.Bundle.TaskTitle
	if title == "" {
		title = "Imported Codex session"
	}
	indexLine := buildCodexIndexLine(id, title, now)

	existingIndex, _ := a.fs.ReadFile(indexPath)
	newIndex := append(existingIndex, []byte(indexLine+"\n")...)
	if err := a.fs.WriteFile(indexPath, newIndex, 0o644); err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, indexPath)

	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	return a.applyNativeGlobalArtifacts(payload, report)
}

// exportNativeCodex exports the imported session natively to the Codex storage layout.
func (a *projectAdapter) exportNativeCodex(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeNative)

	now := a.now().UTC()
	codexHome := filepath.Join(plan.DestinationRoot, ".codex")
	codexSessionsDir := filepath.Join(codexHome, "sessions", fmt.Sprintf("%04d", now.Year()), fmt.Sprintf("%02d", now.Month()), fmt.Sprintf("%02d", now.Day()))
	rolloutFilename := fmt.Sprintf("rollout-%s.jsonl", payload.Bundle.SourceSessionID)
	if payload.Bundle.SourceSessionID == "" {
		rolloutFilename = fmt.Sprintf("rollout-%s.jsonl", now.Format("2006-01-02T15-04-05"))
	}
	rolloutPath := filepath.Join(codexSessionsDir, rolloutFilename)

	// Build rollout content with path patching
	rolloutContent := buildCodexRollout(payload.Bundle, plan.ProjectRoot, now)
	rolloutContent = pathpatch.ReplacePathsInText(rolloutContent, payload.Bundle.ProjectRoot, plan.ProjectRoot)

	if err := a.fs.MkdirAll(codexSessionsDir, 0o755); err != nil {
		return report, err
	}
	if err := a.fs.WriteFile(rolloutPath, []byte(rolloutContent), 0o644); err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, rolloutPath)
	report.Session.Files = append(report.Session.Files, rolloutPath)

	indexPath := filepath.Join(codexHome, "session_index.jsonl")
	id := payload.Bundle.SourceSessionID
	if id == "" {
		id = fmt.Sprintf("session-%s", now.Format("20060102150405"))
	}
	title := payload.Bundle.TaskTitle
	if title == "" {
		title = "Imported Codex session"
	}
	indexLine := buildCodexIndexLine(id, title, now)

	existingIndex, _ := a.fs.ReadFile(indexPath)
	newIndex := append(existingIndex, []byte(indexLine+"\n")...)
	if err := a.fs.WriteFile(indexPath, newIndex, 0o644); err != nil {
		return report, err
	}
	report.FilesUpdated = append(report.FilesUpdated, indexPath)

	report.FilesUpdated = dedupeStrings(report.FilesUpdated)
	return report, nil
}

func buildCodexIndexLine(id, title string, now time.Time) string {
	indexRecord := map[string]any{
		"id":          id,
		"thread_name": title,
		"updated_at":  now.Format(time.RFC3339),
	}
	data, _ := json.Marshal(indexRecord)
	return string(data)
}

func buildCodexRollout(bundle domain.SessionBundle, projectRoot string, now time.Time) string {
	var b strings.Builder
	id := bundle.SourceSessionID
	if id == "" {
		id = fmt.Sprintf("session-%s", now.Format("20060102150405"))
	}
	metaPayload := map[string]any{
		"id":        id,
		"timestamp": now.Format(time.RFC3339),
		"cwd":       projectRoot,
	}
	metaData, _ := json.Marshal(metaPayload)
	metaLine := map[string]any{
		"timestamp": now.Format(time.RFC3339),
		"type":      "session_meta",
		"payload":   json.RawMessage(metaData),
	}
	metaLineData, _ := json.Marshal(metaLine)
	b.WriteString(string(metaLineData) + "\n")

	if bundle.Summary != "" {
		msgPayload := map[string]any{
			"type":    "agent_message",
			"message": bundle.Summary,
		}
		msgData, _ := json.Marshal(msgPayload)
		msgLine := map[string]any{
			"timestamp": now.Format(time.RFC3339),
			"type":      "event_msg",
			"payload":   json.RawMessage(msgData),
		}
		msgLineData, _ := json.Marshal(msgLine)
		b.WriteString(string(msgLineData) + "\n")
	}
	return b.String()
}
