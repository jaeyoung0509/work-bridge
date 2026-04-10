package stringx

import (
	"testing"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "foo bar", "foo-bar"},
		{"consecutive symbols", "foo!!bar", "foo-bar"},
		{"trailing symbols", "foo!!", "foo"},
		{"mixed case", "FooBar", "foobar"},
		{"with numbers", "v1.0", "v1-0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeName(tc.input)
			if result != tc.expected {
				t.Errorf("SanitizeName(%q) = %q; want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestStripJSONCComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "url in string",
			input:    `{"url": "http://example.com"}`,
			expected: `{"url": "http://example.com"}`,
		},
		{
			name:     "real comment",
			input:    `{"url": "http://example.com"} // comment`,
			expected: `{"url": "http://example.com"}`,
		},
		{
			name:     "escaped quote",
			input:    `{"msg": "say \"hi\""}`,
			expected: `{"msg": "say \"hi\""}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := string(StripJSONCComments([]byte(tc.input)))
			// Trim right newline for comparison
			result = result[:len(result)-1]
			if result != tc.expected {
				t.Errorf("StripJSONCComments(%q) = %q; want %q", tc.input, result, tc.expected)
			}
		})
	}
}
