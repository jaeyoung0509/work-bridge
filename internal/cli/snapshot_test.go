package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jaeyoung0509/work-bridge/internal/importer"
	"github.com/jaeyoung0509/work-bridge/internal/platform/fsx"
	"github.com/jaeyoung0509/work-bridge/internal/testutil"
)

type cliSnapshot struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

func TestCLISnapshotsMatchGolden(t *testing.T) {
	repoRoot := testutil.RepoRoot(t)

	cases := []struct {
		name    string
		fixture string
		golden  string
		run     func(t *testing.T, fixture testutil.Fixture, tmpRoot string) cliSnapshot
	}{
		{
			name:    "detect text",
			fixture: "codex/basic_latest",
			golden:  "detect_text_codex_basic_latest.json",
			run: func(t *testing.T, fixture testutil.Fixture, tmpRoot string) cliSnapshot {
				return runFixtureApp(t, fixture, []string{"detect"})
			},
		},
		{
			name:    "inspect json",
			fixture: "codex/basic_latest",
			golden:  "inspect_json_codex_basic_latest.json",
			run: func(t *testing.T, fixture testutil.Fixture, tmpRoot string) cliSnapshot {
				return runFixtureApp(t, fixture, []string{"--format", "json", "inspect", "codex", "--limit", "1"})
			},
		},
		{
			name:    "doctor text",
			fixture: "codex/basic_latest",
			golden:  "doctor_text_codex_to_claude.json",
			run: func(t *testing.T, fixture testutil.Fixture, tmpRoot string) cliSnapshot {
				return runFixtureApp(t, fixture, []string{"doctor", "--from", "codex", "--session", "latest", "--target", "claude"})
			},
		},
		{
			name:    "export json",
			fixture: "codex/basic_latest",
			golden:  "export_json_codex_to_gemini.json",
			run: func(t *testing.T, fixture testutil.Fixture, tmpRoot string) cliSnapshot {
				bundlePath := filepath.Join(tmpRoot, "bundle.json")
				createFixtureBundle(t, fixture, bundlePath)
				return runFixtureApp(t, fixture, []string{"--format", "json", "export", "--bundle", bundlePath, "--target", "gemini", "--out", filepath.Join(tmpRoot, "out")})
			},
		},
		{
			name:    "pack json",
			fixture: "codex/basic_latest",
			golden:  "pack_json_codex_basic_latest.json",
			run: func(t *testing.T, fixture testutil.Fixture, tmpRoot string) cliSnapshot {
				return runFixtureApp(t, fixture, []string{"--format", "json", "pack", "--from", "codex", "--session", "latest", "--out", filepath.Join(tmpRoot, "bundle.spkg")})
			},
		},
		{
			name:    "unpack json",
			fixture: "codex/basic_latest",
			golden:  "unpack_json_codex_to_claude.json",
			run: func(t *testing.T, fixture testutil.Fixture, tmpRoot string) cliSnapshot {
				archivePath := filepath.Join(tmpRoot, "bundle.spkg")
				packResult := runFixtureApp(t, fixture, []string{"pack", "--from", "codex", "--session", "latest", "--out", archivePath})
				if packResult.ExitCode != ExitOK {
					t.Fatalf("pack setup failed: %#v", packResult)
				}
				return runFixtureApp(t, fixture, []string{"--format", "json", "unpack", "--file", archivePath, "--target", "claude", "--out", filepath.Join(tmpRoot, "unpacked")})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := testutil.StageFixture(t, filepath.Join(repoRoot, "testdata", "fixtures", filepath.FromSlash(tc.fixture)))
			tmpRoot := t.TempDir()
			got := tc.run(t, fixture, tmpRoot)
			got = normalizeSnapshotOutput(got, fixture, tmpRoot)
			testutil.AssertGoldenJSON(t, filepath.Join(repoRoot, "testdata", "golden", "cli", tc.golden), got)
		})
	}
}

func runFixtureApp(t *testing.T, fixture testutil.Fixture, args []string) cliSnapshot {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := New(&stdout, &stderr)
	app.fs = fsx.OSFS{}
	app.getwd = func() (string, error) { return fixture.WorkspaceDir, nil }
	app.home = func() (string, error) { return fixture.HomeDir, nil }
	app.look = func(binary string) (string, error) {
		switch binary {
		case "codex", "gemini", "claude":
			return "/opt/bin/" + binary, nil
		default:
			return "", errors.New("not found")
		}
	}
	app.clock = fixedClock{value: time.Date(2026, 4, 7, 16, 0, 0, 0, time.UTC)}

	return cliSnapshot{
		ExitCode: app.Run(context.Background(), args),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

func createFixtureBundle(t *testing.T, fixture testutil.Fixture, bundlePath string) {
	t.Helper()

	bundle, err := importer.Import(importer.Options{
		FS:         fsx.OSFS{},
		CWD:        fixture.WorkspaceDir,
		HomeDir:    fixture.HomeDir,
		Tool:       "codex",
		Session:    "latest",
		ImportedAt: "2026-04-07T16:00:00Z",
		LookPath: func(binary string) (string, error) {
			if binary == "codex" {
				return "/opt/bin/codex", nil
			}
			return "", errors.New("not found")
		},
	})
	if err != nil {
		t.Fatalf("create bundle failed: %v", err)
	}

	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		t.Fatalf("marshal bundle failed: %v", err)
	}
	if err := (fsx.OSFS{}).WriteFile(bundlePath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write bundle failed: %v", err)
	}
}

func normalizeSnapshotOutput(snapshot cliSnapshot, fixture testutil.Fixture, tmpRoot string) cliSnapshot {
	snapshot.Stdout = normalizeSnapshotText(snapshot.Stdout, fixture, tmpRoot)
	snapshot.Stderr = normalizeSnapshotText(snapshot.Stderr, fixture, tmpRoot)
	return snapshot
}

func normalizeSnapshotText(value string, fixture testutil.Fixture, tmpRoot string) string {
	value = strings.ReplaceAll(value, filepath.Clean(fixture.Root), "$FIXTURE")
	value = strings.ReplaceAll(value, filepath.Clean(tmpRoot), "$TMP")
	return filepath.ToSlash(value)
}
