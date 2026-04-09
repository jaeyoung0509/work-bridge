# Contributing

## Local Workflow

```bash
make build
make test
go test ./...
```

## Project Shape

- `internal/tui`: workspace UI and interaction model
- `internal/cli`: Cobra/Viper wiring and TUI backend actions
- `internal/importer`: session normalization from source tools
- `internal/exporter`: portable artifact generation for target tools

## Notes

- Keep changes local-first and deterministic.
- Prefer fixture-backed tests for importer/exporter behavior.
- MCP probing currently supports stdio servers with a minimal runtime handshake.
- Project discovery is driven by configured `workspace_roots` plus common local defaults.
