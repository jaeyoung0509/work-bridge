# Native Session Import/Export/Apply Implementation Guide

이 문서는 `work-bridge`를 `project apply` 중심 도구에서 `project + native`를 명시적으로 지원하는 도구로 확장하기 위한 구현 가이드다. 목표는 코드를 당장 크게 뒤엎는 것이 아니라, 현재 코드베이스와 `todos/` 조사 결과를 기준으로 실제로 끝낼 수 있는 작업 순서와 테스트 절차를 고정하는 것이다.

## 1. 현재 상태 요약

### 1.1 이미 들어간 변경
- public CLI surface는 이미 `inspect`, `switch`, `export`, `version` 중심으로 정리되어 있다.
- `internal/domain/switch.go`에 `SwitchMode`가 추가되어 있고 `project` / `native` 모드 개념이 들어가 있다.
- `internal/cli/switch_command.go`, `internal/cli/export_command.go`에는 `--mode` flag plumbing이 일부 들어가 있다.
- `internal/switcher/service.go`는 `Mode`를 받아 `ApplyNativeProject` / `ExportNative`로 분기하려는 구조까지는 들어가 있다.

### 1.2 지금 실제로 깨진 상태
현재 워크트리는 native mode 리팩터링이 반쯤 들어간 상태다.

`go test ./...` 결과:

```text
# github.com/jaeyoung0509/work-bridge/internal/switcher
internal/switcher/apply.go:41:12: a.previewNative undefined
internal/switcher/apply.go:126:11: a.applyNative undefined
internal/switcher/apply.go:139:11: a.exportNative undefined
```

즉 지금 첫 번째 우선순위는 기능 추가가 아니라 **빌드 복구**다.

### 1.3 tool별 현재 갭

#### Codex
- inspect / import는 실제 JSONL 구조를 잘 읽고 있다.
- native apply/export는 아직 진짜 session tree write로 연결되지 않았다.
- `cwd` patch는 이미 핵심 요구사항이다.

#### Gemini
- inspect / import는 `projects.json`, `tmp/<slug>/chats/session-*.json` 기준으로 동작한다.
- native apply/export는 현재 managed output 중심이고, 실제 `~/.gemini`를 신뢰 가능한 방식으로 업데이트하지 않는다.

#### Claude
- inspect / import는 실제 세션 구조를 대체로 읽는다.
- native apply/export는 현재 `sessions-index.json` 삭제 같은 best-effort patch 수준이다.
- 실제 `~/.claude/projects/.../*.jsonl` 및 `history.jsonl` 생성/갱신이 필요하다.

#### OpenCode
- 가장 큰 갭이다.
- `internal/inspect/inspect.go`와 `internal/importer/opencode.go`는 아직 legacy JSON file 기반이다.
- 실제 저장소는 SQLite(`opencode.db`)이고, native apply/export는 raw DB write가 아니라 delegate 방식으로 가야 한다.
- 현재 `internal/switcher/apply_native.go`는 advisory warning만 남기고 실제 native integration이 아니다.
- 거기에 warning 문구도 현재 코드상 `opencode session import`를 가리키는데, 실제 CLI는 `opencode import <file>`, `opencode export <sessionID>`다.

## 2. 구현 원칙

### 2.1 mode는 명시적이어야 한다
- 기본값은 `project`
- `native`는 opt-in
- `native` 요청 시 project mode로 조용히 fallback 하면 안 된다
- 지원하지 않으면 `ERROR`로 끝내야 한다

### 2.2 project와 native는 다른 결과물이다
- `project` mode:
  - 프로젝트 안의 `CLAUDE.md`, `GEMINI.md`, `AGENTS.md`, `.work-bridge/...` 등을 관리
- `native` mode:
  - 각 툴의 실제 resume 가능한 native storage를 대상으로 함

### 2.3 OpenCode는 예외적으로 delegate 기반으로 간다
- inspect / import: SQLite 직접 읽기
- native apply / export: OpenCode runtime CLI delegate 사용
- 이번 단계에서는 SQLite direct write를 하지 않는다

## 3. 권장 구현 순서

## Phase 0. 빌드 복구

### 목표
- 현재 깨져 있는 `switcher` 빌드를 먼저 복구한다.
- 이 단계에서는 native가 완성되지 않아도 된다. 단, project / native routing 구조는 살아 있어야 한다.

### 수정 파일
- `/Users/apple/Myproject/sessionport/internal/switcher/apply.go`
- 새 파일 권장:
  - `/Users/apple/Myproject/sessionport/internal/switcher/native_mode.go`

### 구현
다음 메서드를 먼저 추가한다.

- `previewNative(payload, projectRoot, destinationOverride) (domain.SwitchPlan, error)`
- `applyNative(payload, plan) (domain.ApplyReport, error)`
- `exportNative(payload, plan) (domain.ApplyReport, error)`

초기 버전은 tool별 dispatch만 하고, 미구현 tool은 명시적으로 실패시켜도 된다.

예상 구조:

```go
func (a *projectAdapter) previewNative(...) (domain.SwitchPlan, error) {
	switch a.target {
	case domain.ToolCodex:
		return a.previewNativeCodex(...)
	case domain.ToolGemini:
		return a.previewNativeGemini(...)
	case domain.ToolClaude:
		return a.previewNativeClaude(...)
	case domain.ToolOpenCode:
		return a.previewNativeOpenCode(...)
	default:
		return domain.SwitchPlan{}, fmt.Errorf("unsupported native target %q", a.target)
	}
}
```

### acceptance
- `go test ./...`가 최소한 compile 단계는 통과해야 한다

## Phase 1. mode plumbing을 진짜 동작하게 만들기

### 목표
- `switch --mode project|native`
- `export --mode project|native`
가 실제 backend 경로를 다르게 타도록 만든다.

### 수정 파일
- `/Users/apple/Myproject/sessionport/internal/domain/switch.go`
- `/Users/apple/Myproject/sessionport/internal/switcher/service.go`
- `/Users/apple/Myproject/sessionport/internal/switcher/apply.go`
- `/Users/apple/Myproject/sessionport/internal/cli/switch_command.go`
- `/Users/apple/Myproject/sessionport/internal/cli/export_command.go`
- `/Users/apple/Myproject/sessionport/internal/switchui/tui.go`

### 해야 할 일
1. `SwitchPlan` / `ApplyReport`에 아래 필드를 고정
   - `Mode`
   - `DestinationRoot`
   - `Warnings`
   - `Status`

2. CLI 출력에 항상 아래를 포함
   - selected mode
   - destination
   - applied mode

3. TUI에도 mode toggle 추가
   - 키 제안: `m`
   - header와 result pane에 현재 mode 노출

4. native preview에서 지원 불가능한 경우 `READY`가 아니라 `ERROR`

### acceptance
- `work-bridge switch --mode native ...`에서 native preview / apply path가 실행됨
- project mode와 동일 출력으로 위장하지 않음

## Phase 2. Codex native 구현

### 목표
- Codex는 native export/apply를 filesystem write로 지원

### 대상 storage
- `~/.codex/session_index.jsonl`
- `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`

### 수정 파일
- 새 파일 권장:
  - `/Users/apple/Myproject/sessionport/internal/switcher/native_codex.go`
- 참고:
  - `/Users/apple/Myproject/sessionport/internal/importer/codex.go`
  - `/Users/apple/Myproject/sessionport/internal/inspect/inspect.go`
  - `/Users/apple/Myproject/sessionport/internal/platform/pathpatch/pathpatch.go`

### 구현 포인트
1. native export
   - export root 아래에 `.codex/` 트리를 실제 native layout으로 생성
   - `session_index.jsonl` + rollout JSONL 작성

2. native apply
   - 실제 codex home(`ToolPaths.Dir(domain.ToolCodex, homeDir)`)에 rollout 파일 쓰기
   - `session_meta.cwd`를 target project root로 patch
   - `session_index.jsonl` 갱신

3. checkpoint 처리
   - 존재 시 copy/preserve
   - target 버전과 안 맞으면 warning 명시

### acceptance
- fixture 기반으로 rollout JSONL이 실제 codex inspect에서 다시 발견됨
- `cwd` patch 확인

## Phase 3. Gemini native 구현

### 목표
- Gemini native export/apply를 실제 `~/.gemini` tree 기준으로 지원

### 대상 storage
- `~/.gemini/projects.json`
- `~/.gemini/tmp/<slug>/.project_root`
- `~/.gemini/tmp/<slug>/chats/session-*.json`

### 수정 파일
- 새 파일 권장:
  - `/Users/apple/Myproject/sessionport/internal/switcher/native_gemini.go`
- 참고:
  - `/Users/apple/Myproject/sessionport/internal/importer/gemini.go`
  - `/Users/apple/Myproject/sessionport/internal/inspect/inspect.go`
  - `/Users/apple/Myproject/sessionport/internal/switcher/apply_native.go`

### 구현 포인트
1. slug strategy를 하나로 고정
   - `projects.json`을 source of truth로 사용
   - fallback slug 생성 함수 하나만 유지

2. native export
   - export root 아래 `.gemini/` 트리를 실제 native layout으로 생성

3. native apply
   - target `projects.json` 업데이트
   - `.project_root` 생성
   - session JSON 내부 path patch

### acceptance
- inspect gemini가 exported / applied session을 다시 찾음
- `projects.json`과 `.project_root`가 일관됨

## Phase 4. Claude native 구현

### 목표
- Claude native export/apply를 실제 project session tree 기준으로 지원

### 대상 storage
- `~/.claude/projects/<encoded-project-dir>/<session-id>.jsonl`
- `~/.claude/history.jsonl`

### 수정 파일
- 새 파일 권장:
  - `/Users/apple/Myproject/sessionport/internal/switcher/native_claude.go`
- 참고:
  - `/Users/apple/Myproject/sessionport/internal/importer/claude.go`
  - `/Users/apple/Myproject/sessionport/internal/inspect/inspect.go`
  - `/Users/apple/Myproject/sessionport/internal/platform/pathpatch/pathpatch.go`

### 구현 포인트
1. encoded project dir naming을 현재 helper로 통일
2. native export
   - export root 아래 `.claude/projects/...` + `history.jsonl`
3. native apply
   - 실제 target store에 session JSONL 쓰기
   - `history.jsonl` append/update
   - `sessions-index.json` invalidation은 보조 작업으로 유지

### 주의
- project mode에서 하던 `CLAUDE.md` patch는 native mode의 필수 동작으로 넣지 않는다
- native mode는 session resume state가 중심이다

### acceptance
- target Claude inspect가 applied session을 다시 발견
- project hash/encoded dir mismatch 없음

## Phase 5. OpenCode inspect/import를 SQLite 기반으로 교체

이 단계가 가장 중요하다. 지금 OpenCode가 안 맞는 핵심 이유는 source read path 자체가 잘못돼 있기 때문이다.

### 목표
- OpenCode inspect / import에서 legacy JSON file 의존 제거
- SQLite(`opencode.db`)를 source of truth로 사용

### 수정 파일
- `/Users/apple/Myproject/sessionport/internal/inspect/inspect.go`
- `/Users/apple/Myproject/sessionport/internal/importer/opencode.go`
- 새 파일 권장:
  - `/Users/apple/Myproject/sessionport/internal/inspect/opencode_sqlite.go`
  - `/Users/apple/Myproject/sessionport/internal/importer/opencode_sqlite.go`

### 의존성
- `modernc.org/sqlite`

### 경로 해석
다음 순서로 찾는다.
1. `ToolPaths.OpenCode`
2. `~/.local/share/opencode/opencode.db`
3. `~/Library/Application Support/opencode/opencode.db`

### 읽어야 할 테이블
- `project`
- `workspace`
- `session`
- `message`
- `part`

### inspect 구현 기준
- session list는 `session` 테이블 기준
- title / updated_at / directory / workspace / project를 DB row에서 조립
- project root는 filename heuristic이 아니라 DB column 사용

### import 구현 기준
- 선택 session ID 기준으로:
  - session row
  - message rows
  - part rows
를 읽고 `RawImportResult` 생성

### 주의
- 기존 `session.StoragePath` JSON/JSONL 읽기 경로는 제거하거나 완전 fallback으로만 남긴다
- fallback을 남기더라도 notes/warnings에 명확히 남긴다

### acceptance
- fixture DB에서 inspect/import가 모두 동작
- legacy file가 없어도 OpenCode session을 읽을 수 있음

## Phase 6. OpenCode native export/apply를 delegate 기반으로 구현

### 목표
- OpenCode native mode는 raw SQLite write가 아니라 공식 runtime delegate 사용

### 실제 CLI 기준
반드시 다음 실제 command를 사용한다.

```bash
opencode export <sessionID>
opencode import <file>
```

`opencode session import/export`를 사용하면 안 된다.

### 수정 파일
- 새 파일 권장:
  - `/Users/apple/Myproject/sessionport/internal/switcher/native_opencode.go`

### 구현 포인트
1. capability probe
   - `lookPath("opencode")`
   - `opencode import --help` 또는 lightweight probe

2. native export
   - target이 OpenCode면 delegate-compatible payload를 export root에 stage
   - report에 `delegate payload`라고 명시

3. native apply
   - temp/staged json 파일 생성
   - `opencode import <file>` 실행
   - 실패 시 바로 `ERROR`
   - advisory warning만 남기고 성공 처리하면 안 됨

4. payload format
   - 실제 `opencode export <sessionID>` 결과 형태와 맞춰야 한다
   - 최소한 top-level:
     - `info`
     - `messages`
   - 각 message는:
     - `info`
     - `parts`

### 이번 단계에서 하지 않을 것
- SQLite direct write
- partial success pretending

### acceptance
- delegate 미설치 시 명시적 실패
- delegate 설치 시 import command가 실제로 실행됨

## Phase 7. README / UX 정리

### 목표
- project mode와 native mode의 차이를 숨기지 않음

### 수정 파일
- `/Users/apple/Myproject/sessionport/README.md`

### 반영 내용
- `--mode project|native`
- tool별 native 지원 수준 표
- OpenCode native는 delegate 기반이라고 명시
- native는 실제 resume state, project는 managed project files라는 점 명시

## 4. 파일 단위 체크리스트

### 꼭 수정해야 할 파일
- `/Users/apple/Myproject/sessionport/internal/domain/switch.go`
- `/Users/apple/Myproject/sessionport/internal/switcher/service.go`
- `/Users/apple/Myproject/sessionport/internal/switcher/apply.go`
- `/Users/apple/Myproject/sessionport/internal/inspect/inspect.go`
- `/Users/apple/Myproject/sessionport/internal/importer/opencode.go`
- `/Users/apple/Myproject/sessionport/internal/cli/switch_command.go`
- `/Users/apple/Myproject/sessionport/internal/cli/export_command.go`
- `/Users/apple/Myproject/sessionport/internal/switchui/tui.go`
- `/Users/apple/Myproject/sessionport/README.md`

### 새 파일 권장
- `/Users/apple/Myproject/sessionport/internal/switcher/native_mode.go`
- `/Users/apple/Myproject/sessionport/internal/switcher/native_codex.go`
- `/Users/apple/Myproject/sessionport/internal/switcher/native_gemini.go`
- `/Users/apple/Myproject/sessionport/internal/switcher/native_claude.go`
- `/Users/apple/Myproject/sessionport/internal/switcher/native_opencode.go`
- `/Users/apple/Myproject/sessionport/internal/inspect/opencode_sqlite.go`
- `/Users/apple/Myproject/sessionport/internal/importer/opencode_sqlite.go`

## 5. 테스트 전략

## 5.1 빠른 빌드 체크

```bash
cd /Users/apple/Myproject/sessionport
go test ./...
```

이 단계에서 깨지면 기능 테스트로 넘어가지 않는다.

## 5.2 로컬 빌드 실행

```bash
cd /Users/apple/Myproject/sessionport
mkdir -p ./bin
go build -o ./bin/work-bridge ./cmd/work-bridge
./bin/work-bridge version
```

## 5.3 CLI smoke test

### inspect

```bash
./bin/work-bridge inspect codex --limit 5
./bin/work-bridge inspect gemini --limit 5
./bin/work-bridge inspect claude --limit 5
./bin/work-bridge inspect opencode --limit 5
```

### project mode switch

```bash
./bin/work-bridge switch \
  --from gemini \
  --session latest \
  --to claude \
  --project /Users/apple/Myproject/sessionport \
  --mode project \
  --dry-run
```

### native mode switch

```bash
./bin/work-bridge switch \
  --from gemini \
  --session latest \
  --to claude \
  --project /Users/apple/Myproject/sessionport \
  --mode native \
  --dry-run
```

### native export

```bash
./bin/work-bridge export \
  --from codex \
  --session latest \
  --to gemini \
  --project /Users/apple/Myproject/sessionport \
  --mode native \
  --out /tmp/work-bridge-native-export
```

## 5.4 tool별 수동 검증

### Codex
```bash
./bin/work-bridge inspect codex --limit 10
codex resume
```

검증 포인트:
- applied/exported session이 list에 보이는지
- `cwd`가 현재 프로젝트와 맞는지

### Gemini
```bash
./bin/work-bridge inspect gemini --limit 10
cat ~/.gemini/projects.json
find ~/.gemini/tmp -name .project_root | head
```

검증 포인트:
- `projects.json` 등록
- `.project_root` 생성
- session JSON 재발견

### Claude
```bash
./bin/work-bridge inspect claude --limit 10
tail -n 5 ~/.claude/history.jsonl
find ~/.claude/projects -name '*.jsonl' | tail
```

검증 포인트:
- session file 생성
- history index 갱신

### OpenCode
```bash
./bin/work-bridge inspect opencode --limit 10
sqlite3 "$HOME/.local/share/opencode/opencode.db" '.tables'
opencode export <real-session-id> | head
```

검증 포인트:
- inspect가 DB 기반으로 세션을 읽는지
- native apply가 `opencode import`를 실제 호출하는지

## 5.5 테스트 코드 우선순위

### unit
- `internal/switcher/service_test.go`
  - mode dispatch
  - native unsupported error
- `internal/cli/public_commands_test.go`
  - `--mode project|native`
  - invalid mode
- `internal/switchui/tui_test.go`
  - mode toggle / preview label

### fixture/integration
- Codex/Gemini/Claude native export/apply fixture
- OpenCode SQLite fixture
- OpenCode delegate probe test

## 6. OpenCode 테스트 데이터 준비 방법

OpenCode는 fixture DB가 없으면 테스트 품질이 낮아진다. 아래 둘 중 하나로 간다.

### 방법 A. test에서 SQLite DB를 직접 생성
- `modernc.org/sqlite`로 temp DB 생성
- schema 생성
- `project`, `workspace`, `session`, `message`, `part` insert
- inspect/import 테스트 수행

이 방법이 가장 안정적이다.

### 방법 B. 실제 DB를 export한 sanitized fixture 사용
- 개인정보 제거
- absolute path를 fixture path로 치환
- repo의 `testdata/fixtures/opencode/` 아래 저장

## 7. 구현 중 금지할 것

- native mode 요청인데 project 결과를 성공처럼 리턴하는 것
- OpenCode에 대해 advisory warning만 남기고 `APPLIED` 처리하는 것
- `opencode session import/export` 같은 실제와 다른 CLI 가정
- source read를 여전히 legacy JSON 파일에 묶어 두는 것

## 8. 완료 정의

아래가 모두 만족되면 native implementation 1차가 끝난 것이다.

1. `go test ./...` green
2. `go build -o ./bin/work-bridge ./cmd/work-bridge` 성공
3. `switch/export --mode native`가 CLI/TUI에 명시적으로 노출
4. Codex/Gemini/Claude native export/apply가 실제 storage layout을 사용
5. OpenCode inspect/import는 SQLite 기반
6. OpenCode native apply/export는 delegate 기반
7. unsupported native path는 성공처럼 보이지 않고 명시적 실패
8. README가 project/native 차이를 정확히 설명

## 9. 작업 시작 추천 순서

바로 시작할 때는 이 순서가 가장 안전하다.

1. `go test ./...` compile 복구
2. mode dispatch tests 추가
3. Codex native 구현
4. Gemini native 구현
5. Claude native 구현
6. OpenCode SQLite inspect/import 교체
7. OpenCode delegate native 구현
8. TUI / README 마무리
