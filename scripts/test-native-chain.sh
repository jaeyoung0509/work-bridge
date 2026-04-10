#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd -P)"

WORK_BRIDGE_BIN="$REPO_ROOT/bin/work-bridge"
PROJECT_ROOT="$(pwd -P)"
START_SESSION_ID=""
LIMIT=100
DRY_RUN=0
NO_SKILLS=0
NO_MCP=0
SESSION_ONLY=0
AUTO_BUILD=1

usage() {
  cat <<'EOF'
Usage:
  scripts/test-native-chain.sh [options]

Runs a native migration chain for the current project:
  codex -> gemini -> claude -> opencode

By default, the script starts from the latest Codex session for the current
project. If no project-matched Codex session exists, it falls back to the
latest available Codex session.

Options:
  -p, --project PATH      Project root to use. Default: current directory.
  -b, --bin PATH          Path to the work-bridge binary.
  -s, --session ID        Starting Codex session ID. Default: auto-detect.
      --limit N           Inspect limit when resolving sessions. Default: 100.
      --dry-run           Preview each hop without writing native state.
      --no-skills         Pass --no-skills to each switch command.
      --no-mcp            Pass --no-mcp to each switch command.
      --session-only      Pass --session-only to each switch command.
      --skip-build        Do not auto-build ./bin/work-bridge if it is missing.
  -h, --help             Show this help message.

Examples:
  scripts/test-native-chain.sh
  scripts/test-native-chain.sh --dry-run
  scripts/test-native-chain.sh --session 019d76d6-4396-7662-9618-7c3789fb93e7
EOF
}

log() {
  printf '[chain-test] %s\n' "$*"
}

die() {
  printf '[chain-test] error: %s\n' "$*" >&2
  exit 1
}

canonicalize_dir() {
  local dir="$1"
  [[ -d "$dir" ]] || die "directory not found: $dir"
  (
    cd -- "$dir"
    pwd -P
  )
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

is_positive_integer() {
  [[ "$1" =~ ^[1-9][0-9]*$ ]]
}

validate_output_path() {
  local path="$1"
  if [[ -L "$path" ]]; then
    die "refusing to use symlinked binary path: $path"
  fi
  if [[ -e "$path" && -d "$path" ]]; then
    die "binary path points to a directory: $path"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -p|--project)
      [[ $# -ge 2 ]] || die "missing value for $1"
      PROJECT_ROOT="$(canonicalize_dir "$2")"
      shift 2
      ;;
    -b|--bin)
      [[ $# -ge 2 ]] || die "missing value for $1"
      WORK_BRIDGE_BIN="$2"
      shift 2
      ;;
    -s|--session)
      [[ $# -ge 2 ]] || die "missing value for $1"
      START_SESSION_ID="$2"
      shift 2
      ;;
    --limit)
      [[ $# -ge 2 ]] || die "missing value for $1"
      LIMIT="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --no-skills)
      NO_SKILLS=1
      shift
      ;;
    --no-mcp)
      NO_MCP=1
      shift
      ;;
    --session-only)
      SESSION_ONLY=1
      shift
      ;;
    --skip-build)
      AUTO_BUILD=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

PROJECT_ROOT="$(canonicalize_dir "$PROJECT_ROOT")"
is_positive_integer "$LIMIT" || die "--limit must be a positive integer: $LIMIT"

require_cmd jq
validate_output_path "$WORK_BRIDGE_BIN"

if [[ ! -x "$WORK_BRIDGE_BIN" ]]; then
  if [[ "$AUTO_BUILD" -ne 1 ]]; then
    die "work-bridge binary is missing or not executable: $WORK_BRIDGE_BIN"
  fi
  require_cmd go
  log "Building work-bridge at $WORK_BRIDGE_BIN"
  (
    cd -- "$REPO_ROOT"
    mkdir -p -- "$(dirname -- "$WORK_BRIDGE_BIN")"
    go build -o "$WORK_BRIDGE_BIN" ./cmd/work-bridge
  )
fi

if [[ ! -x "$WORK_BRIDGE_BIN" ]]; then
  die "work-bridge binary is missing or not executable after build: $WORK_BRIDGE_BIN"
fi

inspect_json() {
  local tool="$1"
  "$WORK_BRIDGE_BIN" --format json inspect "$tool" --limit "$LIMIT"
}

resolve_session_id() {
  local tool="$1"
  local preferred_id="${2:-}"
  local report session_id

  report="$(inspect_json "$tool")"
  session_id="$(
    printf '%s\n' "$report" | jq -r \
      --arg project "$PROJECT_ROOT" \
      --arg preferred "$preferred_id" \
      '
      ([.sessions[] | select($preferred != "" and .id == $preferred)][0].id
       // [.sessions[] | select(.project_root == $project)][0].id
       // .sessions[0].id
       // empty)
      '
  )"

  [[ -n "$session_id" ]] || die "no importable session found for $tool"
  printf '%s\n' "$session_id"
}

wait_for_target_session() {
  local tool="$1"
  local expected_id="$2"
  local attempts="${3:-10}"
  local delay_seconds="${4:-1}"
  local report session_id

  for ((attempt = 1; attempt <= attempts; attempt++)); do
    report="$(inspect_json "$tool" || true)"
    session_id="$(
      printf '%s\n' "$report" | jq -r \
        --arg project "$PROJECT_ROOT" \
        --arg expected "$expected_id" \
        '
        ([.sessions[] | select(.id == $expected)][0].id
         // [.sessions[] | select(.project_root == $project)][0].id
         // empty)
        ' 2>/dev/null || true
    )"

    if [[ -n "$session_id" && "$session_id" != "null" ]]; then
      printf '%s\n' "$session_id"
      return 0
    fi

    sleep "$delay_seconds"
  done

  die "could not confirm migrated session in $tool after ${attempts} attempts"
}

run_switch() {
  local from_tool="$1"
  local session_id="$2"
  local to_tool="$3"
  local cmd=(
    "$WORK_BRIDGE_BIN"
    switch
    --from "$from_tool"
    --session "$session_id"
    --to "$to_tool"
    --project "$PROJECT_ROOT"
    --mode native
  )

  if [[ "$DRY_RUN" -eq 1 ]]; then
    cmd+=(--dry-run)
  fi
  if [[ "$NO_SKILLS" -eq 1 ]]; then
    cmd+=(--no-skills)
  fi
  if [[ "$NO_MCP" -eq 1 ]]; then
    cmd+=(--no-mcp)
  fi
  if [[ "$SESSION_ONLY" -eq 1 ]]; then
    cmd+=(--session-only)
  fi

  log "Running ${from_tool} -> ${to_tool} with session ${session_id}"
  "${cmd[@]}"
}

CHAIN=(codex gemini claude opencode)
CURRENT_SESSION_ID="$START_SESSION_ID"

if [[ -z "$CURRENT_SESSION_ID" ]]; then
  CURRENT_SESSION_ID="$(resolve_session_id codex)"
fi

log "Project root: $PROJECT_ROOT"
log "Starting Codex session: $CURRENT_SESSION_ID"
if [[ "$DRY_RUN" -eq 1 ]]; then
  log "Dry-run mode enabled. Native stores will not be modified."
fi

for ((i = 0; i < ${#CHAIN[@]} - 1; i++)); do
  SOURCE_TOOL="${CHAIN[$i]}"
  TARGET_TOOL="${CHAIN[$((i + 1))]}"

  CURRENT_SESSION_ID="$(resolve_session_id "$SOURCE_TOOL" "$CURRENT_SESSION_ID")"
  run_switch "$SOURCE_TOOL" "$CURRENT_SESSION_ID" "$TARGET_TOOL"

  if [[ "$DRY_RUN" -eq 1 ]]; then
    log "Dry-run: skipping target verification for $TARGET_TOOL"
    continue
  fi

  NEXT_SESSION_ID="$(wait_for_target_session "$TARGET_TOOL" "$CURRENT_SESSION_ID")"
  if [[ "$NEXT_SESSION_ID" != "$CURRENT_SESSION_ID" ]]; then
    log "Target $TARGET_TOOL resolved a different session ID: $NEXT_SESSION_ID"
  else
    log "Verified $TARGET_TOOL session: $NEXT_SESSION_ID"
  fi
  CURRENT_SESSION_ID="$NEXT_SESSION_ID"
done

log "Completed native chain test."
log "Final tool: opencode"
log "Final session: $CURRENT_SESSION_ID"
