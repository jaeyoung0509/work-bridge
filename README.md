# work-bridge

> **Switching between Claude Code, Gemini CLI, OpenCode, and Codex on the same project because of LLM cost?**  
> Keep your project-scoped session context visible, export restart artifacts for the next tool, sync skills where supported, and validate MCP before you switch.

`work-bridge` is a local-first portability layer for AI coding-agent workflows. It helps you inspect project-scoped sessions, compare skills across scopes, validate MCP server configs, and export starter artifacts for the next tool. It does not write directly into another tool's native session database.

> **Stability:** `work-bridge` is still early and not fully stable yet. Some migration paths, especially importer-heavy flows in the TUI, are still under active crash triage. If the TUI is unreliable for your case, prefer the CLI subcommands and inspect/import/doctor/export flows directly.

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

| Tool | Session import | Export artifacts | MCP inspect/probe | Skill sync target |
|------|:--------------:|:----------------:|:-----------------:|:-----------------:|
| **Claude Code** | ✅ | ✅ | ✅ | ✅ user scope |
| **Gemini CLI** | ✅ | ✅ | ✅ | Not yet |
| **OpenCode** | ✅ | ✅ | ✅ | ✅ user + global scope |
| **Codex CLI** | ✅ | ✅ | ✅ | ✅ user scope |

Project-local `skills/` and `.github/skills/` content is still discovered across tools. The table above only describes named per-tool sync targets exposed by the current TUI.

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

### What It Does Today

- `Projects` sets the active scope for `Sessions`, `Skills`, and `MCP`
- `Sessions` lets you import, doctor-check, and export starter artifacts for another tool
- `Skills` lets you compare project/user/global scopes and copy skills where a target path exists
- `MCP` lets you inspect merged scope and run runtime probes for stdio, HTTP, and SSE servers

Current non-goals:

- No native session-store injection into another tool
- No automatic MCP config rewrite or apply step
- No Gemini-specific skill sync target yet

### Migration Workflow

```
You were using Gemini CLI → now switching to Claude Code
```

```bash
# 1. Inspect what Gemini has
work-bridge inspect gemini --limit 5

# 2. Import the latest Gemini session into a portable bundle
work-bridge import --from gemini --session latest --out ./bundle.json

# 3. Check compatibility with Claude Code
work-bridge doctor --from gemini --session latest --target claude

# 4. Export Claude starter artifacts
work-bridge export --bundle ./bundle.json --target claude --out ./out/

# 5. Review the exported files and merge the supplement into your project's CLAUDE.md
ls ./out/
```

The exported `./out/` directory contains:

- `CLAUDE.work-bridge.md` — project supplement to merge into `CLAUDE.md`
- `MEMORY_NOTE.md` — summary and portability warnings
- `STARTER_PROMPT.md` — copy-paste prompt to resume your task
- `manifest.json` — export manifest with portability warnings

For Gemini CLI exports, `work-bridge` writes `GEMINI.work-bridge.md` as a starter artifact. Gemini CLI's default context filename is `GEMINI.md`, so you still need to merge or rename the file, or configure Gemini's `context.fileName` explicitly.

### Claude E2E Shell Script

Run a real `gemini -> claude` migration check against a local project without launching the TUI:

```bash
./scripts/e2e_gemini_to_claude.sh /path/to/repo
```

Useful overrides:

```bash
SESSION_ID=<gemini-session-id> OUT_DIR=/tmp/work-bridge-out ./scripts/e2e_gemini_to_claude.sh /path/to/repo
```

The script will:

1. Run `work-bridge --format json inspect gemini`
2. Pick the latest Gemini session whose `project_root` matches the selected repo
3. Run `import`, `doctor --target claude`, and `export`
4. Print project markers, known skill directories, and MCP config locations
5. Save `inspect`, `detect`, `bundle`, and `doctor` JSON into a debug directory for follow-up

This is the fastest way to debug real-user migration failures because it bypasses the TUI completely. If the latest Gemini session does not belong to the selected project, the script fails instead of silently using some other repo's session.

### Debugging TUI Crashes

If the TUI exits immediately or appears to "just close", capture the full terminal transcript with `script`:

```bash
script -q /tmp/work-bridge-tui.log zsh -lc 'work-bridge'
```

This keeps the alternate-screen escape sequences and any panic trace in one file. It is the easiest way to confirm whether the failure happened in the TUI renderer or inside an importer/exporter command.

To isolate the failing stage without the TUI, run the underlying commands directly:

```bash
work-bridge inspect gemini --limit 10
work-bridge import --from gemini --session <id> --out /tmp/bundle.json
work-bridge doctor --from gemini --session <id> --target claude
work-bridge export --bundle /tmp/bundle.json --target claude --out /tmp/out
```

Known limitation:

- Some importer paths are still under crash triage. If the TUI path is unstable, prefer the shell script or the direct CLI sequence above until the crash is fixed.

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

`work-bridge` does not apply MCP configs to another tool yet. The current MCP flow is inspect, merge, and probe.

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
