package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"sessionport/internal/detect"
)

func (a *App) runDetect(cmd *cobra.Command, _ []string) error {
	cwd, err := a.getwd()
	if err != nil {
		return fmt.Errorf("resolve current directory: %w", err)
	}

	homeDir, err := a.home()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	report, err := detect.Run(detect.Options{
		FS:       a.fs,
		CWD:      cwd,
		HomeDir:  homeDir,
		LookPath: a.look,
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
		_, err := fmt.Fprint(cmd.OutOrStdout(), detect.RenderText(report))
		return err
	default:
		return newExitError(ExitUsage, fmt.Sprintf("unsupported format %q (expected text or json)", a.config.Format))
	}
}
