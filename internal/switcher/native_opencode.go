package switcher

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/pathpatch"
)

const openCodePayloadVersion = "1.3.10"

const (
	openCodeDefaultProviderID = "opencode"
	openCodeDefaultModelID    = "minimax-m2.5-free"
	openCodeDefaultAgent      = "build"
	openCodeDefaultMode       = "build"
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
	return a.applyNativeGlobalArtifacts(payload, report)
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
// The payload stays close to `opencode export <sessionID>` output while
// including compatibility fields that the import validator requires.
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
	sessionID := openCodeSessionID(bundle.SourceSessionID, now)
	title := pathpatchOpenCodeText(bundle.TaskTitle, bundle.ProjectRoot, projectRoot)
	currentGoal := pathpatchOpenCodeText(bundle.CurrentGoal, bundle.ProjectRoot, projectRoot)
	summary := pathpatchOpenCodeText(bundle.Summary, bundle.ProjectRoot, projectRoot)
	model := map[string]any{
		"providerID": openCodeDefaultProviderID,
		"modelID":    openCodeDefaultModelID,
	}
	userMessageID := openCodeObjectID("msg", sessionID, "user")
	assistantMessageID := openCodeObjectID("msg", sessionID, "assistant")
	defaultMessageID := openCodeObjectID("msg", sessionID, "default")

	// Build info block matching opencode export schema
	info := map[string]any{
		"id":        sessionID,
		"slug":      "",
		"projectID": "global",
		"directory": projectRoot,
		"title":     title,
		"version":   openCodePayloadVersion,
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
	if currentGoal != "" {
		msg := map[string]any{
			"info": map[string]any{
				"role":      "user",
				"time":      map[string]any{"created": now.UnixMilli()},
				"agent":     openCodeDefaultAgent,
				"model":     model,
				"summary":   map[string]any{"diffs": []any{}},
				"id":        userMessageID,
				"sessionID": sessionID,
			},
			"parts": []map[string]any{
				{
					"type":      "text",
					"text":      currentGoal,
					"synthetic": false,
					"time":      map[string]any{"start": 0, "end": 0},
					"id":        openCodeObjectID("prt", sessionID, "user-text"),
					"sessionID": sessionID,
					"messageID": userMessageID,
				},
			},
		}
		messages = append(messages, msg)
	}

	// Assistant message with summary
	if summary != "" {
		parts := []map[string]any{
			{
				"type":      "step-start",
				"id":        openCodeObjectID("prt", sessionID, "assistant-step-start"),
				"sessionID": sessionID,
				"messageID": assistantMessageID,
			},
			{
				"type":      "text",
				"text":      summary,
				"time":      map[string]any{"start": now.UnixMilli(), "end": now.UnixMilli()},
				"id":        openCodeObjectID("prt", sessionID, "assistant-text"),
				"sessionID": sessionID,
				"messageID": assistantMessageID,
			},
		}
		msg := map[string]any{
			"info": map[string]any{
				"role":       "assistant",
				"mode":       openCodeDefaultMode,
				"agent":      openCodeDefaultAgent,
				"parentID":   userMessageID,
				"providerID": openCodeDefaultProviderID,
				"modelID":    openCodeDefaultModelID,
				"path": map[string]any{
					"cwd":  projectRoot,
					"root": projectRoot,
				},
				"cost":      0,
				"tokens":    map[string]any{"input": 0, "output": 0, "reasoning": 0, "cache": map[string]any{"read": 0, "write": 0}},
				"finish":    "stop",
				"time":      map[string]any{"created": now.UnixMilli(), "completed": now.UnixMilli()},
				"id":        assistantMessageID,
				"sessionID": sessionID,
			},
			"parts": parts,
		}
		msg["parts"] = append(msg["parts"].([]map[string]any), map[string]any{
			"type":      "step-finish",
			"reason":    "stop",
			"cost":      0,
			"tokens":    map[string]any{"input": 0, "output": 0, "reasoning": 0, "cache": map[string]any{"read": 0, "write": 0}},
			"id":        openCodeObjectID("prt", sessionID, "assistant-step-finish"),
			"sessionID": sessionID,
			"messageID": assistantMessageID,
		})

		messages = append(messages, msg)
	}

	// If no messages, add a minimal user message
	if len(messages) == 0 {
		messages = append(messages, map[string]any{
			"info": map[string]any{
				"role":      "user",
				"time":      map[string]any{"created": now.UnixMilli()},
				"agent":     openCodeDefaultAgent,
				"model":     model,
				"summary":   map[string]any{"diffs": []any{}},
				"id":        defaultMessageID,
				"sessionID": sessionID,
			},
			"parts": []map[string]any{
				{
					"type":      "text",
					"text":      "Imported session via work-bridge",
					"synthetic": false,
					"time":      map[string]any{"start": 0, "end": 0},
					"id":        openCodeObjectID("prt", sessionID, "default-text"),
					"sessionID": sessionID,
					"messageID": defaultMessageID,
				},
			},
		})
	}

	return map[string]any{
		"info":     info,
		"model":    model,
		"messages": messages,
	}
}

// buildOpenCodeExportPayload creates an opencode export-compatible payload for export mode.
// This is identical to import payload but used for out-of-project export.
func buildOpenCodeExportPayload(bundle domain.SessionBundle, projectRoot string, now time.Time) map[string]any {
	return buildOpenCodeImportPayload(bundle, projectRoot, now)
}

func pathpatchOpenCodeText(value, srcPath, dstPath string) string {
	if value == "" || srcPath == "" || srcPath == dstPath {
		return value
	}
	return pathpatch.ReplacePathsInText(value, srcPath, dstPath)
}

func openCodeSessionID(sourceID string, now time.Time) string {
	if strings.HasPrefix(sourceID, "ses_") {
		return sourceID
	}
	seed := sourceID
	if seed == "" {
		seed = now.UTC().Format("20060102150405.000000000")
	}
	return openCodeObjectID("ses", seed, "session")
}

func openCodeObjectID(prefix string, parts ...string) string {
	h := sha1.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	sum := fmt.Sprintf("%x", h.Sum(nil))
	if len(sum) > 24 {
		sum = sum[:24]
	}
	return prefix + "_" + sum
}
