package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
)

func (a *App) newSwitchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Preview and apply a session handoff into a target tool.",
		Args:  cobra.NoArgs,
		RunE:  a.runSwitch,
	}
	cmd.Flags().String("from", "", "Source tool: codex, gemini, claude, opencode.")
	cmd.Flags().String("session", "latest", "Source session identifier or latest.")
	cmd.Flags().String("to", "", "Target tool: codex, gemini, claude, opencode.")
	cmd.Flags().String("project", "", "Project root to scope the handoff.")
	cmd.Flags().String("mode", string(domain.SwitchModeProject), "Apply mode: project, native.")
	cmd.Flags().Bool("dry-run", false, "Preview managed apply without writing files.")
	cmd.Flags().Bool("no-skills", false, "Skip skills when building the switch payload.")
	cmd.Flags().Bool("no-mcp", false, "Skip MCP when building the switch payload.")
	cmd.Flags().Bool("session-only", false, "Apply session context only.")
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
	fmt.Fprintf(&b, "work-bridge %s\n", mode)
	fmt.Fprintf(&b, "source: %s/%s\n", result.Payload.Bundle.SourceTool, result.Payload.Bundle.SourceSessionID)
	fmt.Fprintf(&b, "target: %s\n", result.Plan.TargetTool)
	fmt.Fprintf(&b, "mode: %s\n", result.Plan.Mode)
	fmt.Fprintf(&b, "project: %s\n", result.Plan.ProjectRoot)
	fmt.Fprintf(&b, "status: %s\n", result.Plan.Status)
	fmt.Fprintf(&b, "destination: %s\n", result.Plan.DestinationRoot)
	if result.Plan.ManagedRoot != "" {
		fmt.Fprintf(&b, "managed root: %s\n", result.Plan.ManagedRoot)
	}
	fmt.Fprintf(&b, "\ncomponents\n")
	fmt.Fprintf(&b, "- session: %s (%s)\n", result.Plan.Session.State, result.Plan.Session.Summary)
	fmt.Fprintf(&b, "- skills: %s (%s)\n", result.Plan.Skills.State, result.Plan.Skills.Summary)
	fmt.Fprintf(&b, "- mcp: %s (%s)\n", result.Plan.MCP.State, result.Plan.MCP.Summary)
	fmt.Fprintf(&b, "\nplanned files\n")
	for _, file := range result.Plan.PlannedFiles {
		fmt.Fprintf(&b, "- [%s] %s (%s)\n", file.Section, file.Path, file.Action)
	}
	if result.Report != nil {
		fmt.Fprintf(&b, "\napply report\n")
		fmt.Fprintf(&b, "- mode: %s\n", result.Report.AppliedMode)
		fmt.Fprintf(&b, "- status: %s\n", result.Report.Status)
		fmt.Fprintf(&b, "- destination: %s\n", result.Report.DestinationRoot)
		for _, file := range result.Report.FilesUpdated {
			fmt.Fprintf(&b, "- updated: %s\n", file)
		}
		for _, file := range result.Report.BackupsCreated {
			fmt.Fprintf(&b, "- backup: %s\n", file)
		}
	}
	warnings := append([]string{}, result.Plan.Warnings...)
	if result.Report != nil {
		warnings = append(warnings, result.Report.Warnings...)
	}
	warnings = dedupeText(warnings)
	if len(warnings) > 0 {
		fmt.Fprintf(&b, "\nwarnings\n")
		for _, warning := range warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
	}
	return b.String()
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
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
