# sessionport

Local-first CLI for importing coding-agent sessions, normalizing them into a portable working-state bundle, and rehydrating them for Claude Code, Gemini CLI, and Codex CLI.

The CLI scaffold uses `cobra` for command routing and `viper` for config/env wiring so the command surface can grow without rebuilding flag parsing by hand.

## Current state

This repository is scaffolded for v1 implementation. The following commands are live today:

- `detect`
- `inspect <tool>`
- `import --from codex|gemini|claude`

The following commands are still placeholders:

- `doctor`
- `export`
- `pack`
- `unpack`

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

1. Tighten the config schema and precedence rules on top of the Cobra/Viper baseline.
2. Add degraded-path fixtures and CLI snapshot goldens.
3. Enrich Claude import beyond history-only partial coverage when more local data sources are available.
4. Add doctor/export on top of canonical bundle import.

## Config

For now, configuration is scaffolded but intentionally minimal. Values can come from:

- CLI flags
- environment variables such as `SESSIONPORT_FORMAT` and `SESSIONPORT_VERBOSE`
- a config file passed via `--config`
- default config files discovered from the current directory, then home directory:
  - `.sessionport.yaml`
  - `.sessionport.yml`
  - `.sessionport.toml`
  - `.sessionport.json`
