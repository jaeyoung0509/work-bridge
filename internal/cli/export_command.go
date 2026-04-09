package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jaeyoung0509/work-bridge/internal/switcher"
)

func (a *App) runExport(cmd *cobra.Command, _ []string) error {
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
	outDir, err := cmd.Flags().GetString("out")
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
	if strings.TrimSpace(outDir) == "" {
		outDir = a.config.Output.ExportDir
	}
	if strings.TrimSpace(outDir) == "" {
		return newExitError(ExitUsage, "--out is required")
	}

	fromTool, err := parseToolValue(fromValue)
	if err != nil {
		return newExitError(ExitUsage, err.Error())
	}
	toTool, err := parseToolValue(toValue)
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
		IncludeSkills: !noSkills,
		IncludeMCP:    !noMCP,
		DryRun:        dryRun,
	}
	if sessionOnly {
		req.IncludeSkills = false
		req.IncludeMCP = false
	}

	result, err := service.Export(cmd.Context(), req, outDir)
	if err != nil {
		return err
	}

	switch a.config.Format {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "text":
		_, err := fmt.Fprint(cmd.OutOrStdout(), renderSwitchText(result, "export"))
		return err
	default:
		return newExitError(ExitUsage, fmt.Sprintf("unsupported format %q (expected text or json)", a.config.Format))
	}
}
