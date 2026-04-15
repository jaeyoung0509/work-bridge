# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.8] - 2026-04-15

### Changed

- **Searchable TUI browsing** — Projects, Sessions, Skills, and MCP views now expose an explicit search field with live filtering.
- **Tool-grouped assets** — Skills and MCP entries are grouped by LLM/tool with richer metadata and clearer action labels.
- **Handoff button stability** — Focused handoff actions now keep a stable box model, so the layout no longer shifts when selecting buttons.

## [0.1.7] - 2026-04-14

### Added

- **OpenCode native payload format** — Export payload now matches `opencode export <sessionID>` output structure with `info` + `messages` + `parts` format, ensuring full `opencode import <file>` compatibility
- **Native mode path patching** — Absolute path substitution across all tools during native session migration:
  - **Codex**: `session_meta.cwd` + JSONL internal text paths
  - **Gemini**: Session JSON internal paths
  - **Claude**: JSONL session file internal paths
- **Global/user-scope skills migration** — Native mode now installs user-scope skills to target tool directories:
  - Claude: `~/.claude/skills/`
  - Codex: `~/.codex/skills/`
  - OpenCode: `~/.config/opencode/skills/`
- **OpenCode SQLite fixture tests** — Independent test fixtures using in-memory SQLite DB creation in `internal/importer/opencode_sqlite_test.go` and `internal/inspect/opencode_sqlite_test.go`
- **E2E test infrastructure** — Local-only E2E tests with `//go:build e2e` tag and `WORKBRIDGE_E2E=1` requirement (excluded from CI):
  - `tests/e2e/e2e_test.go` — Cross-tool session migration, smoke tests, mode flag validation
  - `tests/e2e/cross_tool_test.go` — Native mode migration, export/import cycles, global skills validation

### Changed

- **README** — Updated with native mode support details table, project vs native mode comparison, path patching documentation, and global skills/MCP support status
- **`applyGlobalSkills()` / `applyGlobalMCP()`** — New helper methods in `native_mode.go` for migrating user-scope skills and issuing advisory warnings for global MCP configs

## [0.1.5] - 2026-04-09

### Added

- **`internal/platform/pathpatch`** — Cross-agent absolute-path translation engine
  - `ClaudeProjectDirName`: encodes a project path to Claude's `~/.claude/projects/` dir-name format
  - `GeminiProjectHashLegacy`: SHA-256 hash for legacy Gemini project keys
  - `GeminiProjectSlug`: slug derivation with collision handling for `projects.json` injection
  - `PatchJSONBytes` / `PatchJSONLBytes`: recursive JSON / JSONL string-value path substitution
  - `PatchCodexSessionMetaCWD`: rewrites `session_meta.cwd` in Codex rollout JSONL files
  - `ReplacePathsInText`: raw text path substitution for log / shell output

- **`internal/switcher/apply_native.go`** — Post-apply native storage integration (closes #14)
  - **Codex**: rewrites `session_meta.cwd` in managed JSONL exports so `codex resume` resolves the correct project on the target machine; full payload path substitution across tool records
  - **Gemini**: creates `.project_root` sentinel; injects slug into `~/.gemini/projects.json` with collision-safe deduplication
  - **Claude**: patches payload paths; removes stale `sessions-index.json` so the imported session surfaces immediately in the `/resume` picker
  - **OpenCode**: patches JSON/JSONL paths; emits advisory warning for SQLite native integration

- **Codex TOML MCP config** — `apply` now generates `.codex/config.toml` with `[mcp.servers.*]` tables when MCP servers are present

- **`fsx.FS.Remove`** — Added `Remove(name string) error` to the filesystem interface and `OSFS` implementation

- **Build version injection** — `Makefile` now passes `VERSION`, `COMMIT`, and `BUILD_DATE` via `-ldflags`
  - `make build` → binary reports the current git tag / commit
  - `make release VERSION=v0.1.5` → reproducible release build
  - `work-bridge version` and `--version` now print commit hash and build timestamp

### Changed

- `work-bridge version` output now includes commit hash and build date:
  ```
  work-bridge v0.1.5 (commit: abc1234, built: 2026-04-09T20:00:00Z)
  ```
- `--version` flag output updated to match

### Fixed

- Second `apply` on the same project was creating `manifest.json` backups due to path-patch touching the file; `manifest.json` is now always excluded from path patching (it is freshly regenerated on every apply)

## [0.1.4] - 2026-04-09

### Added

- Initial public release with `inspect`, `switch`, `export`, and `version` commands
- Support for Codex CLI, Gemini CLI, Claude Code, and OpenCode session import
- Canonical `SessionBundle` intermediate representation
- Skills and MCP server collection and cross-agent re-emission
- `.work-bridge/<tool>/` managed project root pattern

[Unreleased]: https://github.com/jaeyoung0509/work-bridge/compare/v0.1.8...HEAD
[0.1.8]: https://github.com/jaeyoung0509/work-bridge/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/jaeyoung0509/work-bridge/compare/v0.1.6...v0.1.7
[0.1.5]: https://github.com/jaeyoung0509/work-bridge/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/jaeyoung0509/work-bridge/releases/tag/v0.1.4
