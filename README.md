# work-bridge

> **Switching between Claude Code, Gemini CLI, OpenCode, and Codex on the same project because of model cost or context limits?**  
> Keep moving in another tool without losing the current goal, project instructions, skills, or MCP context that matter.

`work-bridge` is a local-first handoff tool for AI coding-agent workflows. It reads a source session, normalizes the useful project context, and helps you:

- prepare another tool so you can continue working there immediately
- export the same handoff tree into a separate directory for review or transfer

By default (`--mode project`), it writes project-local instruction files, skills, and MCP config without touching external tool storage. Using `--mode native`, it writes a best-effort native continuation into the target tool's home-level session store or uses the target's native CLI delegate.

> **Stability:** `work-bridge` is still early and not fully stable. Project-native apply and export paths are covered by tests, but some migration paths are still under active refinement. Use `--dry-run` first when trying a new source/target pair.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![npm](https://img.shields.io/npm/v/%40work-bridge%2Fwork-bridge?color=cb3837&logo=npm)](https://www.npmjs.com/package/@work-bridge/work-bridge)

---

## Why work-bridge?

Most coding-agent tools keep valuable context in incompatible formats. `work-bridge` focuses on the parts that make "open another tool and keep going" actually work:

| Preserved across tools | Notes |
|------------------------|-------|
| Task title and current goal | Normalized from the source session |
| Session summary and decisions | Added to the target-ready handoff |
| Project instruction context | Applied into `CLAUDE.md`, `GEMINI.md`, or `AGENTS.md` |
| Project-scoped skills | Materialized into native repo skill roots such as `.agents/skills/`, `.claude/skills/`, or `.opencode/skills/` |
| Effective MCP config | Materialized into `.work-bridge/<target>/mcp.json` and patched into supported target project config files |
| Portable settings context | Source secrets remain redacted |

The current design is intentionally simpler than the older import/export pipeline:

- `inspect` shows what can be handed off
- `switch` checks resume readiness, then prepares the target directly in the project
- `export` writes the same target-ready state into a separate directory

---

## Supported Tools

| Tool | Inspect source sessions | Project Mode (`--mode project`) | Native Mode (`--mode native`) |
|------|:-----------------------:|:----------------------:|:------------------------:|
| **Claude Code** | ✅ | ✅ | ✅ |
| **Gemini CLI** | ✅ | ✅ | ✅ |
| **OpenCode** | ✅ (SQLite) | ✅ | ✅ (Delegate)* |
| **Codex CLI** | ✅ | ✅ | ✅ |

* OpenCode Native apply uses the official OpenCode CLI delegate (`opencode import <file>`), and native export writes an import-compatible `.opencode_export.json` payload rather than mutating SQLite directly.

**Mode Project (`--mode project`):** Applies instruction files (`CLAUDE.md`, `GEMINI.md`, etc.), project-scoped skill bundles, and MCP configs inside the project or export root only. It does NOT modify external tool storage.

**Mode Native (`--mode native`):** Modifies external system state (e.g. `~/.codex/session_index.jsonl`, `~/.gemini/projects.json`, `~/.claude/projects/`, or invokes `opencode import`) to bootstrap a best-effort native continuation. It also migrates user-scope/global skill bundles and global MCP config to target tool directories.

### Native Mode Support Details

| Feature | Claude | Gemini | Codex | OpenCode |
|---------|--------|--------|-------|----------|
| Session write | ✅ JSONL | ✅ JSON | ✅ JSONL | ✅ CLI delegate |
| History index update | ✅ `history.jsonl` | ✅ `projects.json` | ✅ `session_index.jsonl` | Via `opencode import` |
| CWD/path patching | ✅ Absolute paths | ✅ Project paths | ✅ `session_meta.cwd` + text | Via payload format |
| User-scope skills | ✅ `~/.claude/skills/` | ✅ `~/.agents/skills/` | ✅ `~/.agents/skills/` | ✅ `~/.config/opencode/skills/` |
| Global MCP migration | ✅ additive merge | ✅ additive merge | ✅ additive merge | ✅ additive merge |

> **Note on Global MCP**: Native mode now performs additive merge into the target tool's user-scope config. Existing target entries win on name conflicts, and lossy fields are surfaced as warnings instead of silently overwriting config.

### When to Use Which Mode

**Use `--mode project` when:**
- Working in teams or shared repos
- You want to preserve instruction context across tools
- You don't want to modify external tool storage
- Safe, non-destructive handoff is preferred

**Use `--mode native` when:**
- Migrating sessions between machines (same user, different device)
- You want to resume a session natively in the target tool
- You need to transfer user-scope skills between tools
- Cross-device work continuity is required

---

## Install

### npm

```bash
npm install -g @work-bridge/work-bridge
```

### Go

```bash
go install github.com/jaeyoung0509/work-bridge/cmd/work-bridge@latest
```

### Binary

Download the latest release from [GitHub Releases](https://github.com/jaeyoung0509/work-bridge/releases).

---

## Quick Start

### 1. Start with the TUI

```bash
work-bridge
```

The TUI is the default path for first-time users:

1. Pick the latest session in the current project.
2. Accept the recommended target tool.
3. Review the `Resume readiness` check:
   - `READY`: likely to continue immediately
   - `PARTIAL`: continue after a few manual checks
   - `BLOCKED`: fix issues before trusting the handoff
4. Apply the handoff, then open the target tool and continue.

### 2. Check resume readiness from the CLI

```bash
work-bridge switch \
  --from gemini \
  --session latest \
  --to claude \
  --project /path/to/repo \
  --dry-run
```

### 3. Prepare the target tool in the project

```bash
work-bridge switch \
  --from gemini \
  --session latest \
  --to claude \
  --project /path/to/repo
```

### 4. Or export a handoff tree without touching the project

```bash
work-bridge export \
  --from gemini \
  --session latest \
  --to claude \
  --project /path/to/repo \
  --out /tmp/claude-handoff
```

### 5. Validate a full native migration chain locally

```bash
./scripts/test-native-chain.sh
```

The helper script walks `codex -> gemini -> claude -> opencode` for the current project, auto-builds `./bin/work-bridge` if needed, and verifies each target session after apply. It is intended for local manual validation and requires `jq`.

---

## What `switch` Applies

`switch` writes a managed target state into the project.

Managed session output:

- Claude: `CLAUDE.md` + `.work-bridge/claude/*`
- Gemini: `GEMINI.md` + `.work-bridge/gemini/*`
- Codex: `AGENTS.md` + `.work-bridge/codex/*`
- OpenCode: `AGENTS.md` + `.work-bridge/opencode/*`

Project skill output:

- Codex: `.agents/skills/<name>/SKILL.md`
- Gemini: `.agents/skills/<name>/SKILL.md`
- Claude: `.claude/skills/<name>/SKILL.md`
- OpenCode: `.opencode/skills/<name>/SKILL.md`

Skill bundles keep their original directory layout. `SKILL.md`, `scripts/`, `references/`, `assets/`, and `agents/openai.yaml` are copied as-is.

Managed MCP output:

- `.work-bridge/<target>/mcp.json`
- plus target project config patch where supported:
  - Claude: `.claude/settings.local.json`
  - Gemini: `.gemini/settings.json`
  - OpenCode: `.opencode/opencode.jsonc`
  - Codex: no separate project config patch

Instruction files are updated through a managed block:

```md
<!-- work-bridge:start -->
...
<!-- work-bridge:end -->
```

Re-running `switch` replaces that block instead of appending duplicate content.

---

## What `export` Writes

`export` writes the same target-ready structure into a separate output root instead of modifying the source project.

Example output for `--to claude --out /tmp/claude-handoff`:

- `/tmp/claude-handoff/CLAUDE.md`
- `/tmp/claude-handoff/.claude/settings.local.json`
- `/tmp/claude-handoff/.work-bridge/claude/manifest.json`
- `/tmp/claude-handoff/.work-bridge/claude/mcp.json`
- `/tmp/claude-handoff/.claude/skills/project-helper/SKILL.md`

This is useful when you want a reviewable, portable handoff tree before applying anything to a live repo.

---

## TUI

Run `work-bridge` without arguments to open the interactive TUI:

```bash
work-bridge
```

![work-bridge skills transfer demo](docs/images/work-bridge-demo.gif)

The TUI is the primary way to continue work in another tool. It guides you step-by-step:

1. **Recent Session Selection**: Choose the latest source session from the current workspace.
2. **Recommended Target**: Accept the suggested target tool or customize it.
3. **Resume Check**: Review what carries over, what is skipped, and which manual checks remain.
4. **Confirm & Result**: Prepare the resume path in the project or export a tree, then follow the next-step guidance to continue in the target tool.

**Browser Views:**
Type `/` at any time to inspect workspace resources in a dedicated browser view:
- `/projects`: Scan and switch active projects (configurable via `--workspace-roots`)
- `/skills`: Browse available project and global skills
- `/mcp`: Browse MCP server configurations

**Core Keys:**
- `Enter` select / confirm
- `r` refresh current view
- `?` toggle help
- `q` or `Ctrl+C` quit
- `esc` go back

---

## Command Reference

Use these commands when you want non-interactive workflows or scripting:

```text
work-bridge [flags]
work-bridge inspect <tool> [--limit N]
work-bridge switch --from <tool> --session <id|latest> --to <tool> --project <path> [--dry-run] [--no-skills] [--no-mcp] [--session-only]
work-bridge export --from <tool> --session <id|latest> --to <tool> --project <path> --out <dir> [--dry-run] [--no-skills] [--no-mcp] [--session-only]
work-bridge version

Global Flags:
  --config <path>             Path to a work-bridge config file.
  --format <text|json>        Output format (default "text").
  --verbose                   Enable verbose logging.
  --workspace-roots <paths>   Directories to scan when discovering projects.
```

Supported tools:

- `claude`
- `gemini`
- `codex`
- `opencode`

---

## Limits

Current non-goals:

- no direct SQLite write for OpenCode (uses official CLI delegate for safety)
- no promise that conflicting global MCP entries will be auto-overwritten
- no promise that every source-specific tool event becomes meaningful in every target

Current behavior to be aware of:

- `--mode project` writes instruction files and project-scoped context only (safe for teams)
- `--mode native` modifies external tool storage for session resume state (machine-specific)
- Global/user-scope skills are installed to target tool directories in native mode
  - Claude: `~/.claude/skills/<name>/SKILL.md`
  - Codex: `~/.agents/skills/<name>/SKILL.md`
  - Gemini: `~/.agents/skills/<name>/SKILL.md`
  - OpenCode: `~/.config/opencode/skills/<name>/SKILL.md`
- Global MCP configs are additively merged into target user-scope config files
  - Existing target entries are preserved on name conflicts
  - Lossy target conversions, such as unsupported OpenCode `cwd`, are reported as warnings
- Path patching handles absolute paths in tool results, shell outputs, and text content
- `--session-only` disables skills and MCP materialization
- `--dry-run` is the safest first step for new tool pairs
- `--no-skills` and `--no-mcp` skip supplementary context

### Path Patching in Native Mode

When migrating sessions between machines with different directory structures, `work-bridge` automatically patches absolute paths:

- **Codex**: Updates `session_meta.cwd` and all JSONL text content
- **Gemini**: Updates paths in session JSON content
- **Claude**: Updates paths in JSONL session files
- **OpenCode**: Handled via delegate payload format (`info.directory` plus assistant `path.cwd` / `path.root`)

This ensures tool results, file paths, and shell outputs reference the target machine's paths rather than the source machine's paths.

---

## Security and Redaction

Sensitive values are stripped during source import before a handoff is built.

Examples include:

- keys containing `secret`, `token`, `password`, `auth`, `credential`, `api_key`
- values matching common token-like patterns such as `sk-*`, `ghp_*`, `github_pat_*`, `AIza*`

Redactions stay visible as warnings in the normalized handoff so you can see what was intentionally omitted.
