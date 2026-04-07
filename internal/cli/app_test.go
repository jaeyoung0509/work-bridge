package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunPrintsHelpByDefault(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), nil, &stdout, &stderr)

	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{"sessionport", "detect", "inspect", "doctor", "--config"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected help output to contain %q, got %q", want, output)
		}
	}
}

func TestRunReturnsUsageForUnknownCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"wat"}, &stdout, &stderr)

	if exitCode != ExitUsage {
		t.Fatalf("expected exit code %d, got %d", ExitUsage, exitCode)
	}
	if !strings.Contains(stderr.String(), `unknown command "wat"`) {
		t.Fatalf("expected unknown command error, got %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout for unknown command, got %q", stdout.String())
	}
}

func TestPlaceholderCommandReturnsNotImplemented(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"detect"}, &stdout, &stderr)

	if exitCode != ExitNotImplemented {
		t.Fatalf("expected exit code %d, got %d", ExitNotImplemented, exitCode)
	}
	if !strings.Contains(stderr.String(), "detect command is scaffolded but not implemented yet") {
		t.Fatalf("expected placeholder message, got %q", stderr.String())
	}
}

func TestVersionCommandPrintsVersion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"version"}, &stdout, &stderr)

	if exitCode != ExitOK {
		t.Fatalf("expected exit code %d, got %d", ExitOK, exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "sessionport") {
		t.Fatalf("expected version output, got %q", stdout.String())
	}
}
