package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"sessionport/internal/detect"
	"sessionport/internal/domain"
	"sessionport/internal/inspect"
	"sessionport/internal/platform/fsx"
)

type Options struct {
	FS         fsx.FS
	CWD        string
	HomeDir    string
	Tool       string
	Session    string
	ImportedAt string
	LookPath   func(string) (string, error)
}

type SessionNotFoundError struct {
	Tool    string
	Session string
}

func (e *SessionNotFoundError) Error() string {
	return fmt.Sprintf("%s session %q was not found", e.Tool, e.Session)
}

func Import(opts Options) (domain.SessionBundle, error) {
	switch opts.Tool {
	case "codex":
		return importCodex(opts)
	case "gemini":
		return importGemini(opts)
	case "claude":
		return domain.SessionBundle{}, errors.New("claude import is not implemented yet")
	default:
		return domain.SessionBundle{}, fmt.Errorf("unsupported tool %q", opts.Tool)
	}
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
	artifacts := []domain.InstructionArtifact{}
	for _, asset := range assets {
		if asset.Kind != "instruction" || !asset.Found {
			continue
		}
		data, err := fs.ReadFile(asset.Path)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		artifacts = append(artifacts, domain.InstructionArtifact{
			Tool:        tool,
			Kind:        "project_instruction",
			Path:        asset.Path,
			Scope:       asset.Scope,
			Content:     string(data),
			ContentHash: hex.EncodeToString(sum[:]),
		})
	}
	return artifacts
}

func readSettingsSnapshot(fs fsx.FS, assets []detect.ArtifactProbe) domain.SettingsSnapshot {
	snapshot := domain.SettingsSnapshot{
		Included:     map[string]any{},
		ExcludedKeys: []string{},
	}

	seenExcluded := map[string]struct{}{}
	for _, asset := range assets {
		if asset.Kind != "config" || !asset.Found {
			continue
		}
		data, err := fs.ReadFile(asset.Path)
		if err != nil {
			continue
		}

		var parsed map[string]any
		switch strings.ToLower(filepath.Ext(asset.Path)) {
		case ".json":
			if err := json.Unmarshal(data, &parsed); err != nil {
				continue
			}
		case ".toml":
			if err := toml.Unmarshal(data, &parsed); err != nil {
				continue
			}
		default:
			continue
		}

		for key, value := range parsed {
			if isSensitiveKey(key) {
				if _, ok := seenExcluded[key]; !ok {
					snapshot.ExcludedKeys = append(snapshot.ExcludedKeys, key)
					seenExcluded[key] = struct{}{}
				}
				continue
			}

			if filtered, ok := simplifySettingValue(value); ok {
				snapshot.Included[key] = filtered
				continue
			}

			if _, ok := seenExcluded[key]; !ok {
				snapshot.ExcludedKeys = append(snapshot.ExcludedKeys, key)
				seenExcluded[key] = struct{}{}
			}
		}
	}

	return snapshot
}

func simplifySettingValue(value any) (any, bool) {
	switch typed := value.(type) {
	case string, bool, int64, int32, int16, int8, int, float64, float32:
		return typed, true
	case []any:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			filtered, ok := simplifySettingValue(item)
			if !ok {
				return nil, false
			}
			values = append(values, filtered)
		}
		return values, true
	case []string:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
		return values, true
	default:
		return nil, false
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(key)
	for _, needle := range []string{"secret", "token", "password", "auth", "oauth", "credential", "api_key", "apikey"} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}
