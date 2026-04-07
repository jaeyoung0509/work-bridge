package cli

import (
	"fmt"
	"path/filepath"
)

func (a *App) readConfigFile(path string) error {
	a.viper.SetConfigFile(path)
	if err := a.viper.ReadInConfig(); err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	return nil
}

func (a *App) findDefaultConfigFile() (string, error) {
	cwd, err := a.getwd()
	if err != nil {
		return "", err
	}
	homeDir, err := a.home()
	if err != nil {
		return "", err
	}

	for _, dir := range []string{cwd, homeDir} {
		for _, name := range []string{
			".sessionport.yaml",
			".sessionport.yml",
			".sessionport.toml",
			".sessionport.json",
		} {
			candidate := filepath.Join(dir, name)
			info, statErr := a.fs.Stat(candidate)
			if statErr != nil {
				continue
			}
			if !info.IsDir() {
				return candidate, nil
			}
		}
	}

	return "", nil
}
