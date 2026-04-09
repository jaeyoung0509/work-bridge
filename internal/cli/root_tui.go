package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"sessionport/internal/catalog"
	"sessionport/internal/detect"
	"sessionport/internal/doctor"
	"sessionport/internal/domain"
	"sessionport/internal/exporter"
	"sessionport/internal/importer"
	"sessionport/internal/inspect"
	"sessionport/internal/tui"
)

func (a *App) runRoot(cmd *cobra.Command, _ []string) error {
	if !shouldLaunchTUI(a.stdout, a.stderr) {
		return a.runHeadlessOverview(cmd.OutOrStdout())
	}

	backend := tui.Backend{
		Detect: a.detectWorkspace,
		Inspect: func(ctx context.Context, tool string) (inspect.Report, error) {
			return inspect.Run(inspect.Options{
				FS:        a.fs,
				CWD:       mustCurrentDir(a.getwd),
				HomeDir:   mustHomeDir(a.home),
				ToolPaths: a.config.Paths,
				Tool:      tool,
				LookPath:  a.look,
				Limit:     100,
			})
		},
		Import: func(ctx context.Context, tool string, session string) (domain.SessionBundle, error) {
			return importer.Import(a.importerOptions(mustCurrentDir(a.getwd), mustHomeDir(a.home), tool, session))
		},
		Doctor: func(ctx context.Context, bundle domain.SessionBundle, target domain.Tool) (domain.CompatibilityReport, error) {
			return doctor.Analyze(doctor.Options{Bundle: bundle, Target: target})
		},
		Export: func(ctx context.Context, bundle domain.SessionBundle, target domain.Tool, outDir string) (domain.ExportManifest, error) {
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
		ScanSkills: func(ctx context.Context) ([]tui.SkillEntry, error) {
			cwd, homeDir, err := a.resolveWorkingDirs()
			if err != nil {
				return nil, err
			}
			entries, err := catalog.ScanSkills(a.fs, cwd, homeDir)
			if err != nil {
				return nil, err
			}
			out := make([]tui.SkillEntry, 0, len(entries))
			for _, entry := range entries {
				out = append(out, tui.SkillEntry(entry))
			}
			return out, nil
		},
		ScanMCP: func(ctx context.Context) ([]tui.MCPEntry, error) {
			cwd, homeDir, err := a.resolveWorkingDirs()
			if err != nil {
				return nil, err
			}
			entries, err := catalog.ScanMCP(a.fs, cwd, homeDir, a.config.Paths)
			if err != nil {
				return nil, err
			}
			out := make([]tui.MCPEntry, 0, len(entries))
			for _, entry := range entries {
				out = append(out, tui.MCPEntry(entry))
			}
			return out, nil
		},
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
		_, err := fmt.Fprintf(out, "sessionport (headless)\nproject: %s\ncwd: %s\ntools: %d\n", detectReport.ProjectRoot, detectReport.CWD, len(detectReport.Tools))
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

func mustCurrentDir(getwd func() (string, error)) string {
	if getwd == nil {
		return ""
	}
	cwd, _ := getwd()
	return cwd
}

func mustHomeDir(home func() (string, error)) string {
	if home == nil {
		return ""
	}
	dir, _ := home()
	return dir
}
