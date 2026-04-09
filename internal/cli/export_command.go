package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"sessionport/internal/doctor"
	"sessionport/internal/domain"
	"sessionport/internal/exporter"
)

func (a *App) runExport(cmd *cobra.Command, _ []string) error {
	bundlePath, err := cmd.Flags().GetString("bundle")
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
		outDir = a.config.Output.ExportDir
	}

	if bundlePath == "" {
		return newExitError(ExitUsage, "--bundle is required")
	}
	if outDir == "" {
		return newExitError(ExitUsage, "--out is required")
	}

	target := domain.Tool(targetValue)
	if !target.IsKnown() {
		return newExitError(ExitUsage, fmt.Sprintf("unsupported target tool %q (expected codex, gemini, or claude)", targetValue))
	}

	data, err := a.fs.ReadFile(bundlePath)
	if err != nil {
		return newExitError(ExitParseFailure, fmt.Sprintf("read bundle %q: %v", bundlePath, err))
	}

	var bundle domain.SessionBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return newExitError(ExitParseFailure, fmt.Sprintf("parse bundle %q: %v", bundlePath, err))
	}
	if bundle.AssetKind == "" {
		bundle.AssetKind = domain.AssetKindSession
	}
	if err := bundle.Validate(); err != nil {
		return newExitError(ExitParseFailure, fmt.Sprintf("validate bundle %q: %v", bundlePath, err))
	}

	report, err := doctor.Analyze(doctor.Options{
		Bundle: bundle,
		Target: target,
	})
	if err != nil {
		return newExitError(ExitExportFailure, err.Error())
	}

	manifest, err := exporter.Export(exporter.Options{
		FS:     a.fs,
		Bundle: bundle,
		Report: report,
		OutDir: outDir,
	})
	if err != nil {
		return newExitError(ExitExportFailure, err.Error())
	}

	switch a.config.Format {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(manifest)
	default:
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Exported %d files to %s\n", len(manifest.Files), outDir)
		return err
	}
}
