package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/jaeyoung0509/work-bridge/internal/detect"
	"github.com/jaeyoung0509/work-bridge/internal/switcher"
	"github.com/jaeyoung0509/work-bridge/internal/switchui"
	"github.com/jaeyoung0509/work-bridge/internal/ui"
	tea "charm.land/bubbletea/v2"
)

func (a *App) runRoot(cmd *cobra.Command, _ []string) error {
	if !shouldLaunchTUI(a.stdout, a.stderr) {
		return cmd.Help()
	}

	cwd, homeDir, err := a.resolveWorkingDirs()
	if err != nil {
		return err
	}
	service := switcher.New(switcher.Options{
		FS:        a.fs,
		CWD:       cwd,
		HomeDir:   homeDir,
		ToolPaths: a.config.Paths,
		Redaction: a.config.Redaction,
		LookPath:  a.look,
		Now:       a.clock.Now,
	})

	if a.config.V2 {
		p := tea.NewProgram(ui.NewMainModel(cmd.Context(), service))
		_, err := p.Run()
		return err
	}

	backend := switchui.Backend{
		LoadWorkspace: service.LoadWorkspace,
		Preview:       service.Preview,
		Apply:         service.Apply,
		Export:        service.Export,
	}
	return switchui.Run(cmd.Context(), backend, os.Stdout, os.Stderr)
}

func (a *App) runHeadlessOverview(out io.Writer) error {
	cwd, homeDir, err := a.resolveWorkingDirs()
	if err != nil {
		return err
	}

	detectReport, err := detect.Run(detect.Options{
		FS:        a.fs,
		CWD:       cwd,
		HomeDir:   homeDir,
		ToolPaths: a.config.Paths,
		LookPath:  a.look,
	})
	if err != nil {
		return err
	}

	summary := map[string]any{
		"cwd":          detectReport.CWD,
		"project_root": detectReport.ProjectRoot,
		"tools":        detectReport.Tools,
		"mode":         "headless",
	}

	switch a.config.Format {
	case "json":
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	default:
		_, err := fmt.Fprintf(out, "work-bridge (headless)\nproject: %s\ncwd: %s\ntools: %d\n", detectReport.ProjectRoot, detectReport.CWD, len(detectReport.Tools))
		return err
	}
}

func (a *App) detectWorkspace(ctx context.Context) (detect.Report, error) {
	cwd, homeDir, err := a.resolveWorkingDirs()
	if err != nil {
		return detect.Report{}, err
	}
	return detect.Run(detect.Options{
		FS:        a.fs,
		CWD:       cwd,
		HomeDir:   homeDir,
		ToolPaths: a.config.Paths,
		LookPath:  a.look,
	})
}

func isInteractiveTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func shouldLaunchTUI(stdout, stderr io.Writer) bool {
	if !isInteractiveTerminal() {
		return false
	}
	if _, ok := stdout.(*os.File); !ok {
		return false
	}
	if _, ok := stderr.(*os.File); !ok {
		return false
	}
	return true
}
