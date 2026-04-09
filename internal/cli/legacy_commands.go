package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) newDetectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "detect",
		Short:  "Detect local installations and project artifacts.",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE:   a.runDetect,
	}
	return cmd
}

func (a *App) newInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "inspect <tool>",
		Short:  "Inspect a tool's importable sessions and assets.",
		Args:   cobra.ExactArgs(1),
		RunE:   a.runInspect,
	}
	cmd.Flags().Int("limit", 20, "Maximum number of sessions to show.")
	return cmd
}

func (a *App) newImportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "import",
		Short:  "Import a session into the canonical bundle format.",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE:   a.runImport,
	}
	cmd.Flags().String("from", "", "Source tool: codex, gemini, claude, opencode.")
	cmd.Flags().String("session", "latest", "Source session identifier or latest.")
	cmd.Flags().String("out", "", "Output path for bundle JSON.")
	return cmd
}

func (a *App) newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "doctor",
		Short:  "Report portability and compatibility for a target tool.",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE:   a.runDoctor,
	}
	cmd.Flags().String("from", "", "Source tool: codex, gemini, claude, opencode.")
	cmd.Flags().String("session", "latest", "Source session identifier or latest.")
	cmd.Flags().String("target", "", "Target tool: codex, gemini, claude, opencode.")
	return cmd
}

func (a *App) newExportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a target-ready handoff without applying to the project.",
		Args:  cobra.NoArgs,
		RunE:  a.runExport,
	}
	cmd.Flags().String("from", "", "Source tool: codex, gemini, claude, opencode.")
	cmd.Flags().String("session", "latest", "Source session identifier or latest.")
	cmd.Flags().String("to", "", "Target tool: codex, gemini, claude, opencode.")
	cmd.Flags().String("project", "", "Project root to scope the export.")
	cmd.Flags().String("mode", "project", "Export mode: project, native.")
	cmd.Flags().String("out", "", "Output directory for exported target-ready files.")
	cmd.Flags().Bool("dry-run", false, "Preview export output without writing files.")
	cmd.Flags().Bool("no-skills", false, "Skip skills when building the export payload.")
	cmd.Flags().Bool("no-mcp", false, "Skip MCP when building the export payload.")
	cmd.Flags().Bool("session-only", false, "Export session context only.")
	return cmd
}

func (a *App) newPackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "pack",
		Short:  "Package a canonical bundle as a portable .spkg archive.",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE:   a.runPack,
	}
	cmd.Flags().String("from", "", "Source tool: codex, gemini, claude, opencode.")
	cmd.Flags().String("session", "latest", "Source session identifier or latest.")
	cmd.Flags().String("out", "", "Output path for the .spkg archive.")
	return cmd
}

func (a *App) newUnpackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "unpack",
		Short:  "Unpack a .spkg archive and prepare target artifacts.",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE:   a.runUnpack,
	}
	cmd.Flags().String("file", "", "Path to a .spkg archive.")
	cmd.Flags().String("target", "", "Target tool: codex, gemini, claude, opencode.")
	cmd.Flags().String("out", "", "Output directory for unpacked contents.")
	return cmd
}

func (a *App) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the build version.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(),
				"work-bridge %s (commit: %s, built: %s)\n",
				Version, Commit, BuildDate,
			)
			return err
		},
	}
}
