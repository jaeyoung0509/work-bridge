# sessionport v1 이슈 분할

이 문서는 바로 이슈 생성에 옮길 수 있게 `title`, `goal`, `deliverables`, `acceptance criteria`, `test notes`, `dependencies`까지 포함한다.

## 권장 마일스톤

- M1: 기반 구축과 canonical schema
- M2: detect/inspect + Codex/Gemini import
- M3: Claude partial import + normalize + doctor
- M4: exporters + pack/unpack + CLI golden tests

## SP-001 Bootstrap Go CLI skeleton

- Priority: P0
- Size: S
- Goal: 실행 가능한 Go CLI 뼈대와 기본 개발 루프를 만든다.
- Deliverables:
  - `go.mod`
  - `cmd/sessionport/main.go`
  - root command + help
  - `make test`, `make lint`, `make build`
  - CI workflow
- Acceptance Criteria:
  - `sessionport --help`가 정상 출력된다.
  - `go test ./...`가 로컬/CI에서 통과한다.
  - 린트/테스트/빌드가 CI에서 자동 실행된다.
- Test Notes:
  - root command stdout/stderr smoke test
  - exit code assertion
- Dependencies: 없음

## SP-002 Define canonical schema v0

- Priority: P0
- Size: M
- Goal: v1의 단일 source of truth가 되는 canonical model을 정의한다.
- Deliverables:
  - `internal/domain` structs
  - JSON marshal/unmarshal
  - bundle validation
  - `bundle_version = v0`
- Acceptance Criteria:
  - 필수 필드 validation이 동작한다.
  - JSON round-trip 후 의미 손실이 없다.
  - empty optional fields가 일관된 형태로 직렬화된다.
- Test Notes:
  - schema round-trip unit tests
  - invalid bundle negative tests
  - golden JSON tests
- Dependencies: SP-001

## SP-003 Add platform abstractions for FS, env, clock, archive

- Priority: P0
- Size: M
- Goal: vendor importer와 CLI가 실제 머신 상태에 직접 묶이지 않도록 추상화 계층을 만든다.
- Deliverables:
  - `fsx.FS`
  - `envx.Lookup`
  - `clockx.Clock`
  - `archivex.Reader/Writer`
  - `redact.Redactor`
- Acceptance Criteria:
  - importer/detect/pack code가 `os.UserHomeDir`, `os.Open` 등에 직접 의존하지 않는다.
  - 테스트에서 fake implementation으로 모든 주요 경로를 구동할 수 있다.
- Test Notes:
  - in-memory FS tests
  - fake env tests
  - fake clock deterministic timestamp tests
  - archive read/write round-trip tests
- Dependencies: SP-001

## SP-004 Build fixture and golden test harness

- Priority: P0
- Size: M
- Goal: vendor 포맷을 fixture로 고정해 parser와 exporter를 unit-testable하게 만든다.
- Deliverables:
  - `testdata/fixtures/<tool>/<case>/...`
  - fixture loader helpers
  - golden file assertion helpers
- Acceptance Criteria:
  - 새 fixture 추가만으로 importer/exporter 테스트 케이스를 쉽게 확장할 수 있다.
  - golden update workflow가 문서화된다.
- Test Notes:
  - loader tests
  - missing fixture negative tests
  - golden mismatch diff tests
- Dependencies: SP-002, SP-003

## SP-005 Implement detector registry and `detect` command

- Priority: P0
- Size: M
- Goal: 로컬 설치 및 프로젝트 자산 탐지 기능을 구현한다.
- Deliverables:
  - detector registry
  - tool installation detection
  - project instruction file detection
  - `sessionport detect`
- Acceptance Criteria:
  - Claude/Gemini/Codex의 존재 여부와 주요 경로를 보고한다.
  - 특정 툴이 없어도 다른 툴 탐지는 계속된다.
  - JSON 출력 모드가 지원된다.
- Test Notes:
  - fake home dir fixtures
  - missing config path tests
  - partial detection warning tests
- Dependencies: SP-003, SP-004

## SP-006 Implement `inspect <tool>` inventory command

- Priority: P0
- Size: M
- Goal: import 전에 세션/설정/instruction inventory를 확인할 수 있게 한다.
- Deliverables:
  - `inspect codex`
  - `inspect gemini`
  - `inspect claude`
  - 공통 inventory renderer
- Acceptance Criteria:
  - import 가능한 세션 id, modified time, root path를 보여준다.
  - 손상된 파일은 warning으로 표기하고 전체 명령은 유지된다.
  - `--json` 옵션이 동작한다.
- Test Notes:
  - per-tool inventory fixture tests
  - broken metadata file negative tests
  - renderer snapshot tests
- Dependencies: SP-005

## SP-007 Implement Codex importer

- Priority: P0
- Size: L
- Goal: Codex session/config/instruction 데이터를 raw import 결과로 읽어온다.
- Deliverables:
  - Codex path resolver
  - session reader (`latest`, explicit id)
  - settings allowlist reader
  - `AGENTS.md` importer
  - raw source snapshot model
- Acceptance Criteria:
  - 최신 세션 또는 명시적 세션 id import가 가능하다.
  - instruction file과 settings snapshot이 함께 수집된다.
  - 읽을 수 없는 필드는 warning으로 남기고 나머지는 계속 가져온다.
- Test Notes:
  - basic fixture
  - missing session file fixture
  - corrupt config fixture
  - secret-bearing config redaction fixture
- Dependencies: SP-004, SP-006

## SP-008 Implement Gemini importer

- Priority: P0
- Size: L
- Goal: Gemini session/history/settings/instruction 자산을 raw import 결과로 읽는다.
- Deliverables:
  - Gemini path resolver
  - session inventory + session load
  - settings snapshot reader
  - `GEMINI.md` importer
- Acceptance Criteria:
  - session metadata, project root, instruction files가 추출된다.
  - 지원되지 않는 필드는 reportable warning으로 정리된다.
  - session selection 방식이 Codex와 같은 CLI contract를 따른다.
- Test Notes:
  - basic fixture
  - multi-session fixture
  - missing settings fixture
  - malformed history fixture
- Dependencies: SP-004, SP-006

## SP-009 Implement Claude partial importer

- Priority: P1
- Size: M
- Goal: Claude는 partial support 기준으로 instruction/memory/settings 중심 import를 구현한다.
- Deliverables:
  - Claude path resolver
  - `CLAUDE.md` importer
  - auto memory / settings partial reader
  - partial support warnings
- Acceptance Criteria:
  - instruction/memory/settings 자산을 읽을 수 있다.
  - raw session portability가 제한된 이유가 report에 명시된다.
  - partial support policy가 code와 docs에서 일치한다.
- Test Notes:
  - instruction-only fixture
  - memory-only fixture
  - unsupported session field fixture
- Dependencies: SP-004, SP-006

## SP-010 Normalize raw imports into `SessionBundle`

- Priority: P0
- Size: L
- Goal: vendor별 raw import 결과를 canonical bundle로 변환한다.
- Deliverables:
  - raw import interface
  - normalizer pipeline
  - provenance tracking
  - warnings/redactions aggregation
- Acceptance Criteria:
  - 모든 importer 결과가 동일한 `SessionBundle`로 변환된다.
  - 필수 필드 누락 시 명시적 validation error가 난다.
  - source별 손실 정보가 provenance/warnings로 남는다.
- Test Notes:
  - importer-to-bundle contract tests
  - required field negative tests
  - provenance snapshot tests
- Dependencies: SP-002, SP-007, SP-008, SP-009

## SP-011 Implement doctor compatibility engine

- Priority: P0
- Size: M
- Goal: source bundle이 target tool에서 얼마나 재수화 가능한지 정량적으로 보여준다.
- Deliverables:
  - target capability matrix
  - `CompatibilityReport`
  - `doctor` command
  - text/json renderer
- Acceptance Criteria:
  - compatible/partial/unsupported/redacted 필드가 구분된다.
  - 어떤 정보가 요약/축약되었는지 설명할 수 있다.
  - report 결과가 exporter와 모순되지 않는다.
- Test Notes:
  - matrix-driven unit tests
  - per-target golden report tests
  - unsupported field negative tests
- Dependencies: SP-010

## SP-012 Implement Codex exporter

- Priority: P0
- Size: M
- Goal: bundle을 Codex starter artifact로 재수화한다.
- Deliverables:
  - `AGENTS.md` supplement generator
  - starter prompt generator
  - config hints generator
  - export manifest entries
- Acceptance Criteria:
  - 출력 디렉터리에 예측 가능한 파일명이 생성된다.
  - bundle의 instruction/goal/decision이 읽기 좋은 형태로 반영된다.
  - unsupported 필드는 manifest에 기록된다.
- Test Notes:
  - golden file tests
  - empty bundle negative tests
  - path collision tests
- Dependencies: SP-010, SP-011

## SP-013 Implement Gemini exporter

- Priority: P0
- Size: M
- Goal: bundle을 Gemini starter artifact로 재수화한다.
- Deliverables:
  - `GEMINI.md` supplement generator
  - starter prompt generator
  - settings patch hints
  - export manifest entries
- Acceptance Criteria:
  - Gemini 친화적인 instruction artifact가 생성된다.
  - partial field는 note/manifest로 degrade 된다.
  - 생성 파일이 deterministic하다.
- Test Notes:
  - golden file tests
  - partial compatibility tests
  - duplicate instruction dedupe tests
- Dependencies: SP-010, SP-011

## SP-014 Implement Claude exporter

- Priority: P0
- Size: M
- Goal: bundle을 Claude starter artifact로 재수화한다.
- Deliverables:
  - `CLAUDE.md` supplement generator
  - starter prompt generator
  - memory note generator
  - export manifest entries
- Acceptance Criteria:
  - Claude partial import의 제한을 exporter 문서에도 반영한다.
  - instruction/decision/failure가 concise note로 정리된다.
  - report와 export 결과가 일치한다.
- Test Notes:
  - golden file tests
  - partial-only bundle tests
  - warning propagation tests
- Dependencies: SP-010, SP-011

## SP-015 Implement bundle pack/unpack

- Priority: P1
- Size: M
- Goal: canonical bundle을 `.spkg`로 묶고 다시 풀 수 있게 한다.
- Deliverables:
  - pack manifest
  - zip writer/reader
  - `pack` command
  - `unpack` command
- Acceptance Criteria:
  - pack 후 unpack 시 bundle 무결성이 유지된다.
  - archive 내부 파일 순서/이름이 deterministic하다.
  - 손상된 archive는 명시적 에러로 처리된다.
- Test Notes:
  - archive round-trip tests
  - corrupt archive negative tests
  - deterministic hash tests
- Dependencies: SP-003, SP-010

## SP-016 Wire end-to-end CLI flows and golden snapshots

- Priority: P0
- Size: L
- Goal: detect → inspect → import → doctor → export 전체 흐름을 CLI 수준에서 검증한다.
- Deliverables:
  - subcommand wiring
  - shared flags/options
  - end-to-end golden snapshots
  - example fixtures for README
- Acceptance Criteria:
  - 주요 CLI 명령 흐름이 fixture 기반으로 회귀 테스트된다.
  - human-readable output과 JSON output 둘 다 검증된다.
  - exit code contract가 명확하다.
- Test Notes:
  - fixture-based command tests
  - stdout/stderr snapshot tests
  - exit code matrix tests
- Dependencies: SP-011, SP-012, SP-013, SP-014, SP-015

## SP-017 Add release docs and contributor guide

- Priority: P2
- Size: S
- Goal: 신규 contributor가 fixture를 추가하고 새로운 vendor 포맷을 대응할 수 있게 문서화한다.
- Deliverables:
  - local dev guide
  - fixture authoring guide
  - compatibility policy doc
- Acceptance Criteria:
  - contributor가 fixture 추가 절차를 문서만 보고 따라갈 수 있다.
  - unsupported policy와 redaction policy가 문서화된다.
- Test Notes:
  - 문서 이슈이므로 별도 unit test 없음
- Dependencies: SP-016

## 병렬화 추천

아래처럼 나누면 충돌이 적다.

- Track A: SP-001, SP-002, SP-003, SP-004
- Track B: SP-005, SP-006
- Track C: SP-007, SP-008
- Track D: SP-009, SP-010
- Track E: SP-011, SP-012, SP-013, SP-014
- Track F: SP-015, SP-016, SP-017

## 첫 주 착수 권장 순서

1. SP-001
2. SP-002
3. SP-003
4. SP-004
5. SP-005
6. SP-007
7. SP-008

이 순서로 가면 첫 주 안에 Codex/Gemini importer를 테스트 가능한 형태로 붙일 수 있다.
