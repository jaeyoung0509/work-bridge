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
- [x] `import --from codex|gemini|claude` implemented
- [x] `doctor --from <tool> --target <tool>` implemented
- [x] `export --bundle <file> --target <tool> --out <dir>` implemented
- [x] Codex session bundle importer added
- [x] Gemini session bundle importer added
- [x] Claude partial bundle importer added
- [x] Import CLI exit codes for missing sessions added
- [x] Importer unit tests added
- [x] Fixture loader helpers added
- [x] First Codex fixture set added
- [x] First Gemini fixture set added
- [x] First Claude fixture set added
- [x] Importer golden contract tests added

## Next

- [ ] Define concrete CLI config schema and precedence rules
- [ ] Add degraded-path fixtures (`missing_*`, `corrupt_*`, `secret_*`)
- [ ] Add CLI snapshot goldens
- [ ] Add doctor matrix fixtures
- [ ] Start pack/unpack

## Importers

- [x] Codex path resolver
- [x] Codex session inventory + latest session selection
- [x] Codex config allowlist import
- [x] Codex `AGENTS.md` import
- [x] Gemini path resolver
- [x] Gemini session/history import
- [x] Gemini settings allowlist import
- [x] Gemini `GEMINI.md` import
- [x] Claude partial instruction/memory import
- [x] Claude session/history import
- [ ] Redaction policy hardening for imported settings
- [ ] Touched file / decision / failure extraction heuristics
- [ ] Claude raw transcript import when local session storage is available

## Core

- [ ] Raw import -> canonical bundle normalizer
- [ ] Provenance aggregation
- [ ] Warning/redaction aggregation
- [ ] Bundle validation pass tightening
- [x] Doctor compatibility matrix

## Exporters

- [x] Codex exporter
- [x] Gemini exporter
- [x] Claude exporter
- [x] Export manifest format

## Package

- [ ] `.spkg` manifest spec
- [ ] Pack command
- [ ] Unpack command

## Testing

- [x] Golden file helpers
- [ ] Fixture naming conventions encoded in helper APIs
- [x] Importer contract tests
- [x] Doctor matrix tests
- [x] Exporter golden tests
- [ ] End-to-end CLI snapshots
