package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"golang.org/x/sync/errgroup"

	"github.com/jaeyoung0509/work-bridge/internal/detect"
	"github.com/jaeyoung0509/work-bridge/internal/domain"
	"github.com/jaeyoung0509/work-bridge/internal/inspect"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
)

type Options struct {
	FS         fsx.FS
	CWD        string
	HomeDir    string
	ToolPaths  domain.ToolPaths
	Tool       string
	Session    string
	ImportedAt string
	Redaction  domain.RedactionPolicy
	LookPath   func(string) (string, error)
}

type settingsImport struct {
	Snapshot   domain.SettingsSnapshot
	Redactions []string
	Warnings   []string
}

type SessionNotFoundError struct {
	Tool    string
	Session string
}

func (e *SessionNotFoundError) Error() string {
	return fmt.Sprintf("%s session %q was not found", e.Tool, e.Session)
}

func Import(opts Options) (domain.SessionBundle, error) {
	raw, err := ImportRaw(opts)
	if err != nil {
		return domain.SessionBundle{}, err
	}
	return NewSessionNormalizer().Normalize(raw)
}

func ImportRaw(opts Options) (RawImportResult, error) {
	imp, err := getImporter(opts.Tool)
	if err != nil {
		return RawImportResult{}, err
	}
	return imp.ImportRaw(opts)
}

func selectSession(tool string, requested string, sessions []inspect.Session) (inspect.Session, error) {
	if len(sessions) == 0 {
		return inspect.Session{}, &SessionNotFoundError{Tool: tool, Session: requested}
	}

	if requested == "" || requested == "latest" {
		return sessions[0], nil
	}

	for _, session := range sessions {
		if session.ID == requested {
			return session, nil
		}
	}

	return inspect.Session{}, &SessionNotFoundError{Tool: tool, Session: requested}
}

func readInstructionArtifacts(fs fsx.FS, tool domain.Tool, assets []detect.ArtifactProbe) []domain.InstructionArtifact {
	var artifacts []domain.InstructionArtifact
	var g errgroup.Group
	g.SetLimit(10)

	results := make([]*domain.InstructionArtifact, len(assets))

	for i, asset := range assets {
		if asset.Kind != "instruction" || !asset.Found {
			continue
		}
		idx, a := i, asset
		g.Go(func() error {
			data, err := fs.ReadFile(a.Path)
			if err != nil {
				return nil
			}
			sum := sha256.Sum256(data)
			results[idx] = &domain.InstructionArtifact{
				Tool:        tool,
				Kind:        "project_instruction",
				Path:        a.Path,
				Scope:       a.Scope,
				Content:     string(data),
				ContentHash: hex.EncodeToString(sum[:]),
			}
			return nil
		})
	}
	_ = g.Wait()

	for _, res := range results {
		if res != nil {
			artifacts = append(artifacts, *res)
		}
	}
	return artifacts
}

func readSettingsSnapshot(fs fsx.FS, assets []detect.ArtifactProbe, policy domain.RedactionPolicy) settingsImport {
	result := settingsImport{
		Snapshot: domain.SettingsSnapshot{
			Included:     map[string]any{},
			ExcludedKeys: []string{},
		},
		Redactions: []string{},
		Warnings:   []string{},
	}

	var g errgroup.Group
	g.SetLimit(10)

	type fileResult struct {
		parsed map[string]any
	}
	results := make([]*fileResult, len(assets))

	for i, asset := range assets {
		if asset.Kind != "config" || !asset.Found {
			continue
		}
		idx, a := i, asset
		g.Go(func() error {
			data, err := fs.ReadFile(a.Path)
			if err != nil {
				return nil
			}

			var parsed map[string]any
			switch strings.ToLower(filepath.Ext(a.Path)) {
			case ".json":
				if err := json.Unmarshal(data, &parsed); err != nil {
					return nil
				}
			case ".jsonc":
				if err := json.Unmarshal(stripJSONCComments(data), &parsed); err != nil {
					return nil
				}
			case ".toml":
				if err := toml.Unmarshal(data, &parsed); err != nil {
					return nil
				}
			default:
				return nil
			}
			results[idx] = &fileResult{parsed: parsed}
			return nil
		})
	}

	_ = g.Wait()

	seenExcluded := map[string]struct{}{}
	for _, res := range results {
		if res == nil || res.parsed == nil {
			continue
		}
		for key, value := range res.parsed {
			if isSensitiveKey(key, policy) {
				if _, ok := seenExcluded[key]; !ok {
					result.Snapshot.ExcludedKeys = append(result.Snapshot.ExcludedKeys, key)
					result.Redactions = append(result.Redactions, "settings."+key)
					seenExcluded[key] = struct{}{}
				}
				continue
			}

			if filtered, ok := simplifySettingValue(value, policy); ok {
				result.Snapshot.Included[key] = filtered
				continue
			}

			if _, ok := seenExcluded[key]; !ok {
				result.Snapshot.ExcludedKeys = append(result.Snapshot.ExcludedKeys, key)
				result.Redactions = append(result.Redactions, "settings."+key)
				seenExcluded[key] = struct{}{}
			}
		}
	}

	sort.Strings(result.Snapshot.ExcludedKeys)
	return result
}

func simplifySettingValue(value any, policy domain.RedactionPolicy) (any, bool) {
	switch typed := value.(type) {
	case string, bool, int64, int32, int16, int8, int, float64, float32:
		if typedString, ok := typed.(string); ok && policy.DetectSensitiveValues && looksSensitiveValue(typedString) {
			return nil, false
		}
		return typed, true
	case []any:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			filtered, ok := simplifySettingValue(item, policy)
			if !ok {
				return nil, false
			}
			values = append(values, filtered)
		}
		return values, true
	case []string:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			if policy.DetectSensitiveValues && looksSensitiveValue(item) {
				return nil, false
			}
			values = append(values, item)
		}
		return values, true
	default:
		return nil, false
	}
}

func isSensitiveKey(key string, policy domain.RedactionPolicy) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, needle := range []string{"secret", "token", "password", "auth", "oauth", "credential", "api_key", "apikey"} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	for _, needle := range policy.AdditionalSensitiveKeys {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle != "" && strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

func looksSensitiveValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "sk-") || strings.HasPrefix(value, "ghp_") || strings.HasPrefix(value, "github_pat_") || strings.HasPrefix(value, "AIza") {
		return true
	}
	return len(value) >= 24 && (strings.Count(value, "_")+strings.Count(value, "-")) >= 2
}

func mergeSettings(raw *RawImportResult, imported settingsImport) {
	raw.SettingsSnapshot = imported.Snapshot
	raw.Redactions = append(raw.Redactions, imported.Redactions...)
	raw.Warnings = append(raw.Warnings, imported.Warnings...)
}

func inspectOptions(opts Options, tool string) inspect.Options {
	return inspect.Options{
		FS:        opts.FS,
		CWD:       opts.CWD,
		HomeDir:   opts.HomeDir,
		ToolPaths: opts.ToolPaths,
		Tool:      tool,
		LookPath:  opts.LookPath,
		Limit:     1 << 30,
	}
}

func defaultToolRoot(opts Options, tool domain.Tool) string {
	return opts.ToolPaths.Dir(tool, opts.HomeDir)
}
