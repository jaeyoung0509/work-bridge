# work-bridge

> **Switching between Claude Code, Gemini CLI, OpenCode, and Codex on the same project because of model cost or context limits?**  
> Inspect the source session, then either apply a target-ready handoff directly into the project or export the same handoff into a separate output tree.

`work-bridge` is a local-first handoff tool for AI coding-agent workflows. It reads a source session, normalizes the useful project context, and either:

- applies a target-ready state into project-native files
- exports the same target-ready state to a separate directory

By default (`--mode project`), it applies changes to managed project files. Using `--mode native`, it actively writes the actual resume state into the target tool's home-level session database or uses the target's native CLI delegate to ensure the session can be seamlessly resumed.

> **Stability:** `work-bridge` is still early and not fully stable. Project-native apply and export paths are covered by tests, but some migration paths are still under active refinement. Use `--dry-run` first when trying a new source/target pair.

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![npm](https://img.shields.io/npm/v/%40work-bridge%2Fwork-bridge?color=cb3837&logo=npm)](https://www.npmjs.com/package/@work-bridge/work-bridge)

---

## Why work-bridge?

Most coding-agent tools keep valuable context in incompatible formats. `work-bridge` keeps the useful project-facing parts portable:

| Preserved across tools | Notes |
|------------------------|-------|
| Task title and current goal | Normalized from the source session |
| Session summary and decisions | Added to the target-ready handoff |
| Project instruction context | Applied into `CLAUDE.md`, `GEMINI.md`, or `AGENTS.md` |
| Project-scoped skills | Materialized into `.work-bridge/<target>/skills/` |
| Effective MCP config | Materialized into `.work-bridge/<target>/mcp.json` and patched into supported target project config files |
| Portable settings context | Source secrets remain redacted |

The current design is intentionally simpler than the older import/export pipeline:

- `inspect` shows what can be handed off
- `switch` previews and applies directly into the project
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

**Mode Project (`--mode project`):** Applies instruction files (`CLAUDE.md`, `GEMINI.md`, etc.), project-scoped skills, and MCP configs inside the project root only. Does NOT modify external tool storage. Safe for teams and shared repos.

**Mode Native (`--mode native`):** Modifies external system state (e.g. `~/.codex/session_index.jsonl`, `~/.gemini/projects.json`, `~/.claude/projects/`, or invokes `opencode import`) to directly load the resume state. Enables seamless session continuation across machines. Also migrates user-scope/global skills to target tool directories.

### Native Mode Support Details

| Feature | Claude | Gemini | Codex | OpenCode |
|---------|--------|--------|-------|----------|
| Session write | ✅ JSONL | ✅ JSON | ✅ JSONL | ✅ CLI delegate |
| History index update | ✅ `history.jsonl` | ✅ `projects.json` | ✅ `session_index.jsonl` | Via `opencode import` |
| CWD/path patching | ✅ Absolute paths | ✅ Project paths | ✅ `session_meta.cwd` + text | Via payload format |
| User-scope skills | ✅ `~/.claude/skills/` | ✅ `~/.gemini/GEMINI.md` managed block | ✅ `~/.codex/skills/` | ✅ `~/.config/opencode/skills/` |
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

### 1. Inspect available source sessions

```bash
work-bridge inspect gemini --limit 5
```

### 2. Preview a handoff into another tool

```bash
work-bridge switch \
  --from gemini \
  --session latest \
  --to claude \
  --project /path/to/repo \
  --dry-run
```

### 3. Apply the handoff into the project

```bash
work-bridge switch \
  --from gemini \
  --session latest \
  --to claude \
  --project /path/to/repo
```

### 4. Or export the same target-ready tree without touching the project

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

Managed skills output:

- `.work-bridge/<target>/skills/*.md`
- `.work-bridge/<target>/skills/index.json`

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
- `/tmp/claude-handoff/.work-bridge/claude/skills/index.json`

This is useful when you want a reviewable, portable handoff tree before applying anything to a live repo.

---

## TUI

Run `work-bridge` without arguments to open the interactive migration console:

```bash
work-bridge
```

The TUI is now focused on one workflow:

- choose a source session from the current project
- choose a target tool
- preview the handoff
- apply or export

Key actions:

- `Enter` preview
- `a` apply into the project
- `e` export into `.work-bridge/exports/<target>/`
- `r` refresh
- `?` help
- `q` quit

User-facing statuses are simplified:

- `READY`
- `APPLIED`
- `PARTIAL`
- `ERROR`

---

## CLI Reference

```text
work-bridge [flags]
work-bridge inspect <tool> [--limit N]
work-bridge switch --from <tool> --session <id|latest> --to <tool> --project <path> [--dry-run] [--no-skills] [--no-mcp] [--session-only]
work-bridge export --from <tool> --session <id|latest> --to <tool> --project <path> --out <dir> [--dry-run] [--no-skills] [--no-mcp] [--session-only]
work-bridge version
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
  - Claude: `~/.claude/skills/`
  - Codex: `~/.codex/skills/`
  - OpenCode: `~/.config/opencode/skills/<name>/SKILL.md`
  - Gemini: appended to a managed block inside `~/.gemini/GEMINI.md`
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
