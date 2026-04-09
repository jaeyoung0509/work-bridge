# work-bridge

`work-bridge` is a local-first bridge for coding-agent workflows.

It does two jobs:

- inspect local sessions, skills, and MCP configs across supported tools
- export portable session artifacts so work can continue in another tool

The default entrypoint is a Bubble Tea TUI. Legacy CLI commands remain available for automation and tests.

## Supported Tools

- `codex`
- `claude`
- `gemini`
- `opencode`

## Current TUI Scope

- `Sessions`: inspect, import, doctor, export
- `Projects`: index project roots from configured workspace roots and drive active workspace scope
- `Skills`: inspect grouped skills, compare project/user/global coverage, and sync across scopes
- `MCP`: inspect known config locations, merge effective scope by server, and run runtime validation for stdio, HTTP, and SSE transports
- `Logs`: recent workspace actions and errors

Mouse support currently covers:

- pane focus
- list selection
- preview tab switching
- wheel scrolling for lists and previews

MCP validation performs a real runtime handshake where the transport supports it. `work-bridge` sends `initialize`, follows with `notifications/initialized`, and counts advertised `resources`, `resourceTemplates`, `tools`, and `prompts` where available for stdio, HTTP, and legacy SSE servers.

## Build

```bash
make build
./bin/work-bridge
```

## Test

```bash
make test
go test ./...
```

## CLI Examples

```bash
./bin/work-bridge detect
./bin/work-bridge inspect codex --limit 5
./bin/work-bridge import --from codex --session latest
./bin/work-bridge doctor --from codex --session latest --target claude
./bin/work-bridge export --bundle ./bundle.json --target gemini --out ./out
```

## Config

Configuration precedence:

1. CLI flags
2. environment variables
3. `--config`
4. discovered config in the current directory, then home directory
5. built-in defaults

Supported config files:

- `.work-bridge.yaml`
- `.work-bridge.yml`
- `.work-bridge.toml`
- `.work-bridge.json`

Environment variables use the `WORK_BRIDGE_` prefix.

Useful config keys:

- `workspace_roots`
- `paths.codex`
- `paths.gemini`
- `paths.claude`
- `paths.opencode`
- `output.export_dir`

Example:

```json
{
  "format": "json",
  "workspace_roots": [
    "~/Projects",
    "~/work"
  ],
  "paths": {
    "codex": "/Users/me/.local/share/codex"
  },
  "output": {
    "export_dir": "./out/work-bridge"
  }
}
```

## Repository Layout

```text
cmd/work-bridge/      CLI entrypoint
internal/cli/         command wiring and TUI backend adapters
internal/tui/         Bubble Tea workspace
internal/domain/      portable bundle types
internal/importer/    tool-specific session importers
internal/exporter/    target-specific artifact generation
testdata/             fixtures and golden outputs
```

## Contributing

See `CONTRIBUTING.md`.
