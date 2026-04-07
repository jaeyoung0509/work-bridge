package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"sessionport/internal/doctor"
	"sessionport/internal/domain"
	"sessionport/internal/importer"
)

func (a *App) runDoctor(cmd *cobra.Command, _ []string) error {
	from, err := cmd.Flags().GetString("from")
	if err != nil {
		return err
	}
	sessionID, err := cmd.Flags().GetString("session")
	if err != nil {
		return err
	}
	targetValue, err := cmd.Flags().GetString("target")
	if err != nil {
		return err
	}

	switch from {
	case "codex", "gemini", "claude":
	default:
		return newExitError(ExitUsage, fmt.Sprintf("unsupported source tool %q (expected codex, gemini, or claude)", from))
	}

	target := domain.Tool(targetValue)
	if !target.IsKnown() {
		return newExitError(ExitUsage, fmt.Sprintf("unsupported target tool %q (expected codex, gemini, or claude)", targetValue))
	}

	cwd, err := a.getwd()
	if err != nil {
		return fmt.Errorf("resolve current directory: %w", err)
	}
	homeDir, err := a.home()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	bundle, err := importer.Import(importer.Options{
		FS:         a.fs,
		CWD:        cwd,
		HomeDir:    homeDir,
		Tool:       from,
		Session:    sessionID,
		ImportedAt: a.clock.Now().Format("2006-01-02T15:04:05Z07:00"),
		LookPath:   a.look,
	})
	if err != nil {
		var notFound *importer.SessionNotFoundError
		if errors.As(err, &notFound) {
			return newExitError(ExitSessionNotFound, notFound.Error())
		}
		return err
	}

	report, err := doctor.Analyze(doctor.Options{
		Bundle: bundle,
		Target: target,
	})
	if err != nil {
		return err
	}

	switch a.config.Format {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	default:
		_, err := fmt.Fprint(cmd.OutOrStdout(), doctor.RenderText(report))
		return err
	}
}
