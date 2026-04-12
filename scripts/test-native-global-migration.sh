#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd -P)"

WORK_BRIDGE_BIN="$REPO_ROOT/bin/work-bridge"
PROJECT_ROOT="$(pwd -P)"
START_SESSION_ID=""
LIMIT=100
AUTO_BUILD=1

usage() {
  cat <<'EOF'
Usage:
  scripts/test-native-global-migration.sh [options]

Runs a native migration chain and verifies that user-scope/global skills and MCP
settings are applied at each hop:

  codex -> gemini -> claude -> opencode

The script captures the source tool's global skills and MCP servers before each
hop, runs `work-bridge switch --mode native`, then checks that those same names
appear in the target tool's user-scope config layout.

Options:
  -p, --project PATH      Project root to use. Default: current directory.
  -b, --bin PATH          Path to the work-bridge binary.
  -s, --session ID        Starting Codex session ID. Default: auto-detect.
      --limit N           Inspect limit when resolving sessions. Default: 100.
      --skip-build        Do not auto-build ./bin/work-bridge if it is missing.
  -h, --help             Show this help message.
EOF
}

log() {
  printf '[global-test] %s\n' "$*"
}

die() {
  printf '[global-test] error: %s\n' "$*" >&2
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

normalize_skill_name() {
  printf '%s' "$1" \
    | tr '[:upper:]' '[:lower:]' \
    | sed 's/[^a-z0-9]/-/g' \
    | sed 's/--*/-/g' \
    | sed 's/^-//' \
    | sed 's/-$//'
  printf '\n'
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

  log "Running ${from_tool} -> ${to_tool} with session ${session_id}"
  "$WORK_BRIDGE_BIN" switch \
    --from "$from_tool" \
    --session "$session_id" \
    --to "$to_tool" \
    --project "$PROJECT_ROOT" \
    --mode native
}

skill_dir_for_tool() {
  local tool="$1"
  local home_dir
  home_dir="${HOME:?HOME is not set}"

  case "$tool" in
    codex) printf '%s\n' "$home_dir/.codex/skills" ;;
    claude) printf '%s\n' "$home_dir/.claude/skills" ;;
    opencode) printf '%s\n' "$home_dir/.config/opencode/skills" ;;
    gemini) printf '%s\n' "$home_dir/.gemini" ;;
    *) return 1 ;;
  esac
}

mcp_config_for_tool() {
  local tool="$1"
  local home_dir
  home_dir="${HOME:?HOME is not set}"

  case "$tool" in
    codex) printf '%s\n' "$home_dir/.codex/config.toml" ;;
    claude) printf '%s\n' "$home_dir/.claude/settings.json" ;;
    gemini) printf '%s\n' "$home_dir/.gemini/settings.json" ;;
    opencode)
      if [[ -f "$home_dir/.config/opencode/opencode.jsonc" ]]; then
        printf '%s\n' "$home_dir/.config/opencode/opencode.jsonc"
      else
        printf '%s\n' "$home_dir/.config/opencode/opencode.json"
      fi
      ;;
    *) return 1 ;;
  esac
}

collect_skill_names() {
  local tool="$1"
  local dir
  dir="$(skill_dir_for_tool "$tool")"

  case "$tool" in
    gemini)
      local file="$dir/GEMINI.md"
      if [[ ! -f "$file" ]]; then
        return 0
      fi
      awk '
        /## work-bridge imported global skills/ { in_block=1; next }
        /<!-- work-bridge:end -->/ { in_block=0 }
        in_block && /^### / {
          value = $0
          sub(/^### /, "", value)
          gsub(/^"/, "", value)
          gsub(/"$/, "", value)
          if (value ~ /^[a-z0-9-]+$/) {
            print value
          }
        }
      ' "$file" \
        | sed '/^[[:space:]]*$/d' \
        | while IFS= read -r name; do normalize_skill_name "$name"; done \
        | sed '/^[[:space:]]*$/d' \
        | sort -u
      ;;
    opencode)
      if [[ ! -d "$dir" ]]; then
        return 0
      fi
      {
        find "$dir" -maxdepth 2 -type f -name 'SKILL.md' -print 2>/dev/null \
          | xargs -I{} dirname "{}" \
          | xargs -I{} basename "{}"
        find "$dir" -maxdepth 1 -type f -name '*.md' -print 2>/dev/null \
          | sed 's#^.*/##' \
          | sed 's/\.md$//'
      } \
        | sed '/^[[:space:]]*$/d' \
        | while IFS= read -r name; do normalize_skill_name "$name"; done \
        | sort -u
      ;;
    *)
      if [[ ! -d "$dir" ]]; then
        return 0
      fi
      find "$dir" -maxdepth 1 -type f -name '*.md' -print 2>/dev/null \
        | sed 's#^.*/##' \
        | sed 's/\.md$//' \
        | sed '/^[[:space:]]*$/d' \
        | while IFS= read -r name; do normalize_skill_name "$name"; done \
        | sort -u
      ;;
  esac
}

collect_mcp_names() {
  local tool="$1"
  local file
  file="$(mcp_config_for_tool "$tool")"
  [[ -f "$file" ]] || return 0

  case "$tool" in
    codex)
      sed -n 's/^\[mcp\.servers\.\([^.[:space:]]*\)\]$/\1/p' "$file" | sort -u
      ;;
    opencode)
      python3 - "$file" <<'PY'
import json, sys
path = sys.argv[1]
with open(path, 'r', encoding='utf-8') as fh:
    text = fh.read()
clean = []
in_string = False
escaped = False
line_comment = False
block_comment = False
i = 0
while i < len(text):
    ch = text[i]
    if line_comment:
        if ch == '\n':
            line_comment = False
            clean.append(ch)
        i += 1
        continue
    if block_comment:
        if ch == '*' and i + 1 < len(text) and text[i + 1] == '/':
            block_comment = False
            i += 2
            continue
        if ch == '\n':
            clean.append(ch)
        i += 1
        continue
    if in_string:
        clean.append(ch)
        if escaped:
            escaped = False
        elif ch == '\\':
            escaped = True
        elif ch == '"':
            in_string = False
        i += 1
        continue
    if ch == '"':
        in_string = True
        clean.append(ch)
        i += 1
        continue
    if ch == '/' and i + 1 < len(text):
        nxt = text[i + 1]
        if nxt == '/':
            line_comment = True
            i += 2
            continue
        if nxt == '*':
            block_comment = True
            i += 2
            continue
    clean.append(ch)
    i += 1

sanitized = ''.join(clean)
out = []
in_string = False
escaped = False
i = 0
while i < len(sanitized):
    ch = sanitized[i]
    if in_string:
        out.append(ch)
        if escaped:
            escaped = False
        elif ch == '\\':
            escaped = True
        elif ch == '"':
            in_string = False
        i += 1
        continue
    if ch == '"':
        in_string = True
        out.append(ch)
        i += 1
        continue
    if ch == ',':
        j = i + 1
        while j < len(sanitized) and sanitized[j] in ' \t\r\n':
            j += 1
        if j < len(sanitized) and sanitized[j] in '}]':
            i += 1
            continue
    out.append(ch)
    i += 1

data = json.loads(''.join(out))
for name in sorted((data.get("mcp") or {})):
    print(name)
PY
      ;;
    *)
      jq -r '(.mcpServers // {}) | keys[]?' "$file" | sort -u
      ;;
  esac
}

verify_names_present() {
  local kind="$1"
  local target_tool="$2"
  local expected_names="$3"
  local actual_names="$4"

  if [[ -z "$expected_names" ]]; then
    log "No source ${kind} entries found for ${target_tool} hop"
    return 0
  fi

  local missing=()
  while IFS= read -r name; do
    [[ -n "$name" ]] || continue
    if ! printf '%s\n' "$actual_names" | grep -Fxq "$name"; then
      missing+=("$name")
    fi
  done <<<"$expected_names"

  if (( ${#missing[@]} > 0 )); then
    die "${kind} verification failed for ${target_tool}; missing: ${missing[*]}"
  fi
}

verify_gemini_skills_present() {
  local expected_names="$1"
  local file="${HOME:?HOME is not set}/.gemini/GEMINI.md"
  [[ -f "$file" ]] || die "skill verification failed for gemini; missing GEMINI.md"

  local missing=()
  while IFS= read -r name; do
    [[ -n "$name" ]] || continue
    if ! grep -Fqi "$name" "$file"; then
      missing+=("$name")
    fi
  done <<<"$expected_names"

  if (( ${#missing[@]} > 0 )); then
    die "skill verification failed for gemini; missing: ${missing[*]}"
  fi
}

verify_target_state() {
  local from_tool="$1"
  local to_tool="$2"
  local source_skills="$3"
  local source_mcp="$4"
  local target_skills target_mcp

  target_skills="$(collect_skill_names "$to_tool" || true)"
  target_mcp="$(collect_mcp_names "$to_tool" || true)"

  if [[ "$to_tool" == "gemini" ]]; then
    verify_gemini_skills_present "$source_skills"
  else
    verify_names_present "skill" "$to_tool" "$source_skills" "$target_skills"
  fi
  verify_names_present "MCP server" "$to_tool" "$source_mcp" "$target_mcp"

  log "Verified ${from_tool} -> ${to_tool} skills: ${source_skills:-<none>}"
  log "Verified ${from_tool} -> ${to_tool} MCP: ${source_mcp:-<none>}"
}

CHAIN=(codex gemini claude opencode)
CURRENT_SESSION_ID="$START_SESSION_ID"
CURRENT_SKILLS=""
CURRENT_MCP=""

if [[ -z "$CURRENT_SESSION_ID" ]]; then
  CURRENT_SESSION_ID="$(resolve_session_id codex)"
fi

CURRENT_SKILLS="$(collect_skill_names codex || true)"
CURRENT_MCP="$(collect_mcp_names codex || true)"

log "Project root: $PROJECT_ROOT"
log "Starting Codex session: $CURRENT_SESSION_ID"

for ((i = 0; i < ${#CHAIN[@]} - 1; i++)); do
  SOURCE_TOOL="${CHAIN[$i]}"
  TARGET_TOOL="${CHAIN[$((i + 1))]}"

  CURRENT_SESSION_ID="$(resolve_session_id "$SOURCE_TOOL" "$CURRENT_SESSION_ID")"

  run_switch "$SOURCE_TOOL" "$CURRENT_SESSION_ID" "$TARGET_TOOL"

  verify_target_state "$SOURCE_TOOL" "$TARGET_TOOL" "$CURRENT_SKILLS" "$CURRENT_MCP"

  if [[ "$TARGET_TOOL" == "opencode" ]]; then
    log "Skipping session-id confirmation for opencode; global migration verification is complete."
    continue
  fi

  NEXT_SESSION_ID="$(wait_for_target_session "$TARGET_TOOL" "$CURRENT_SESSION_ID")"
  CURRENT_SESSION_ID="$NEXT_SESSION_ID"
done

log "Completed global migration verification."
log "Final tool: opencode"
log "Final session: $CURRENT_SESSION_ID"
