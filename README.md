# work-bridge

> **Switching between Claude Code, Gemini CLI, OpenCode, and Codex due to LLM cost?**  
> Bring your session context, MCP configs, and skills with you — instantly.

`work-bridge` is a local-first portability layer for AI coding-agent workflows. It inspects your current sessions, skills, and MCP server configs across all major tools, then exports a portable bundle that any other tool can pick up from where you left off.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![npm](https://img.shields.io/npm/v/%40work-bridge%2Fwork-bridge?color=cb3837&logo=npm)](https://www.npmjs.com/package/@work-bridge/work-bridge)

---

## Why work-bridge?

Modern AI coding agents store their state in incompatible formats:

| What you lose when switching tools | What work-bridge preserves |
|------------------------------------|---------------------------|
| Current task title & goal          | ✅ Task title & current goal |
| Session summary & decisions        | ✅ Summary, decisions, failures |
| Project instruction files (AGENTS.md, CLAUDE.md, GEMINI.md…) | ✅ All instruction artifacts |
| MCP server configurations          | ✅ Inspected & validated MCP configs |
| Portable settings (non-sensitive)  | ✅ Settings snapshot (secrets stripped) |
| Touched files & tool events        | ✅ File touch history & event log |

---

## Supported Tools

| Tool | Import | Export | MCP | Skills |
|------|:------:|:------:|:---:|:------:|
| **Claude Code** | ✅ | ✅ | ✅ | ✅ |
| **Gemini CLI** | ✅ | ✅ | ✅ | ✅ |
| **OpenCode** | ✅ | ✅ | ✅ | ✅ |
| **Codex CLI** | ✅ | ✅ | ✅ | ✅ |

---

## Quick Start

### Install via npm (recommended)

```bash
npm install -g @work-bridge/work-bridge
work-bridge
```

### Install via Go

```bash
go install github.com/jaeyoung0509/work-bridge/cmd/work-bridge@latest
```

### Download binary

Grab the latest release binary from [GitHub Releases](https://github.com/jaeyoung0509/work-bridge/releases).

---

## Usage

### TUI (Interactive Mode)

Just run `work-bridge` in your terminal for the full interactive experience:

```bash
work-bridge
```

The TUI provides five panels:

| Panel | What it does |
|-------|-------------|
| **Sessions** | Browse, inspect, import, doctor-check, and export sessions |
| **Projects** | Index project roots from configured workspace roots |
| **Skills** | Compare project/user/global skill coverage and sync across scopes |
| **MCP** | Inspect config locations, merge effective scope, and run runtime validation |
| **Logs** | View recent workspace actions and errors |

**Mouse support:** pane focus · list selection · preview tab switching · scroll

### Migration Workflow

```
You were using Claude Code → now switching to Gemini CLI
```

```bash
# 1. Inspect what Claude Code has
work-bridge inspect claude --limit 5

# 2. Import the latest session into a portable bundle
work-bridge import --from claude --session latest --out ./bundle.json

# 3. Check compatibility with the target tool
work-bridge doctor --from claude --session latest --target gemini

# 4. Export target-native artifacts
work-bridge export --bundle ./bundle.json --target gemini --out ./out/

# 5. Start Gemini CLI — it'll pick up GEMINI.work-bridge.md automatically
cd ./out && gemini
```

The exported `./out/` directory contains:

- `GEMINI.work-bridge.md` — context supplement injected into Gemini CLI
- `SETTINGS_PATCH.json` — portable settings to apply
- `STARTER_PROMPT.md` — copy-paste prompt to resume your task
- `manifest.json` — export manifest with portability warnings

### Pack / Unpack (Portable Archives)

Share your session state across machines or teammates:

```bash
# Pack a session into a portable .spkg archive
work-bridge pack --from claude --session latest --out ./my-session.spkg

# Unpack on another machine and target a different tool
work-bridge unpack --file ./my-session.spkg --target codex --out ./out/
```

### Detect

```bash
# Auto-detect all installed tools and project artifacts
work-bridge detect
```

### CLI Reference

```
work-bridge [flags]
work-bridge detect
work-bridge inspect <tool> [--limit N]
work-bridge import  --from <tool> [--session <id|latest>] [--out <path>]
work-bridge doctor  --from <tool> [--session <id|latest>] --target <tool>
work-bridge export  --bundle <path> --target <tool> [--out <dir>]
work-bridge pack    --from <tool> [--session <id|latest>] --out <path>
work-bridge unpack  --file <path> --target <tool> [--out <dir>]
work-bridge version
```

**Supported tools:** `claude` · `gemini` · `codex` · `opencode`

---

## How It Works

```
┌─────────────────────────────────────────────────────────────────┐
│                         work-bridge                             │
│                                                                 │
│  detect/ ──► inspect/ ──► importer/ ──► domain.SessionBundle   │
│                                              │                  │
│                                         doctor/                 │
│                                    (compatibility check)        │
│                                              │                  │
│                                         exporter/               │
│                                    (target-native artifacts)    │
└─────────────────────────────────────────────────────────────────┘

SessionBundle fields:
  source_tool        · source_session_id  · project_root
  task_title         · current_goal       · summary
  instruction_artifacts (AGENTS.md, CLAUDE.md, GEMINI.md …)
  settings_snapshot  (sensitive keys redacted automatically)
  tool_events        · touched_files
  decisions          · failures           · resume_hints
  token_stats        · provenance         · redactions
```

### Security & Redaction

`work-bridge` automatically strips sensitive values during import:

- Key-based: `secret`, `token`, `password`, `auth`, `oauth`, `credential`, `api_key`, `apikey`
- Value-based heuristics: `sk-*`, `ghp_*`, `github_pat_*`, `AIza*` prefixes, and long random-looking strings
- Redacted fields are listed in `bundle.redactions` for transparency — nothing is silently dropped

---

## MCP Validation

The **MCP** panel (and `inspect` output) runs a real runtime handshake for supported transports:

1. Spawns the server process (stdio) or connects (HTTP / SSE)
2. Sends `initialize` → waits for capability response
3. Sends `notifications/initialized`
4. Counts advertised `resources`, `resourceTemplates`, `tools`, and `prompts`

This catches config problems that a static lint pass would miss.

---

## Configuration

Configuration is resolved in this priority order:

1. CLI flags
2. Environment variables (`WORK_BRIDGE_` prefix)
3. `--config <file>`
4. Auto-discovered config in CWD → then home directory
5. Built-in defaults

### Supported Config Files

- `.work-bridge.yaml` / `.work-bridge.yml`
- `.work-bridge.toml`
- `.work-bridge.json`

### Key Config Fields

```yaml
# .work-bridge.yaml
workspace_roots:
  - ~/Projects
  - ~/work

paths:
  codex:     ~/.local/share/codex
  gemini:    ~/.config/gemini
  claude:    ~/.claude
  opencode:  ~/.config/opencode

output:
  export_dir:   ./out/work-bridge
  package_path: ./session.spkg
  unpack_dir:   ./out/unpacked

redaction:
  detect_sensitive_values: true
  additional_sensitive_keys:
    - my_internal_token
```

### JSON Example

```json
{
  "format": "json",
  "workspace_roots": ["~/Projects", "~/work"],
  "paths": {
    "codex": "/Users/me/.local/share/codex"
  },
  "output": {
    "export_dir": "./out/work-bridge"
  }
}
```

### Environment Variables

| Variable | Config Key |
|----------|-----------|
| `WORK_BRIDGE_FORMAT` | `format` |
| `WORK_BRIDGE_WORKSPACE_ROOTS` | `workspace_roots` |
| `WORK_BRIDGE_PATHS_CODEX` | `paths.codex` |
| `WORK_BRIDGE_PATHS_GEMINI` | `paths.gemini` |
| `WORK_BRIDGE_PATHS_CLAUDE` | `paths.claude` |
| `WORK_BRIDGE_PATHS_OPENCODE` | `paths.opencode` |
| `WORK_BRIDGE_OUTPUT_EXPORT_DIR` | `output.export_dir` |

---

## Repository Layout

```
cmd/work-bridge/          CLI entrypoint (main.go)
internal/
  cli/                    Cobra/Viper root command, TUI backend wiring
    app.go                App struct, Run(), Config
    root_tui.go           TUI launch logic and Backend wiring
    legacy_commands.go    detect/inspect/import/doctor/export commands
    package_commands.go   pack/unpack commands
    root_tui_backend.go   All TUI backend action implementations
  tui/                    Bubble Tea v2 interactive workspace
  domain/                 Portable bundle types (SessionBundle, Tool, …)
  importer/               Tool-specific session importers
    claude.go / codex.go / gemini.go / opencode.go
    normalizer.go         Signal extraction & normalization
    signals.go            Decision/failure/hint signal detection
  exporter/               Target-native artifact generation
  detect/                 Tool installation & project artifact detection
  inspect/                Session listing for each tool
  doctor/                 Cross-tool compatibility analysis
  catalog/                Skills catalog (project/user/global scopes)
  capability/             MCP capability registry
  packagex/               .spkg pack/unpack (zip-based)
  platform/               FS, clock, archive, env, redact utilities
testdata/                 Fixtures and golden outputs
scripts/
  install.cjs             npm postinstall binary downloader
```

---

## Building from Source

```bash
git clone https://github.com/jaeyoung0509/work-bridge.git
cd work-bridge

# Build
make build
./bin/work-bridge

# Test
make test

# Lint
make lint

# Format
make fmt
```

**Requirements:** Go 1.21+

---

## Publishing a Release

Releases use [GoReleaser](https://goreleaser.com/). A GitHub Actions workflow triggers on version tags:

```bash
git tag v0.1.0
git push origin v0.1.0
```

GoReleaser builds cross-platform binaries (`darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`, `windows/arm64`) and uploads them as `.tar.gz` / `.zip` archives to the GitHub Release.

The npm package wrapper then downloads the correct binary at install time via `scripts/install.cjs`.

---

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).

**Good first areas:**
- Add a new tool importer under `internal/importer/`
- Extend `internal/exporter/` with a new target format
- Improve MCP runtime validation for new transport types
- Add fixture-backed tests for edge cases

Principles:
- Local-first, no network calls during normal operation
- Deterministic output for fixture-backed tests
- Sensitive data must never leave the machine unredacted

---

## License

MIT © [jaeyoung0509](https://github.com/jaeyoung0509)
