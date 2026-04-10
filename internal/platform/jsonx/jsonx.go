package jsonx

import "encoding/json"

// UnmarshalRelaxed parses JSON and JSONC-like config content while tolerating
// comments and trailing commas outside of strings.
func UnmarshalRelaxed(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err == nil {
		return nil
	}
	return json.Unmarshal(Sanitize(data), v)
}

// Sanitize removes comments and trailing commas from JSON-like config content
// without touching characters inside quoted strings.
func Sanitize(data []byte) []byte {
	return stripTrailingCommas(stripComments(data))
}

func stripComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	escaped := false
	lineComment := false
	blockComment := false

	for i := 0; i < len(data); i++ {
		ch := data[i]

		switch {
		case lineComment:
			if ch == '\n' {
				lineComment = false
				out = append(out, ch)
			}
			continue
		case blockComment:
			if ch == '\n' {
				out = append(out, ch)
				continue
			}
			if ch == '*' && i+1 < len(data) && data[i+1] == '/' {
				blockComment = false
				i++
			}
			continue
		case inString:
			out = append(out, ch)
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}
		if ch == '/' && i+1 < len(data) {
			switch data[i+1] {
			case '/':
				lineComment = true
				i++
				continue
			case '*':
				blockComment = true
				i++
				continue
			}
		}

		out = append(out, ch)
	}

	return out
}

func stripTrailingCommas(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	escaped := false

	for i := 0; i < len(data); i++ {
		ch := data[i]
		if inString {
			out = append(out, ch)
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(data) {
				switch data[j] {
				case ' ', '\t', '\r', '\n':
					j++
				default:
					goto nextToken
				}
			}
		nextToken:
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				continue
			}
		}
		out = append(out, ch)
	}

	return out
}
