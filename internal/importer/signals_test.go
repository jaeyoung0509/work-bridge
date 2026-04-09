package importer

import (
	"reflect"
	"testing"
)

func TestExtractTouchedFilesIgnoresScalarJSONStrings(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"false", "123", "\"hello\"", "null"} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			if got := extractTouchedFiles(input); len(got) != 0 {
				t.Fatalf("expected no touched files for %q, got %#v", input, got)
			}
		})
	}
}

func TestExtractTouchedFilesParsesNestedJSONContainers(t *testing.T) {
	t.Parallel()

	got := extractTouchedFiles(map[string]any{
		"payload": `{"path":"README.md","enabled":false}`,
	})
	want := []string{"README.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected nested JSON path extraction %#v, got %#v", want, got)
	}
}

func TestExtractTouchedFilesNormalizesStructInputs(t *testing.T) {
	t.Parallel()

	type args struct {
		Path    string `json:"path"`
		Enabled bool   `json:"enabled"`
	}

	got := extractTouchedFiles(args{
		Path:    "docs/design.md",
		Enabled: false,
	})
	want := []string{"docs/design.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected struct path extraction %#v, got %#v", want, got)
	}
}
