package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/jaeyoung0509/work-bridge/internal/detect"
	"github.com/jaeyoung0509/work-bridge/internal/doctor"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/exporter"
	"github.com/jaeyoung0509/work-bridge/internal/importer"
	"github.com/jaeyoung0509/work-bridge/internal/tui"
)

func (a *App) runRoot(cmd *cobra.Command, _ []string) error {
	if !shouldLaunchTUI(a.stdout, a.stderr) {
		return a.runHeadlessOverview(cmd.OutOrStdout())
	}

	backend := tui.Backend{
		LoadWorkspaceSnapshot: a.loadWorkspaceSnapshot,
		ImportSession: func(ctx context.Context, tool domain.Tool, session string) (domain.SessionBundle, error) {
			cwd, homeDir, err := a.resolveWorkingDirs()
			if err != nil {
				return domain.SessionBundle{}, err
			}
			return importer.Import(a.importerOptions(cwd, homeDir, string(tool), session))
		},
		DoctorBundle: func(ctx context.Context, bundle domain.SessionBundle, target domain.Tool) (domain.CompatibilityReport, error) {
			return doctor.Analyze(doctor.Options{Bundle: bundle, Target: target})
		},
		ExportBundle: func(ctx context.Context, bundle domain.SessionBundle, target domain.Tool, outDir string) (domain.ExportManifest, error) {
			report, err := doctor.Analyze(doctor.Options{Bundle: bundle, Target: target})
			if err != nil {
				return domain.ExportManifest{}, err
			}
			return exporter.Export(exporter.Options{
				FS:     a.fs,
				Bundle: bundle,
				Report: report,
				OutDir: outDir,
			})
		},
		InstallSkill:     a.installSkillFromTUI,
		ProbeMCP:         a.probeMCPFromTUI,
		DefaultExportDir: a.config.Output.ExportDir,
	}

	return tui.Run(cmd.Context(), backend, os.Stdout, os.Stderr)
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
