# Codex CLI Session Storage Analysis

This document details the internal session storage architecture of Codex CLI (codex-rs), based on a deep dive into the source code and research. This information is intended to facilitate session import/export and interoperability with other AI agents like Claude Code, Gemini CLI, and OpenCode.

## 1. Storage Location & Directory Structure

On macOS, Codex CLI stores its session data in a date-based hierarchical structure within the user's home directory.

- **Root Directory**: `~/.codex/sessions/`
- **Structure**: `YYYY/MM/DD/rollout-<timestamp>-<uuid>.jsonl`
- **Example**: `~/.codex/sessions/2026/04/09/rollout-2026-04-09T14-30-00-550e8400-e29b-41d4-a716-446655440000.jsonl`

This structure allows for efficient organization and auditing of work history by date.

## 2. File Format: JSON Lines (JSONL)

Codex CLI uses a JSON Lines format for its "rollout" files. Each line is a standalone JSON object representing an event or a piece of metadata. This append-only design ensures data integrity even if the process crashes.

### 2.1. The Root Item Structure
Each line in the `.jsonl` file follows this top-level schema:
```json
{
  "type": "item_type",
  "payload": { ... }
}
```

## 3. The Session Metadata (`session_meta`)

The **first line** of every rollout file is always a `session_meta` item. This is the most critical part for session identification and resumption.

### 3.1. Schema
```json
{
  "type": "session_meta",
  "payload": {
    "id": "uuid-string",
    "timestamp": "ISO-8601-timestamp",
    "cwd": "/absolute/path/to/project",
    "cli_version": "0.x.x",
    "originator": "user-identifier",
    "model_provider": "provider-id",
    "source": { "type": "terminal" },
    "git": {
      "commit_hash": "sha1-hash",
      "branch": "branch-name",
      "repository_url": "git-remote-url"
    }
  }
}
```

### 3.2. Critical Field: `cwd` (Current Working Directory)
Codex CLI uses the `cwd` field to filter sessions when the user runs `codex resume`.
- **Logic**: When `codex resume` is executed, the CLI scans `~/.codex/sessions/` and reads the first line of every file. It only displays sessions where the `cwd` matches the current directory.
- **Interoperability Requirement**: For cross-device migration (e.g., Mac Studio to MacBook), the `cwd` path **must be updated** to match the absolute path on the target machine. If the paths differ, the session will be "invisible" to the `resume` command on the new device.

## 4. Response Items (`response_item`)

Subsequent lines in the file represent the actual conversation and tool interactions.

### 4.1. Message Event
Stores user prompts and assistant responses.
```json
{
  "type": "response_item",
  "payload": {
    "type": "message",
    "role": "user" | "assistant",
    "content": [
      { "type": "output_text", "text": "..." },
      { "type": "input_text", "text": "..." }
    ],
    "phase": "commentary" | "final_answer"
  }
}
```

### 4.2. Reasoning Event
Stores the assistant's internal "thinking" or chain-of-thought.
```json
{
  "type": "response_item",
  "payload": {
    "type": "reasoning",
    "summary": "Short summary of thought",
    "content": "Full detailed reasoning..."
  }
}
```

### 4.3. Tool Execution Event (Local Shell)
Stores bash/shell command executions.
```json
{
  "type": "response_item",
  "payload": {
    "type": "local_shell_call",
    "call_id": "call-uuid",
    "status": { "type": "completed", "exit_code": 0 },
    "action": {
      "type": "run_command",
      "command": "ls -la",
      "output": "..."
    }
  }
}
```

## 5. Import/Export & Migration Strategy (work-bridge)

To successfully migrate a Codex session between machines:

1.  **Extract**: Identify the `.jsonl` files in `~/.codex/sessions/` corresponding to the project.
2.  **Transfer**: Copy the files while maintaining the `YYYY/MM/DD` directory structure on the target machine.
3.  **Patch (The "CWD Patching" Step)**:
    - Open the `.jsonl` file.
    - Read the first line (`session_meta`).
    - Replace the value of `payload.cwd` with the absolute path of the project on the **target** machine.
    - Save the file.
4.  **Verify**: Run `codex resume` in the project directory on the target machine. The migrated session should now appear in the list.

## 6. Checkpoints

Codex CLI also generates a `checkpoint_v1.json` for internal optimization and context compression. While not strictly necessary for basic resume functionality, it's recommended to include it in full migrations if it exists in the same directory as the session files (though research suggests it's often stored alongside the rollout).
