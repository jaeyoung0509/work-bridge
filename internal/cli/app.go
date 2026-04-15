package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/platform/archivex"
	"github.com/jaeyoung0509/work-bridge/internal/platform/clockx"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
)

// Version, Commit, and BuildDate are overridden at build time via -ldflags:
//
//	go build -ldflags "-X 'github.com/jaeyoung0509/work-bridge/internal/cli.Version=v0.1.8'"
//
// When built without ldflags (e.g. go run, go test) these default to "dev" / "unknown".
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type Config struct {
	ConfigFile     string   `mapstructure:"config"`
	Format         string   `mapstructure:"format"`
	Verbose        bool     `mapstructure:"verbose"`
	WorkspaceRoots []string `mapstructure:"workspace_roots"`
	Paths          domain.ToolPaths
	Output         OutputConfig
	Redaction      domain.RedactionPolicy
}

type OutputConfig struct {
	ImportBundlePath string `mapstructure:"import_bundle_path"`
	ExportDir        string `mapstructure:"export_dir"`
	PackagePath      string `mapstructure:"package_path"`
	UnpackDir        string `mapstructure:"unpack_dir"`
}

type App struct {
	stdout io.Writer
	stderr io.Writer
	viper  *viper.Viper
	config Config
	fs     fsx.FS
	getwd  func() (string, error)
	home   func() (string, error)
	look   func(string) (string, error)
	clock  clockx.Clock
	zip    archivex.ZIPArchive
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
	v.SetEnvPrefix("WORK_BRIDGE")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()
	v.SetDefault("format", "text")
	v.SetDefault("redaction.detect_sensitive_values", true)

	return &App{
		stdout: stdout,
		stderr: stderr,
		viper:  v,
		config: Config{
			Format: "text",
		},
		fs:    fsx.OSFS{},
		getwd: os.Getwd,
		home:  os.UserHomeDir,
		look:  exec.LookPath,
		clock: clockx.SystemClock{},
		zip:   archivex.ZIPArchive{},
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
			_, _ = fmt.Fprintln(a.stderr, exitErr.Message)
		}
		return exitErr.Code
	}

	message := err.Error()
	_, _ = fmt.Fprintln(a.stderr, message)

	if strings.Contains(message, "unknown command") {
		return ExitUsage
	}

	return ExitRuntime
}

func (a *App) initConfig(cmd *cobra.Command) error {
	configPath := a.viper.GetString("config")
	if configPath != "" {
		if err := a.readConfigFile(configPath); err != nil {
			return err
		}
	} else if candidate, err := a.findDefaultConfigFile(); err != nil {
		return fmt.Errorf("resolve default config: %w", err)
	} else if candidate != "" {
		if err := a.readConfigFile(candidate); err != nil {
			return err
		}
		configPath = candidate
	}

	a.config = Config{
		ConfigFile:     configPath,
		Format:         a.viper.GetString("format"),
		Verbose:        a.viper.GetBool("verbose"),
		WorkspaceRoots: a.viper.GetStringSlice("workspace_roots"),
		Paths: domain.ToolPaths{
			Codex:    a.viper.GetString("paths.codex"),
			Gemini:   a.viper.GetString("paths.gemini"),
			Claude:   a.viper.GetString("paths.claude"),
			OpenCode: a.viper.GetString("paths.opencode"),
		},
		Output: OutputConfig{
			ImportBundlePath: a.viper.GetString("output.import_bundle_path"),
			ExportDir:        a.viper.GetString("output.export_dir"),
			PackagePath:      a.viper.GetString("output.package_path"),
			UnpackDir:        a.viper.GetString("output.unpack_dir"),
		},
		Redaction: domain.RedactionPolicy{
			AdditionalSensitiveKeys: a.viper.GetStringSlice("redaction.additional_sensitive_keys"),
			DetectSensitiveValues:   a.viper.GetBool("redaction.detect_sensitive_values"),
		},
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
		Use:           "work-bridge",
		Short:         "Project-scoped handoff CLI for coding-agent sessions.",
		Long:          "Inspect sessions, switch a project into Claude Code, Gemini CLI, OpenCode, or Codex using explicit project or native mode, or export the same handoff into project-managed or native-store output.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       Version,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return a.initConfig(cmd)
		},
		RunE: a.runRoot,
	}

	root.CompletionOptions.DisableDefaultCmd = true
	root.SetVersionTemplate("work-bridge {{.Version}} (" + Commit + ", built " + BuildDate + ")\n")

	root.PersistentFlags().String("config", "", "Path to a work-bridge config file.")
	root.PersistentFlags().String("format", "text", "Output format. One of: text, json.")
	root.PersistentFlags().Bool("verbose", false, "Enable verbose logging.")
	root.PersistentFlags().StringSlice("workspace-roots", nil, "Workspace roots to scan for projects.")

	_ = a.viper.BindPFlag("config", root.PersistentFlags().Lookup("config"))
	_ = a.viper.BindPFlag("format", root.PersistentFlags().Lookup("format"))
	_ = a.viper.BindPFlag("verbose", root.PersistentFlags().Lookup("verbose"))
	_ = a.viper.BindPFlag("workspace_roots", root.PersistentFlags().Lookup("workspace-roots"))

	root.AddCommand(
		a.newInspectCommand(),
		a.newSwitchCommand(),
		a.newExportCommand(),
		a.newVersionCommand(),
	)
	return root
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
