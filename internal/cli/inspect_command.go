package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"sessionport/internal/inspect"
)

func (a *App) runInspect(cmd *cobra.Command, args []string) error {
	switch args[0] {
	case "codex", "gemini", "claude":
	default:
		return newExitError(ExitUsage, fmt.Sprintf("unsupported tool %q (expected codex, gemini, or claude)", args[0]))
	}

	cwd, err := a.getwd()
	if err != nil {
		return fmt.Errorf("resolve current directory: %w", err)
	}

	homeDir, err := a.home()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}

	report, err := inspect.Run(inspect.Options{
		FS:       a.fs,
		CWD:      cwd,
		HomeDir:  homeDir,
		Tool:     args[0],
		LookPath: a.look,
		Limit:    limit,
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
