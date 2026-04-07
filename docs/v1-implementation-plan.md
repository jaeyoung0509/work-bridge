# sessionport v1 구현 딥다이브

## 1. 제품 해석

`sessionport`의 v1은 세션을 완전 복제하는 도구가 아니라, 서로 다른 coding-agent CLI 사이에서 "다음 작업을 바로 이어갈 수 있는 working-state bundle"을 만드는 로컬 우선 CLI다.

따라서 구현의 기준점은 아래 3가지다.

1. 가져올 수 있는 것은 최대한 가져오되, 손실은 명시적으로 드러낸다.
2. 모든 vendor 포맷은 canonical bundle로 먼저 정규화한다.
3. export 결과물은 target tool이 바로 읽을 수 있는 starter artifact여야 한다.

## 2. v1에서 확정할 제품 원칙

### 2.1 지원 우선순위

1. `Codex importer`
2. `Gemini importer`
3. `Claude partial importer`

이 순서로 가는 이유는 PRD의 전제와 동일하다. Codex/Gemini는 세션 및 설정 surface가 비교적 선명하고, Claude는 instruction/memory는 강하지만 raw session portability는 보수적으로 다뤄야 한다.

### 2.2 v1 산출물

v1이 반드시 만들어야 하는 사용자 가치는 아래다.

- 현재 머신/프로젝트에서 Claude, Gemini, Codex 관련 자산을 탐지할 수 있다.
- 특정 세션 또는 최신 세션을 canonical bundle로 가져올 수 있다.
- 대상 툴 기준으로 어떤 정보가 유지/축약/제외되는지 `doctor`로 보여줄 수 있다.
- 타겟 툴용 starter artifact를 생성할 수 있다.
- bundle을 `pack/unpack` 할 수 있다.

### 2.3 v1에서 의도적으로 포기하는 것

- hidden reasoning/state 복원
- secret 실값 export
- full transcript 무손실 변환
- hook/plugin 완전 이식
- daemon/sync/SaaS

## 3. 자산 지원 매트릭스

| Asset | Codex Source | Gemini Source | Claude Source | Canonical 처리 |
| --- | --- | --- | --- | --- |
| session metadata | Strong | Strong | Partial | 필수 |
| project root / cwd | Strong | Strong | Partial | 필수 |
| instruction files | Strong | Strong | Strong | 필수 |
| settings snapshot | Partial | Partial | Partial | allowlist 기반 |
| task title / current goal | Strong | Strong | Partial | 필수 |
| tool events | Partial | Partial | Partial | summary 중심 |
| touched files | Partial | Partial | Partial | dedupe 후 보존 |
| decisions / failures | Best-effort | Best-effort | Best-effort | summary 추출 |
| token stats | Best-effort | Best-effort | Best-effort | optional |
| resume hints | Strong | Strong | Partial | optional |
| secrets / tokens | Excluded | Excluded | Excluded | redacted |

`Strong / Partial / Best-effort`는 구현 acceptance 기준에도 그대로 반영한다.

## 4. v1 canonical schema v0

### 4.1 핵심 설계 원칙

- bundle은 사람이 읽을 수 있어야 한다.
- 모든 핵심 필드는 provenance를 추적할 수 있어야 한다.
- exporter는 source vendor를 직접 보지 않고 bundle만 읽어야 한다.
- 손실/제외 정보는 별도 report에서 숨기지 않는다.

### 4.2 주요 엔티티

#### `SessionBundle`

- `bundle_version`: 예. `v0`
- `bundle_id`: deterministic UUID 또는 hash
- `source_tool`: `codex | gemini | claude`
- `source_session_id`
- `imported_at`
- `project_root`
- `task_title`
- `current_goal`
- `summary`
- `instruction_artifacts`
- `settings_snapshot`
- `tool_events`
- `touched_files`
- `decisions`
- `failures`
- `resume_hints`
- `token_stats`
- `provenance`
- `redactions`
- `warnings`

#### `InstructionArtifact`

- `tool`
- `kind`: `project_instruction | user_memory | supplement`
- `path`
- `scope`: `global | project | local`
- `content`
- `content_hash`

#### `ToolEvent`

- `type`: `command | patch | read | write | search | tool_call`
- `summary`
- `timestamp`
- `status`
- `raw_ref`

#### `Decision`

- `summary`
- `reason`
- `confidence`
- `source_refs`

#### `Failure`

- `summary`
- `attempted_fix`
- `status`
- `source_refs`

#### `CompatibilityReport`

- `source_tool`
- `target_tool`
- `compatible_fields`
- `partial_fields`
- `unsupported_fields`
- `redacted_fields`
- `generated_artifacts`
- `warnings`

### 4.3 최소 JSON 예시

```json
{
  "bundle_version": "v0",
  "bundle_id": "spkg_01",
  "source_tool": "codex",
  "source_session_id": "abc123",
  "imported_at": "2026-04-07T10:00:00Z",
  "project_root": "/workspace/repo",
  "task_title": "add portability doctor command",
  "current_goal": "export a claude starter bundle",
  "summary": "User wants to continue work in Claude with project rules preserved.",
  "instruction_artifacts": [],
  "settings_snapshot": {
    "included": {},
    "excluded_keys": []
  },
  "tool_events": [],
  "touched_files": [],
  "decisions": [],
  "failures": [],
  "resume_hints": [],
  "token_stats": {},
  "provenance": [],
  "redactions": [],
  "warnings": []
}
```

## 5. 추천 Go 패키지 구조

```text
cmd/sessionport/main.go

internal/cli/
internal/domain/
internal/detect/
internal/importer/codex/
internal/importer/gemini/
internal/importer/claude/
internal/normalize/
internal/doctor/
internal/exporter/codex/
internal/exporter/gemini/
internal/exporter/claude/
internal/bundle/
internal/platform/fsx/
internal/platform/envx/
internal/platform/clockx/
internal/platform/archivex/
internal/platform/redact/
internal/render/

testdata/fixtures/
testdata/golden/
```

### 경계 규칙

- `detect/`는 경로 탐지만 한다. 파일 parsing은 하지 않는다.
- `importer/*`는 vendor-specific raw 읽기만 담당한다.
- `normalize/`는 raw import 결과를 `domain.SessionBundle`로 변환한다.
- `doctor/`는 bundle과 target capability matrix만 본다.
- `exporter/*`는 bundle만 입력으로 받는다.
- `platform/*`는 테스트 가능한 추상화 레이어다.

## 6. CLI 명령 계약

### `sessionport detect`

- 역할: 현재 머신/프로젝트에서 지원 툴의 설치/지침/설정 파일 후보를 찾는다.
- 출력: table 또는 JSON
- 실패 규칙: 일부 툴이 없어도 전체 명령은 성공하고 warning만 남긴다.

### `sessionport inspect <tool>`

- 역할: 특정 툴의 import 가능한 세션/설정/instruction inventory를 보여준다.
- 출력: 세션 id, last modified, root path, warnings
- 실패 규칙: 지원되지 않는 툴 이름만 hard fail

### `sessionport import --from <tool> --session <id|latest>`

- 역할: source 자산을 읽고 canonical bundle JSON 생성
- 출력: `bundle.json`
- 실패 규칙: 세션이 없으면 exit code 분리

### `sessionport doctor --from <tool> --session <id|latest> --target <tool>`

- 역할: import + normalize + compatibility report 생성
- 출력: human-readable report + JSON 옵션

### `sessionport export --bundle <file> --target <tool> --out <dir>`

- 역할: target-native starter artifact 생성
- 출력: 파일 생성 + export manifest

### `sessionport pack --from <tool> --session <id|latest> --out <file>`

- 역할: import 결과를 `.spkg`에 패키징
- 형식: zip 기반 `bundle + manifest + optional artifacts`

### `sessionport unpack --file <file> --target <tool>`

- 역할: `.spkg`를 풀고 즉시 export 가능한 상태로 변환

## 7. 구현상 중요한 정책 결정

### 7.1 `latest` 해석

- vendor별 최근 세션 판정 기준을 명시해야 한다.
- 기본은 `last modified timestamp` 우선, 없으면 vendor-specific session index 사용.
- `latest`가 애매하면 warning과 함께 후보 목록을 제시한다.

### 7.2 settings snapshot 정책

- settings는 전체 복사 금지
- allowlist 기반 포함만 허용
- secret 가능성이 있는 값은 key 단위로 redaction
- `excluded_keys`를 반드시 report한다

### 7.3 decisions / failures 추출 정책

- v1에서는 LLM 재요약이 아니라 deterministic rule 기반이 안전하다.
- 우선 source metadata, tool event labels, known markers에서 추출한다.
- 추출 불가능하면 빈 배열로 둔다. 억지 추론은 하지 않는다.

### 7.4 exporter 정책

- exporter는 bundle의 모든 필드를 target에 매핑하려고 하지 않는다.
- 생성물은 `instruction supplement`, `starter prompt`, `settings hints`, `manifest` 4종을 기본으로 한다.
- target이 native import 형식을 제공하지 않으면 plain text note로 degrade 한다.

## 8. 개발 시작 순서

### Phase 1. 기반 계층

- Go module + CLI skeleton
- canonical schema
- FS/Clock/Env abstraction
- fixture harness

### Phase 2. source read path

- detect
- inspect
- Codex importer
- Gemini importer
- Claude partial importer

### Phase 3. portability core

- normalize
- doctor
- exporters

### Phase 4. package + polish

- pack/unpack
- golden tests
- example bundles
- docs

## 9. 개발 착수 기준

아래가 모두 충족되면 바로 구현에 들어가도 된다.

1. canonical schema v0 확정
2. fixture 디렉터리 규칙 확정
3. importers가 직접 `os` 패키지에 의존하지 않도록 interface 경계 확정
4. doctor capability matrix 초안 확정
5. exporter 산출물 파일명 규칙 확정

이 5개가 먼저 고정되면 이후 vendor 포맷 변경에도 방어적으로 확장할 수 있다.
