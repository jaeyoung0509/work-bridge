package switcher

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

// previewNativeOpenCode provides a plan for OpenCode native mode.
func (a *projectAdapter) previewNativeOpenCode(payload domain.SwitchPayload, projectRoot string, destinationOverride string) (domain.SwitchPlan, error) {
	plan, err := a.previewProject(payload, projectRoot, destinationOverride)
	if err != nil {
		return plan, err
	}
	plan.Mode = domain.SwitchModeNative
	plan.DestinationRoot = a.toolPaths.Dir(domain.ToolOpenCode, a.homeDir)
	if strings.TrimSpace(destinationOverride) != "" {
		plan.DestinationRoot = destinationOverride
	}
	return plan, nil
}

// applyNativeOpenCode writes a staged JSON file in opencode export-compatible format
// and invokes `opencode import <file>`.
func (a *projectAdapter) applyNativeOpenCode(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeNative)

	if _, err := exec.LookPath("opencode"); err != nil {
		return report, fmt.Errorf("opencode CLI is not installed or not in PATH: %w", err)
	}

	// Build opencode-compatible import payload
	now := a.now().UTC()
	importPayload := buildOpenCodeImportPayload(payload.Bundle, plan.ProjectRoot, now)

	tempFile := filepath.Join(plan.ProjectRoot, ".opencode_staged.json")
	data, err := json.MarshalIndent(importPayload, "", "  ")
	if err != nil {
		return report, err
	}

	if err := a.fs.WriteFile(tempFile, data, 0o600); err != nil {
		return report, err
	}
	defer a.fs.Remove(tempFile) // Cleanup

	cmd := exec.Command("opencode", "import", tempFile)
	cmd.Dir = plan.ProjectRoot
	if err := cmd.Run(); err != nil {
		return report, fmt.Errorf("opencode import failed: %w", err)
	}

	report.Warnings = append(report.Warnings, "OpenCode session applied via CLI delegate.")

	// Apply global skills and MCP
	report, _ = a.applyGlobalSkills(payload, report)
	report, _ = a.applyGlobalMCP(payload, report)

	return report, nil
}

// exportNativeOpenCode writes the opencode export-compatible payload natively.
func (a *projectAdapter) exportNativeOpenCode(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	report, err := a.applyPlan(payload, plan)
	if err != nil {
		return report, err
	}
	report.AppliedMode = string(domain.SwitchModeNative)

	now := a.now().UTC()
	exportPayload := buildOpenCodeExportPayload(payload.Bundle, plan.ProjectRoot, now)

	exportPath := filepath.Join(plan.DestinationRoot, ".opencode_export.json")
	data, err := json.MarshalIndent(exportPayload, "", "  ")
	if err != nil {
		return report, err
	}

	if err := a.fs.MkdirAll(plan.DestinationRoot, 0o755); err != nil {
		return report, err
	}
	if err := a.fs.WriteFile(exportPath, data, 0o644); err != nil {
		return report, err
	}

	report.FilesUpdated = append(report.FilesUpdated, exportPath)
	report.Warnings = append(report.Warnings, "OpenCode delegate payload exported. Run: opencode import <file>")
	return report, nil
}

// buildOpenCodeImportPayload creates an opencode import-compatible payload from a SessionBundle.
// Format matches `opencode export <sessionID>` output:
//
//	{
//	  "info": { ... },
//	  "messages": [
//	    {
//	      "info": { "role": "user"|"assistant", ... },
//	      "parts": [ { "type": "text", "text": "..." }, ... ]
//	    }
//	  ]
//	}
func buildOpenCodeImportPayload(bundle domain.SessionBundle, projectRoot string, now time.Time) map[string]any {
	sessionID := bundle.SourceSessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("imported-%s", now.Format("20060102150405"))
	}

	// Build info block matching opencode export schema
	info := map[string]any{
		"id":        sessionID,
		"slug":      "imported-session",
		"projectID": "imported",
		"directory": projectRoot,
		"title":     bundle.TaskTitle,
		"version":   "1.3.10",
		"summary": map[string]any{
			"additions": 0,
			"deletions": 0,
			"files":     0,
		},
		"time": map[string]any{
			"created": now.UnixMilli(),
			"updated": now.UnixMilli(),
		},
	}

	// Build messages from bundle data
	var messages []map[string]any

	// User message with current goal
	if bundle.CurrentGoal != "" {
		msg := map[string]any{
			"info": map[string]any{
				"role":      "user",
				"time":      map[string]any{"created": now.UnixMilli()},
				"agent":     "build",
				"id":        fmt.Sprintf("msg-user-%s", sessionID),
				"sessionID": sessionID,
			},
			"parts": []map[string]any{
				{
					"type": "text",
					"text": bundle.CurrentGoal,
					"id":   fmt.Sprintf("prt-user-%s", sessionID),
				},
			},
		}
		messages = append(messages, msg)
	}

	// Assistant message with summary
	if bundle.Summary != "" {
		msg := map[string]any{
			"info": map[string]any{
				"role":  "assistant",
				"mode":  "build",
				"agent": "build",
				"path": map[string]any{
					"cwd":  projectRoot,
					"root": projectRoot,
				},
				"id":        fmt.Sprintf("msg-assistant-%s", sessionID),
				"sessionID": sessionID,
			},
			"parts": []map[string]any{
				{
					"type": "text",
					"text": bundle.Summary,
					"id":   fmt.Sprintf("prt-assistant-%s", sessionID),
				},
			},
		}

		// Add tool events if available
		if len(bundle.ToolEvents) > 0 {
			parts := msg["parts"].([]map[string]any)
			for i, event := range bundle.ToolEvents {
				toolPart := map[string]any{
					"type": "tool_use",
					"name": event.Type,
					"id":   fmt.Sprintf("prt-tool-%d-%s", i, sessionID),
				}
				if event.Summary != "" {
					toolPart["output"] = event.Summary
				}
				if event.Status != "" {
					toolPart["status"] = event.Status
				}
				parts = append(parts, toolPart)
			}
			msg["parts"] = parts
		}

		messages = append(messages, msg)
	}

	// If no messages, add a minimal user message
	if len(messages) == 0 {
		messages = append(messages, map[string]any{
			"info": map[string]any{
				"role":      "user",
				"time":      map[string]any{"created": now.UnixMilli()},
				"agent":     "build",
				"id":        fmt.Sprintf("msg-default-%s", sessionID),
				"sessionID": sessionID,
			},
			"parts": []map[string]any{
				{
					"type": "text",
					"text": "Imported session via work-bridge",
					"id":   fmt.Sprintf("prt-default-%s", sessionID),
				},
			},
		})
	}

	return map[string]any{
		"info":     info,
		"messages": messages,
	}
}

// buildOpenCodeExportPayload creates an opencode export-compatible payload for export mode.
// This is identical to import payload but used for out-of-project export.
func buildOpenCodeExportPayload(bundle domain.SessionBundle, projectRoot string, now time.Time) map[string]any {
	return buildOpenCodeImportPayload(bundle, projectRoot, now)
}
