# sessionport TODO

## Done

- [x] Go module initialized
- [x] Basic repository scaffold created
- [x] Canonical bundle domain skeleton added
- [x] Platform abstraction skeleton added
- [x] CI / Makefile / basic tests added
- [x] CLI migrated to `cobra`
- [x] Config/env wiring scaffolded with `viper`
- [x] `detect` command implemented with text/json output
- [x] `inspect <tool>` inventory command implemented
- [x] `import --from codex|gemini` implemented
- [x] Codex session bundle importer added
- [x] Gemini session bundle importer added
- [x] Import CLI exit codes for missing sessions added
- [x] Importer unit tests added

## Next

- [ ] Define concrete CLI config schema and precedence rules
- [ ] Add fixture loader helpers
- [ ] Add first Codex fixture set
- [ ] Add first Gemini fixture set
- [ ] Add importer golden outputs

## Importers

- [x] Codex path resolver
- [x] Codex session inventory + latest session selection
- [x] Codex config allowlist import
- [x] Codex `AGENTS.md` import
- [x] Gemini path resolver
- [x] Gemini session/history import
- [x] Gemini settings allowlist import
- [x] Gemini `GEMINI.md` import
- [ ] Claude partial instruction/memory import
- [ ] Claude session/history import
- [ ] Redaction policy hardening for imported settings
- [ ] Touched file / decision / failure extraction heuristics

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
