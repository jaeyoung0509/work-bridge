package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jaeyoung0509/work-bridge/internal/inspect"
)

func (a *App) newInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect <tool>",
		Short: "Inspect a tool's importable sessions and assets.",
		Args:  cobra.ExactArgs(1),
		RunE:  a.runInspect,
	}
	cmd.Flags().Int("limit", 20, "Maximum number of sessions to show.")
	return cmd
}

func (a *App) runInspect(cmd *cobra.Command, args []string) error {
	switch args[0] {
	case "codex", "gemini", "claude", "opencode":
	default:
		return newExitError(ExitUsage, fmt.Sprintf("unsupported tool %q (expected codex, gemini, claude, or opencode)", args[0]))
	}

	cwd, homeDir, err := a.resolveWorkingDirs()
	if err != nil {
		return err
	}

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}

	report, err := inspect.Run(inspect.Options{
		FS:        a.fs,
		CWD:       cwd,
		HomeDir:   homeDir,
		ToolPaths: a.config.Paths,
		Tool:      args[0],
		LookPath:  a.look,
		Limit:     limit,
	})
	if err != nil {
		return err
	}

	switch a.config.Format {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	case "text":
		_, err := fmt.Fprint(cmd.OutOrStdout(), inspect.RenderText(report))
		return err
	default:
		return newExitError(ExitUsage, fmt.Sprintf("unsupported format %q (expected text or json)", a.config.Format))
	}
}
