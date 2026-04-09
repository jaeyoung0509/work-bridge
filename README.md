# sessionport

Local-first CLI for importing coding-agent sessions, normalizing them into a portable working-state bundle, and rehydrating them for Claude Code, Gemini CLI, and Codex CLI.

`sessionport` remains session-first. Skill/workspace portability and any Bubble Tea TUI are follow-on layers that build on this engine rather than replace it.

The CLI scaffold uses `cobra` for command routing and `viper` for config/env wiring so the command surface can grow without rebuilding flag parsing by hand.

## Current state

This repository now has a working session portability core. The following commands are live today:

- `detect`
- `inspect <tool>`
- `import --from codex|gemini|claude`
- `doctor --from <tool> --target <tool>`
- `export --bundle <file> --target <tool> --out <dir>`
- `pack --from <tool> --session <id|latest> --out <file>`
- `unpack --file <file> --target <tool> --out <dir>`

Global flags already scaffolded:

- `--config`
- `--format`
- `--verbose`

## Quickstart

```bash
make test
make build
./bin/sessionport --help
./bin/sessionport detect
./bin/sessionport --format json detect
./bin/sessionport inspect codex --limit 5
./bin/sessionport --format json inspect gemini --limit 5
./bin/sessionport import --from codex --session latest
./bin/sessionport import --from gemini --session latest --out ./out/gemini-bundle.json
./bin/sessionport import --from claude --session latest
./bin/sessionport doctor --from codex --session latest --target claude
./bin/sessionport --format json doctor --from gemini --session latest --target codex
./bin/sessionport export --bundle ./out/gemini-bundle.json --target claude --out ./rehydrated
./bin/sessionport pack --from codex --session latest --out ./bundle.spkg
./bin/sessionport unpack --file ./bundle.spkg --target gemini --out ./rehydrated-from-spkg
```

## Layout

```text
cmd/sessionport/        CLI entrypoint
internal/cli/          cobra/viper wiring, command routing, exit codes
internal/domain/       canonical bundle types
internal/platform/     filesystem, env, clock, archive, redaction abstractions
testdata/fixtures/     vendor fixture inputs for parser tests
testdata/golden/       expected outputs for golden tests
docs/                  implementation and testing docs
```

## Next implementation slice

1. Expand fixture coverage for degraded/import-fidelity cases and keep doctor/export consistency locked.
2. Continue enriching session import signals such as touched files, decisions, failures, and Claude transcript augmentation.
3. Stabilize config-driven path overrides, output defaults, and redaction policy handling.
4. Treat skill/workspace portability and Bubble Tea surfaces as phase-2 consumers of the session engine.

## Config

Configuration precedence is:

1. CLI flags
2. environment variables
3. an explicit config passed with `--config`
4. a discovered config from the current directory, then home directory
5. built-in defaults

Supported config categories now include:

- default output and display settings: `format`, `verbose`
- per-tool root overrides: `paths.codex`, `paths.gemini`, `paths.claude`
- output defaults: `output.import_bundle_path`, `output.export_dir`, `output.package_path`, `output.unpack_dir`
- redaction policy: `redaction.additional_sensitive_keys`, `redaction.detect_sensitive_values`

Values can come from:

- CLI flags
- environment variables such as `SESSIONPORT_FORMAT` and `SESSIONPORT_VERBOSE`
- a config file passed via `--config`
- default config files discovered from the current directory, then home directory:
  - `.sessionport.yaml`
  - `.sessionport.yml`
  - `.sessionport.toml`
  - `.sessionport.json`

Example:

```json
{
  "format": "json",
  "paths": {
    "codex": "/Users/me/.local/share/codex"
  },
  "output": {
    "export_dir": "./out/sessionport"
  },
  "redaction": {
    "additional_sensitive_keys": ["workspace_secret"],
    "detect_sensitive_values": true
  }
}
```
