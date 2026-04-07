# Fixture Conventions

Use this directory for vendor-shaped input fixtures.

Recommended layout:

```text
testdata/fixtures/<tool>/<case>/input
testdata/fixtures/<tool>/<case>/expected
```

Current live cases:

- `codex/basic_latest`
- `codex/missing_session`
- `codex/secret_redaction`
- `gemini/explicit_session`
- `gemini/malformed_history`
- `claude/partial_history`
