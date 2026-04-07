package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Version = "dev"

type Config struct {
	ConfigFile string `mapstructure:"config"`
	Format     string `mapstructure:"format"`
	Verbose    bool   `mapstructure:"verbose"`
}

type App struct {
	stdout io.Writer
	stderr io.Writer
	viper  *viper.Viper
	config Config
}

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	app := New(stdout, stderr)
	return app.Run(ctx, args)
}

func New(stdout io.Writer, stderr io.Writer) *App {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	v := viper.New()
	v.SetEnvPrefix("SESSIONPORT")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	return &App{
		stdout: stdout,
		stderr: stderr,
		viper:  v,
		config: Config{
			Format: "text",
		},
	}
}

func (a *App) Run(ctx context.Context, args []string) int {
	root := a.newRootCommand()
	root.SetArgs(args)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)

	if err := root.ExecuteContext(ctx); err != nil {
		return a.handleError(err)
	}

	return ExitOK
}

func (a *App) handleError(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		if exitErr.Message != "" {
			fmt.Fprintln(a.stderr, exitErr.Message)
		}
		return exitErr.Code
	}

	message := err.Error()
	fmt.Fprintln(a.stderr, message)

	if strings.Contains(message, "unknown command") {
		return ExitUsage
	}

	return ExitRuntime
}

func (a *App) initConfig(cmd *cobra.Command) error {
	configPath := a.viper.GetString("config")
	if configPath != "" {
		a.viper.SetConfigFile(configPath)
		if err := a.viper.ReadInConfig(); err != nil {
			return fmt.Errorf("read config: %w", err)
		}
	}

	a.config = Config{
		ConfigFile: a.viper.GetString("config"),
		Format:     a.viper.GetString("format"),
		Verbose:    a.viper.GetBool("verbose"),
	}

	switch a.config.Format {
	case "", "text", "json":
		if a.config.Format == "" {
			a.config.Format = "text"
		}
		return nil
	default:
		return newExitError(ExitUsage, fmt.Sprintf("unsupported format %q (expected text or json)", a.config.Format))
	}
}

func (a *App) newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "sessionport",
		Short:         "Portable working-state CLI for coding-agent sessions.",
		Long:          "Normalize, import, and rehydrate coding-agent sessions across Claude Code, Gemini CLI, and Codex CLI.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       Version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return a.initConfig(cmd)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.CompletionOptions.DisableDefaultCmd = true
	root.SetVersionTemplate("sessionport {{.Version}}\n")

	root.PersistentFlags().String("config", "", "Path to a sessionport config file.")
	root.PersistentFlags().String("format", "text", "Output format. One of: text, json.")
	root.PersistentFlags().Bool("verbose", false, "Enable verbose logging.")

	_ = a.viper.BindPFlag("config", root.PersistentFlags().Lookup("config"))
	_ = a.viper.BindPFlag("format", root.PersistentFlags().Lookup("format"))
	_ = a.viper.BindPFlag("verbose", root.PersistentFlags().Lookup("verbose"))

	root.AddCommand(
		a.newDetectCommand(),
		a.newInspectCommand(),
		a.newImportCommand(),
		a.newDoctorCommand(),
		a.newExportCommand(),
		a.newPackCommand(),
		a.newUnpackCommand(),
		a.newVersionCommand(),
	)

	return root
}

func (a *App) newDetectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Detect local installations and project artifacts.",
		Args:  cobra.NoArgs,
		RunE:  placeholderRunE("detect"),
	}
	return cmd
}

func (a *App) newInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect <tool>",
		Short: "Inspect a tool's importable sessions and assets.",
		Args:  cobra.ExactArgs(1),
		RunE:  placeholderRunE("inspect"),
	}
	return cmd
}

func (a *App) newImportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a session into the canonical bundle format.",
		Args:  cobra.NoArgs,
		RunE:  placeholderRunE("import"),
	}
	cmd.Flags().String("from", "", "Source tool: codex, gemini, claude.")
	cmd.Flags().String("session", "latest", "Source session identifier or latest.")
	cmd.Flags().String("out", "", "Output path for bundle JSON.")
	return cmd
}

func (a *App) newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report portability and compatibility for a target tool.",
		Args:  cobra.NoArgs,
		RunE:  placeholderRunE("doctor"),
	}
	cmd.Flags().String("from", "", "Source tool: codex, gemini, claude.")
	cmd.Flags().String("session", "latest", "Source session identifier or latest.")
	cmd.Flags().String("target", "", "Target tool: codex, gemini, claude.")
	return cmd
}

func (a *App) newExportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Generate target-native starter artifacts from a bundle.",
		Args:  cobra.NoArgs,
		RunE:  placeholderRunE("export"),
	}
	cmd.Flags().String("bundle", "", "Path to a canonical bundle JSON file.")
	cmd.Flags().String("target", "", "Target tool: codex, gemini, claude.")
	cmd.Flags().String("out", "", "Output directory for generated artifacts.")
	return cmd
}

func (a *App) newPackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Package a canonical bundle as a portable .spkg archive.",
		Args:  cobra.NoArgs,
		RunE:  placeholderRunE("pack"),
	}
	cmd.Flags().String("from", "", "Source tool: codex, gemini, claude.")
	cmd.Flags().String("session", "latest", "Source session identifier or latest.")
	cmd.Flags().String("out", "", "Output path for the .spkg archive.")
	return cmd
}

func (a *App) newUnpackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unpack",
		Short: "Unpack a .spkg archive and prepare target artifacts.",
		Args:  cobra.NoArgs,
		RunE:  placeholderRunE("unpack"),
	}
	cmd.Flags().String("file", "", "Path to a .spkg archive.")
	cmd.Flags().String("target", "", "Target tool: codex, gemini, claude.")
	cmd.Flags().String("out", "", "Output directory for unpacked contents.")
	return cmd
}

func (a *App) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the build version.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "sessionport %s\n", Version)
			return err
		},
	}
}
