package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"sessionport/internal/domain"
	"sessionport/internal/importer"
	"sessionport/internal/packagex"
)

func (a *App) runPack(cmd *cobra.Command, _ []string) error {
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
		outPath = a.config.Output.PackagePath
	}
	if outPath == "" {
		return newExitError(ExitUsage, "--out is required")
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

	manifest, err := packagex.Pack(packagex.PackOptions{
		Archive: a.zip,
		Bundle:  bundle,
		OutFile: outPath,
	})
	if err != nil {
		return newExitError(ExitExportFailure, err.Error())
	}

	switch a.config.Format {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(map[string]any{
			"path":              outPath,
			"bundle_id":         manifest.BundleID,
			"source_tool":       manifest.SourceTool,
			"source_session_id": manifest.SourceSessionID,
			"files":             manifest.Files,
		})
	default:
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Packed archive to %s\n", outPath)
		return err
	}
}

func (a *App) runUnpack(cmd *cobra.Command, _ []string) error {
	filePath, err := cmd.Flags().GetString("file")
	if err != nil {
		return err
	}
	targetValue, err := cmd.Flags().GetString("target")
	if err != nil {
		return err
	}
	outDir, err := cmd.Flags().GetString("out")
	if err != nil {
		return err
	}
	if outDir == "" {
		outDir = a.config.Output.UnpackDir
	}
	if filePath == "" {
		return newExitError(ExitUsage, "--file is required")
	}
	if outDir == "" {
		return newExitError(ExitUsage, "--out is required")
	}

	target := domain.Tool(targetValue)
	if !target.IsKnown() {
		return newExitError(ExitUsage, fmt.Sprintf("unsupported target tool %q (expected codex, gemini, or claude)", targetValue))
	}

	result, err := packagex.Unpack(packagex.UnpackOptions{
		Archive: a.zip,
		FS:      a.fs,
		File:    filePath,
		Target:  target,
		OutDir:  outDir,
	})
	if err != nil {
		return newExitError(ExitParseFailure, err.Error())
	}

	switch a.config.Format {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	default:
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Unpacked archive to %s\n", outDir)
		return err
	}
}
