# work-bridge

> **Switching between Claude Code, Gemini CLI, OpenCode, and Codex on the same project because of model cost or context limits?**  
> Inspect the source session, then either apply a target-ready handoff directly into the project or export the same handoff into a separate output tree.

`work-bridge` is a local-first handoff tool for AI coding-agent workflows. It reads a source session, normalizes the useful project context, and either:

- applies a target-ready state into project-native files
- exports the same target-ready state to a separate directory

It does **not** write into another tool's home-level session database.

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

| Tool | Inspect source sessions | Apply to project files | Export target-ready tree |
|------|:-----------------------:|:----------------------:|:------------------------:|
| **Claude Code** | ✅ | ✅ | ✅ |
| **Gemini CLI** | ✅ | ✅ | ✅ |
| **OpenCode** | ✅ | ✅ | ✅ |
| **Codex CLI** | ✅ | ✅ | ✅ |

Project-native apply means files inside the project root only. `work-bridge` does **not** recreate native session state in `~/.codex`, `~/.gemini`, `~/.claude`, or `~/.config/opencode`.

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

Current non-goals in this slice:

- no home-level session-store injection
- no recreation of a target tool's private resume database
- no promise that every source-specific tool event becomes meaningful in every target

Current behavior to be aware of:

- `switch` is project-native apply, not native session resurrection
- `export` is out-of-project handoff generation, not a bundle archive format
- `--session-only` disables skills and MCP materialization
- `--dry-run` is the safest first step for new tool pairs

---

## Security and Redaction

Sensitive values are stripped during source import before a handoff is built.

Examples include:

- keys containing `secret`, `token`, `password`, `auth`, `credential`, `api_key`
- values matching common token-like patterns such as `sk-*`, `ghp_*`, `github_pat_*`, `AIza*`

Redactions stay visible as warnings in the normalized handoff so you can see what was intentionally omitted.
