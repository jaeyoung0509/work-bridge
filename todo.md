# sessionport TODO

## Done

- [x] Go module initialized
- [x] Basic repository scaffold created
- [x] Canonical bundle domain skeleton added
- [x] Platform abstraction skeleton added
- [x] CI / Makefile / basic tests added
- [x] CLI migrated to `cobra`
- [x] Config/env wiring scaffolded with `viper`

## Next

- [ ] Define concrete CLI config schema and precedence rules
- [ ] Implement `detect` command with JSON/text output
- [ ] Implement `inspect <tool>` inventory command
- [ ] Add fixture loader helpers
- [ ] Add first Codex fixture set
- [ ] Add first Gemini fixture set

## Importers

- [ ] Codex path resolver
- [ ] Codex session inventory + latest session selection
- [ ] Codex config allowlist import
- [ ] Codex `AGENTS.md` import
- [ ] Gemini path resolver
- [ ] Gemini session/history import
- [ ] Gemini settings allowlist import
- [ ] Gemini `GEMINI.md` import
- [ ] Claude partial instruction/memory import

## Core

- [ ] Raw import -> canonical bundle normalizer
- [ ] Provenance aggregation
- [ ] Warning/redaction aggregation
- [ ] Bundle validation pass tightening
- [ ] Doctor compatibility matrix

## Exporters

- [ ] Codex exporter
- [ ] Gemini exporter
- [ ] Claude exporter
- [ ] Export manifest format

## Package

- [ ] `.spkg` manifest spec
- [ ] Pack command
- [ ] Unpack command

## Testing

- [ ] Golden file helpers
- [ ] Fixture naming conventions encoded in helper APIs
- [ ] Importer contract tests
- [ ] Doctor matrix tests
- [ ] Exporter golden tests
- [ ] End-to-end CLI snapshots
