# sessionport 테스트 및 모킹 전략

이 프로젝트는 vendor 로컬 포맷을 다루기 때문에, 실제 사용자 머신에 의존하는 테스트보다 fixture 기반 unit test가 훨씬 중요하다. v1의 테스트 전략은 "실제 파일을 최대한 닮은 fixture + 얇은 추상화 + golden output"에 맞춘다.

## 1. 테스트 원칙

1. 실제 홈 디렉터리를 읽는 테스트는 금지한다.
2. vendor 포맷 대응은 fixture 추가로 검증한다.
3. importers/exporters는 unit test가 메인이고, end-to-end는 회귀 방지용 최소 세트만 둔다.
4. secret 관련 값은 테스트 fixture에서도 dummy 값만 사용한다.
5. deterministic output이 가능한 모든 곳에 golden test를 둔다.

## 2. 테스트 피라미드

### Unit tests

가장 많은 수를 차지해야 한다.

- detector path resolution
- settings parsing
- session metadata parsing
- bundle validation
- compatibility matrix
- exporter text generation
- archive pack/unpack

### Fixture integration tests

실제 vendor 디렉터리 구조를 흉내 내는 테스트다.

- `inspect`
- `import`
- `doctor`
- `export`

### CLI snapshot tests

최소 수량만 유지한다.

- `--help`
- 대표 happy path
- 대표 degraded path
- 대표 error path

## 3. 반드시 추상화할 인터페이스

아래는 실제 구현 전에 고정해야 하는 핵심 mock points다.

```go
type FS interface {
    ReadFile(name string) ([]byte, error)
    WriteFile(name string, data []byte, perm fs.FileMode) error
    Stat(name string) (fs.FileInfo, error)
    ReadDir(name string) ([]fs.DirEntry, error)
    MkdirAll(path string, perm fs.FileMode) error
}

type Env interface {
    LookupEnv(key string) (string, bool)
}

type Clock interface {
    Now() time.Time
}

type ArchiveWriter interface {
    WritePackage(dst string, files []ArchiveFile) error
}

type ArchiveReader interface {
    ReadPackage(src string) ([]ArchiveFile, error)
}
```

핵심 규칙은 간단하다. `detect`, `importer`, `bundle`, `exporter`가 직접 `os`, `time`, 실제 사용자 홈 디렉터리, 실제 환경변수에 강하게 묶이면 안 된다.

## 4. fixture 디렉터리 규칙

권장 구조는 아래와 같다.

```text
testdata/
  fixtures/
    codex/
      basic_latest/
        input/
          home/.codex/config.toml
          workspace/AGENTS.md
          sessions/abc123.json
        expected/
          inspect.json
          bundle.json
          doctor_to_claude.json
          export_codex/
            AGENTS.sessionport.md
            STARTER_PROMPT.md
            manifest.json
      corrupt_config/
      missing_session/
      secret_redaction/
    gemini/
      basic_latest/
      malformed_history/
      multi_session/
    claude/
      instruction_only/
      memory_only/
      partial_session/
```

### fixture naming 규칙

- `basic_latest`: 정상 최신 세션
- `explicit_session`: 명시적 세션 id
- `missing_*`: 일부 파일 없음
- `corrupt_*`: 파싱 불가
- `partial_*`: 일부 필드만 지원
- `secret_*`: redaction 검증

## 5. golden test 대상

golden이 특히 유효한 대상은 아래다.

- canonical bundle JSON
- doctor JSON
- doctor human-readable report
- exporter가 생성한 markdown/text files
- CLI stdout/stderr

golden 대상이 아닌 것은 아래다.

- 절대 경로 전체 문자열
- OS별 timestamp 포맷 차이가 나는 값
- map iteration 순서에 영향받는 비정형 출력

그런 필드는 normalize 후 비교하거나 placeholder로 치환해야 한다.

## 6. importer 테스트 설계

각 importer는 최소 아래 케이스를 가져야 한다.

### 공통 happy path

- latest session import
- explicit session import
- instruction file import
- settings allowlist import

### 공통 degraded path

- session file missing
- settings file missing
- malformed JSON/TOML
- unreadable file permission
- unsupported vendor field

### 공통 security path

- env-like key redaction
- token-like value exclusion
- excluded key reporting

## 7. normalizer contract tests

가장 중요한 테스트 중 하나다. 모든 importer는 최종적으로 같은 bundle contract를 만족해야 한다.

검증 포인트:

- `source_tool`이 정확하다.
- `project_root`가 비어 있지 않다.
- instruction artifacts가 scope와 path를 가진다.
- warnings/redactions가 누락 없이 합쳐진다.
- missing optional field가 잘못된 기본값으로 채워지지 않는다.

이 테스트를 통과하지 못하면 exporter와 doctor가 흔들린다.

## 8. doctor 규칙 테스트

doctor는 if/else가 늘어나기 쉬우므로 matrix-driven test로 고정하는 게 낫다.

예시:

| Field | Codex Target | Gemini Target | Claude Target |
| --- | --- | --- | --- |
| instruction_artifacts | compatible | compatible | compatible |
| settings_snapshot | partial | partial | partial |
| raw tool outputs | partial | partial | partial |
| secrets | redacted | redacted | redacted |
| hidden reasoning | unsupported | unsupported | unsupported |

이 표를 그대로 test matrix로 코드화하면 규칙 drift를 줄일 수 있다.

## 9. exporter 테스트 설계

각 exporter는 입력 bundle 하나에 대해 deterministic file set을 생성해야 한다.

반드시 검증할 것:

- 파일명
- 파일 수
- 파일 본문
- manifest 내용
- warning propagation
- overwrite policy

권장 방식:

- temp dir + fake FS
- generated file tree snapshot
- manifest JSON golden

## 10. CLI 테스트 설계

CLI는 parser보다 command contract를 검증하는 용도로만 둔다.

최소 커버:

- `detect --json`
- `inspect codex --json`
- `import --from codex --session latest`
- `doctor --from codex --session latest --target claude`
- `export --bundle fixture.json --target gemini --out tmp`
- `pack` / `unpack`

exit code도 같이 고정하는 것이 좋다.

- `0`: success or degraded success
- `2`: usage error
- `3`: source session not found
- `4`: parse failure with no recoverable output
- `5`: export failure

## 11. 회귀를 잘 잡는 fixture 세트

처음부터 아래 케이스는 꼭 만들어 두는 게 좋다.

1. Codex basic latest
2. Gemini basic latest
3. Claude instruction only
4. missing session file
5. malformed settings
6. secret redaction
7. multi instruction hierarchy
8. empty touched files
9. unsupported vendor-specific option
10. pack/unpack round-trip

## 12. 구현 중 피해야 할 테스트 안티패턴

- 실제 사용자 홈 디렉터리에서 live read
- network access를 기대하는 테스트
- 현재 날짜/시간에 따라 변하는 assertion
- vendor CLI가 설치되어 있어야 통과하는 테스트
- snapshot 갱신이 너무 쉬워서 regression을 가리는 구조

## 13. 추천 Definition of Done

각 이슈는 아래를 만족해야 끝난 것으로 본다.

1. happy path unit tests
2. degraded path unit tests
3. secret/redaction 관련 테스트
4. golden 또는 contract test 최소 1개
5. docs 또는 fixture 업데이트

이 기준을 유지하면 vendor 포맷이 바뀌어도 수리 범위를 빨리 좁힐 수 있다.
