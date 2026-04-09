package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jaeyoung0509/work-bridge/internal/importer"
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
	if outPath == "" {
		outPath = a.config.Output.ImportBundlePath
	}

	switch from {
	case "codex", "gemini", "claude":
	default:
		return newExitError(ExitUsage, fmt.Sprintf("unsupported source tool %q (expected codex, gemini, or claude)", from))
	}

	cwd, homeDir, err := a.resolveWorkingDirs()
	if err != nil {
		return err
	}

	bundle, err := importer.Import(a.importerOptions(cwd, homeDir, from, sessionID))
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
