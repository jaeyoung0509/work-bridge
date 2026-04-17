package presentation

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

type ResumeReadiness string

const (
	ResumeReadinessReady   ResumeReadiness = "READY"
	ResumeReadinessPartial ResumeReadiness = "PARTIAL"
	ResumeReadinessBlocked ResumeReadiness = "BLOCKED"
)

type ContinueGuide struct {
	Readiness    ResumeReadiness
	Headline     string
	Keeps        []string
	Skips        []string
	ManualChecks []string
	NextSteps    []string
}

var quotedTokenPattern = regexp.MustCompile(`"([^"]+)"`)

func RecommendedTarget(source domain.Tool) domain.Tool {
	switch source {
	case domain.ToolCodex:
		return domain.ToolGemini
	case domain.ToolGemini:
		return domain.ToolCodex
	case domain.ToolClaude:
		return domain.ToolCodex
	case domain.ToolOpenCode:
		return domain.ToolCodex
	default:
		for _, candidate := range []domain.Tool{domain.ToolCodex, domain.ToolGemini, domain.ToolClaude, domain.ToolOpenCode} {
			if candidate != source {
				return candidate
			}
		}
		return source
	}
}

func Readiness(state domain.SwitchState) ResumeReadiness {
	switch state {
	case domain.SwitchStateError:
		return ResumeReadinessBlocked
	case domain.SwitchStatePartial:
		return ResumeReadinessPartial
	default:
		return ResumeReadinessReady
	}
}

func ReadinessLabel(state domain.SwitchState) string {
	return string(Readiness(state))
}

func InstructionFile(tool domain.Tool) string {
	switch tool {
	case domain.ToolClaude:
		return "CLAUDE.md"
	case domain.ToolGemini:
		return "GEMINI.md"
	default:
		return "AGENTS.md"
	}
}

func DescribePlan(plan domain.SwitchPlan, operation string) ContinueGuide {
	target := strings.ToUpper(string(plan.TargetTool))
	guide := ContinueGuide{
		Readiness: Readiness(plan.Status),
		Headline:  previewHeadline(Readiness(plan.Status), target, operation),
		Keeps:     planCarryLines(plan),
		Skips:     planSkipLines(plan),
	}

	checks := compatibilityChecks(plan.Compatibility, target)
	checks = append(checks, translateWarnings(plan.Warnings)...)
	guide.ManualChecks = dedupeLines(checks)
	guide.NextSteps = nextPlanSteps(plan, operation)
	return guide
}

func DescribeResult(plan domain.SwitchPlan, report *domain.ApplyReport, operation string) ContinueGuide {
	if report == nil {
		return DescribePlan(plan, operation)
	}

	target := strings.ToUpper(string(plan.TargetTool))
	guide := ContinueGuide{
		Readiness: Readiness(report.Status),
		Headline:  resultHeadline(Readiness(report.Status), target, operation),
		Keeps:     reportCarryLines(report),
		Skips:     reportSkipLines(report),
	}

	checks := compatibilityChecks(plan.Compatibility, target)
	combinedWarnings := append([]string{}, plan.Warnings...)
	combinedWarnings = append(combinedWarnings, report.Warnings...)
	checks = append(checks, translateWarnings(combinedWarnings)...)
	for _, err := range report.Errors {
		if text := strings.TrimSpace(err); text != "" {
			checks = append(checks, "Resolve before continuing: "+text)
		}
	}
	guide.ManualChecks = dedupeLines(checks)
	guide.NextSteps = nextResultSteps(plan, report, operation, guide.Readiness)
	return guide
}

func previewHeadline(readiness ResumeReadiness, target string, operation string) string {
	switch operation {
	case "export":
		switch readiness {
		case ResumeReadinessBlocked:
			return "Fix the blocking issues before exporting this handoff tree."
		case ResumeReadinessPartial:
			return fmt.Sprintf("You can export a handoff tree for %s, but review a few items first.", target)
		default:
			return fmt.Sprintf("Ready to export a handoff tree for %s.", target)
		}
	default:
		switch readiness {
		case ResumeReadinessBlocked:
			return fmt.Sprintf("Fix the blocking issues before preparing %s.", target)
		case ResumeReadinessPartial:
			return fmt.Sprintf("You can continue in %s, but review a few items first.", target)
		default:
			return fmt.Sprintf("Ready to continue in %s.", target)
		}
	}
}

func resultHeadline(readiness ResumeReadiness, target string, operation string) string {
	switch operation {
	case "export":
		switch readiness {
		case ResumeReadinessBlocked:
			return "work-bridge could not finish exporting a usable handoff tree."
		case ResumeReadinessPartial:
			return fmt.Sprintf("Exported the handoff tree for %s, but review a few items first.", target)
		default:
			return fmt.Sprintf("Exported a target-ready handoff tree for %s.", target)
		}
	default:
		switch readiness {
		case ResumeReadinessBlocked:
			return fmt.Sprintf("work-bridge could not fully prepare %s.", target)
		case ResumeReadinessPartial:
			return fmt.Sprintf("%s is prepared, but review a few items before you continue.", target)
		default:
			return fmt.Sprintf("%s is ready for you to continue.", target)
		}
	}
}

func planCarryLines(plan domain.SwitchPlan) []string {
	return componentLines([]componentSummary{
		{label: "Session context", summary: plan.Session.Summary, state: plan.Session.State},
		{label: "Skills", summary: plan.Skills.Summary, state: plan.Skills.State},
		{label: "MCP", summary: plan.MCP.Summary, state: plan.MCP.State},
	}, false)
}

func planSkipLines(plan domain.SwitchPlan) []string {
	return componentLines([]componentSummary{
		{label: "Skills", summary: plan.Skills.Summary, state: plan.Skills.State},
		{label: "MCP", summary: plan.MCP.Summary, state: plan.MCP.State},
	}, true)
}

func reportCarryLines(report *domain.ApplyReport) []string {
	return componentLines([]componentSummary{
		{label: "Session context", summary: report.Session.Summary, state: report.Session.State},
		{label: "Skills", summary: report.Skills.Summary, state: report.Skills.State},
		{label: "MCP", summary: report.MCP.Summary, state: report.MCP.State},
	}, false)
}

func reportSkipLines(report *domain.ApplyReport) []string {
	return componentLines([]componentSummary{
		{label: "Skills", summary: report.Skills.Summary, state: report.Skills.State},
		{label: "MCP", summary: report.MCP.Summary, state: report.MCP.State},
	}, true)
}

type componentSummary struct {
	label   string
	summary string
	state   domain.SwitchState
}

func componentLines(components []componentSummary, wantSkipped bool) []string {
	lines := make([]string, 0, len(components))
	for _, component := range components {
		summary := strings.TrimSpace(component.summary)
		if summary == "" || component.state == domain.SwitchStateError {
			continue
		}
		skipped := isSkippedSummary(summary)
		if skipped != wantSkipped {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", component.label, summary))
	}
	return lines
}

func isSkippedSummary(summary string) bool {
	lower := strings.ToLower(strings.TrimSpace(summary))
	return strings.HasPrefix(lower, "no ")
}

func compatibilityChecks(report domain.CompatibilityReport, target string) []string {
	checks := []string{}
	if len(report.PartialFields) > 0 || len(report.UnsupportedFields) > 0 {
		checks = append(checks, fmt.Sprintf("Some source session details do not map perfectly into %s. Verify the task framing after it opens.", target))
	}
	if len(report.RedactedFields) > 0 {
		checks = append(checks, "Secrets stay redacted by design. Re-add them in the target tool if this workflow needs them.")
	}
	return checks
}

func nextPlanSteps(plan domain.SwitchPlan, operation string) []string {
	target := strings.ToUpper(string(plan.TargetTool))
	instructionFile := InstructionFile(plan.TargetTool)
	switch operation {
	case "export":
		steps := []string{
			"Export to build a target-ready tree in your chosen output directory.",
			fmt.Sprintf("Then review %s before using that tree in %s.", instructionFile, target),
		}
		if root := strings.TrimSpace(plan.ManagedRoot); root != "" {
			steps = append(steps, fmt.Sprintf("The managed context bundle will be included under %s.", root))
		}
		return steps
	default:
		if plan.Mode == domain.SwitchModeNative {
			return []string{
				fmt.Sprintf("Apply to register a native continuation for %s.", target),
				fmt.Sprintf("Then open %s and resume it in %s.", target, plan.ProjectRoot),
			}
		}

		steps := []string{
			fmt.Sprintf("Apply to prepare %s for %s.", instructionFile, target),
			fmt.Sprintf("Then open %s in %s.", target, plan.DestinationRoot),
		}
		if root := strings.TrimSpace(plan.ManagedRoot); root != "" {
			steps = append(steps, fmt.Sprintf("The managed context will be available under %s.", root))
		}
		return steps
	}
}

func nextResultSteps(plan domain.SwitchPlan, report *domain.ApplyReport, operation string, readiness ResumeReadiness) []string {
	target := strings.ToUpper(string(plan.TargetTool))
	instructionFile := InstructionFile(plan.TargetTool)
	destination := firstNonEmpty(report.DestinationRoot, plan.DestinationRoot, plan.ProjectRoot)

	var steps []string
	switch operation {
	case "export":
		steps = []string{
			fmt.Sprintf("Review the exported handoff tree in %s.", destination),
			fmt.Sprintf("Open %s from that tree when you are ready to continue in %s.", instructionFile, target),
		}
	default:
		if report.AppliedMode == string(domain.SwitchModeNative) || plan.Mode == domain.SwitchModeNative {
			steps = []string{
				fmt.Sprintf("Open %s and resume the imported session for %s.", target, destination),
				fmt.Sprintf("Confirm %s opens the expected project before you continue.", target),
			}
		} else {
			steps = []string{
				fmt.Sprintf("Open %s in %s.", target, destination),
				fmt.Sprintf("Start from %s and the managed context work-bridge prepared for this project.", instructionFile),
			}
			if root := strings.TrimSpace(plan.ManagedRoot); root != "" {
				steps = append(steps, fmt.Sprintf("Managed context lives under %s.", root))
			}
		}
	}

	switch readiness {
	case ResumeReadinessPartial:
		steps = append(steps, "Review the manual checks below before trusting the resumed setup.")
	case ResumeReadinessBlocked:
		steps = append(steps, "Resolve the blocking issues, then rerun work-bridge.")
	}

	return steps
}

func translateWarnings(warnings []string) []string {
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		text := strings.TrimSpace(warning)
		if text == "" {
			continue
		}
		lower := strings.ToLower(text)
		switch {
		case strings.Contains(lower, "opencode mcp config does not support cwd"):
			server := firstQuoted(text)
			if server != "" {
				out = append(out, fmt.Sprintf("After switching to OpenCode, verify the working directory for MCP server %s manually.", server))
			} else {
				out = append(out, "After switching to OpenCode, verify MCP server working directories manually.")
			}
		case strings.Contains(lower, "already exists in the target config"):
			server := firstQuoted(text)
			if server != "" {
				out = append(out, fmt.Sprintf("The target already has MCP server %s configured. Review that target-side entry before relying on it.", server))
			} else {
				out = append(out, "The target already has an MCP server with different settings. Review the target-side config before relying on it.")
			}
		case strings.Contains(lower, "declared by multiple source configs"):
			server := firstQuoted(text)
			if server != "" {
				out = append(out, fmt.Sprintf("Multiple source configs define MCP server %s. Verify the version work-bridge kept before you continue.", server))
			} else {
				out = append(out, "Multiple source configs define the same MCP server. Verify which version work-bridge kept before you continue.")
			}
		case strings.Contains(lower, "unsupported config format"):
			out = append(out, "One MCP config could not be translated automatically. Recreate or review it manually in the target tool.")
		case strings.Contains(lower, "missing command or url"):
			out = append(out, "One MCP server definition is incomplete. Fix it manually in the target tool before you rely on it.")
		case strings.Contains(lower, "is not an object"):
			out = append(out, "One MCP server definition has an unexpected shape. Review that config manually before you continue.")
		case strings.HasPrefix(lower, "codex cwd patch:"):
			out = append(out, "work-bridge could not fully patch the native Codex resume path. Confirm the opened project before you continue.")
		case strings.HasPrefix(lower, "gemini .project_root patch:"):
			out = append(out, "work-bridge could not fully patch Gemini project metadata. Confirm the opened project before you continue.")
		default:
			out = append(out, "Review: "+text)
		}
	}
	return out
}

func firstQuoted(text string) string {
	matches := quotedTokenPattern.FindStringSubmatch(text)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func dedupeLines(lines []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func ShortPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return path
	}
	return base
}
