package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"sessionport/internal/importer"
)

func (a *App) runImport(cmd *cobra.Command, _ []string) error {
	from, err := cmd.Flags().GetString("from")
	if err != nil {
		return err
	}
	sessionID, err := cmd.Flags().GetString("session")
	if err != nil {
		return err
	}
	outPath, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}

	switch from {
	case "codex", "gemini":
	case "claude":
		return newExitError(ExitNotImplemented, "claude import is not implemented yet")
	default:
		return newExitError(ExitUsage, fmt.Sprintf("unsupported source tool %q (expected codex, gemini, or claude)", from))
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

	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}

	if outPath == "" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	dir := filepath.Dir(outPath)
	if dir != "" && dir != "." {
		if err := a.fs.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := a.fs.WriteFile(outPath, append(data, '\n'), 0o644); err != nil {
		return err
	}

	switch a.config.Format {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(map[string]any{
			"path":              outPath,
			"source_tool":       bundle.SourceTool,
			"source_session_id": bundle.SourceSessionID,
		})
	default:
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Wrote bundle to %s\n", outPath)
		return err
	}
}
