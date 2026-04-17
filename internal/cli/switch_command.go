package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/stringx"
	"github.com/jaeyoung0509/work-bridge/internal/presentation"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
)

func (a *App) newSwitchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Prepare another tool so you can continue working there.",
		Args:  cobra.NoArgs,
		RunE:  a.runSwitch,
	}
	cmd.Flags().String("from", "", "Source tool: codex, gemini, claude, opencode.")
	cmd.Flags().String("session", "latest", "Source session identifier or latest.")
	cmd.Flags().String("to", "", "Target tool: codex, gemini, claude, opencode.")
	cmd.Flags().String("project", "", "Project root that the resumed work should open in.")
	cmd.Flags().String("mode", string(domain.SwitchModeProject), "Resume mode: project, native.")
	cmd.Flags().Bool("dry-run", false, "Check resume readiness without writing files.")
	cmd.Flags().Bool("no-skills", false, "Skip skills when preparing the resume path.")
	cmd.Flags().Bool("no-mcp", false, "Skip MCP when preparing the resume path.")
	cmd.Flags().Bool("session-only", false, "Carry over session context only.")
	return cmd
}

func (a *App) runSwitch(cmd *cobra.Command, _ []string) error {
	fromValue, err := cmd.Flags().GetString("from")
	if err != nil {
		return err
	}
	toValue, err := cmd.Flags().GetString("to")
	if err != nil {
		return err
	}
	projectRoot, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}
	modeValue, err := cmd.Flags().GetString("mode")
	if err != nil {
		return err
	}
	sessionID, err := cmd.Flags().GetString("session")
	if err != nil {
		return err
	}
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return err
	}
	noSkills, err := cmd.Flags().GetBool("no-skills")
	if err != nil {
		return err
	}
	noMCP, err := cmd.Flags().GetBool("no-mcp")
	if err != nil {
		return err
	}
	sessionOnly, err := cmd.Flags().GetBool("session-only")
	if err != nil {
		return err
	}
	if strings.TrimSpace(projectRoot) == "" {
		return newExitError(ExitUsage, "--project is required")
	}

	fromTool, err := parseToolValue(fromValue)
	if err != nil {
		return newExitError(ExitUsage, err.Error())
	}
	toTool, err := parseToolValue(toValue)
	if err != nil {
		return newExitError(ExitUsage, err.Error())
	}
	mode, err := parseModeValue(modeValue)
	if err != nil {
		return newExitError(ExitUsage, err.Error())
	}

	cwd, homeDir, err := a.resolveWorkingDirs()
	if err != nil {
		return err
	}
	service := switcher.New(switcher.Options{
		FS:        a.fs,
		CWD:       cwd,
		HomeDir:   homeDir,
		ToolPaths: a.config.Paths,
		Redaction: a.config.Redaction,
		LookPath:  a.look,
		Now:       a.clock.Now,
	})
	req := switcher.Request{
		From:          fromTool,
		Session:       sessionID,
		To:            toTool,
		ProjectRoot:   projectRoot,
		Mode:          mode,
		IncludeSkills: !noSkills,
		IncludeMCP:    !noMCP,
		DryRun:        dryRun,
	}
	if sessionOnly {
		req.IncludeSkills = false
		req.IncludeMCP = false
	}

	var result switcher.Result
	if dryRun {
		result, err = service.Preview(cmd.Context(), req)
	} else {
		result, err = service.Apply(cmd.Context(), req)
	}
	if err != nil {
		return err
	}

	switch a.config.Format {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "text":
		mode := "switch"
		if dryRun {
			mode = "preview"
		}
		_, err := fmt.Fprint(cmd.OutOrStdout(), renderSwitchText(result, mode))
		return err
	default:
		return newExitError(ExitUsage, fmt.Sprintf("unsupported format %q (expected text or json)", a.config.Format))
	}
}

func renderSwitchText(result switcher.Result, mode string) string {
	var b strings.Builder

	operation := mode
	if operation != "preview" && operation != "export" {
		operation = "switch"
	}

	guide := presentation.DescribePlan(result.Plan, operation)
	if result.Report != nil {
		guide = presentation.DescribeResult(result.Plan, result.Report, operation)
	}

	fmt.Fprintf(&b, "work-bridge %s\n", mode)
	fmt.Fprintf(&b, "%s\n", guide.Headline)
	fmt.Fprintf(&b, "resume readiness: %s\n", guide.Readiness)
	fmt.Fprintf(&b, "continue from: %s/%s\n", result.Payload.Bundle.SourceTool, result.Payload.Bundle.SourceSessionID)
	fmt.Fprintf(&b, "continue in: %s\n", strings.ToUpper(string(result.Plan.TargetTool)))
	fmt.Fprintf(&b, "resume mode: %s\n", result.Plan.Mode)
	fmt.Fprintf(&b, "project: %s\n", result.Plan.ProjectRoot)
	fmt.Fprintf(&b, "destination: %s\n", result.Plan.DestinationRoot)
	if result.Plan.ManagedRoot != "" {
		fmt.Fprintf(&b, "managed root: %s\n", result.Plan.ManagedRoot)
	}

	renderTextSection(&b, "what carries over", guide.Keeps)
	renderTextSection(&b, "not included", guide.Skips)
	renderTextSection(&b, "manual checks", guide.ManualChecks)
	renderTextSection(&b, "next steps", guide.NextSteps)

	if result.Report != nil {
		files := make([]string, 0, len(result.Report.FilesUpdated))
		for _, file := range result.Report.FilesUpdated {
			files = append(files, "updated: "+file)
		}
		renderTextSection(&b, "prepared files", files)

		backups := make([]string, 0, len(result.Report.BackupsCreated))
		for _, file := range result.Report.BackupsCreated {
			backups = append(backups, file)
		}
		renderTextSection(&b, "backups", backups)
	} else {
		files := make([]string, 0, len(result.Plan.PlannedFiles))
		for _, file := range result.Plan.PlannedFiles {
			files = append(files, fmt.Sprintf("[%s] %s (%s)", file.Section, file.Path, file.Action))
		}
		renderTextSection(&b, "files to prepare", files)
	}
	return b.String()
}

func renderTextSection(b *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s\n", title)
	for _, line := range lines {
		fmt.Fprintf(b, "- %s\n", line)
	}
}

func parseToolValue(value string) (domain.Tool, error) {
	tool := domain.Tool(strings.TrimSpace(strings.ToLower(value)))
	if !tool.IsKnown() {
		return "", fmt.Errorf("unsupported tool %q (expected codex, gemini, claude, or opencode)", value)
	}
	return tool, nil
}

func parseModeValue(value string) (domain.SwitchMode, error) {
	mode := domain.SwitchMode(strings.TrimSpace(strings.ToLower(value)))
	if !mode.IsKnown() {
		return "", fmt.Errorf("unsupported mode %q (expected project or native)", value)
	}
	return mode, nil
}

func dedupeText(values []string) []string {
	return stringx.Dedupe(values)
}
