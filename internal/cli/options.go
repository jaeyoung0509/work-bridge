package cli

import (
	"fmt"

	"github.com/jaeyoung0509/work-bridge/internal/importer"
)

func (a *App) resolveWorkingDirs() (string, string, error) {
	cwd, err := a.getwd()
	if err != nil {
		return "", "", fmt.Errorf("resolve current directory: %w", err)
	}
	homeDir, err := a.home()
	if err != nil {
		return "", "", fmt.Errorf("resolve home directory: %w", err)
	}
	return cwd, homeDir, nil
}

func (a *App) importerOptions(cwd string, homeDir string, tool string, sessionID string) importer.Options {
	return importer.Options{
		FS:         a.fs,
		CWD:        cwd,
		HomeDir:    homeDir,
		ToolPaths:  a.config.Paths,
		Tool:       tool,
		Session:    sessionID,
		ImportedAt: a.clock.Now().Format("2006-01-02T15:04:05Z07:00"),
		Redaction:  a.config.Redaction,
		LookPath:   a.look,
	}
}
