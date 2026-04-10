# Gemini CLI 세션 스토리지 아키텍처 및 상호 운용성 분석

본 문서는 `Gemini CLI`의 내부 세션 저장 방식 및 프로젝트 식별 알고리즘을 분석하여, 타 에이전트(Claude Code, Codex CLI, OpenCode)와의 세션 임포트/익스포트 및 상호 운용성을 위한 기술적 가이드를 제공합니다.

## 1. 스토리지 아키텍처 개요 (macOS 기준)

Gemini CLI는 모든 전역 설정 및 프로젝트별 임시 데이터를 사용자의 홈 디렉토리 하위의 고유 폴더에 관리합니다.

- **기본 경로:** `~/.gemini/` (환경 변수 `GEMINI_CLI_HOME`으로 변경 가능)
- **프로젝트 레지스트리:** `~/.gemini/projects.json` (절대 경로와 슬러그(Slug) 매핑 저장)
- **전역 설정:** `~/.gemini/settings.json`

## 2. 프로젝트 식별 및 경로 결정 알고리즘

Gemini CLI는 프로젝트를 식별하기 위해 과거에는 SHA-256 해시를 사용했으나, 현재는 가독성이 좋은 **슬러그(Slug)** 기반의 식별자로 마이그레이션되었습니다.

### 2.1. 프로젝트 식별자 (ID) 생성
- **SHA-256 (레거시):** 프로젝트 절대 경로의 SHA-256 해시값 (예: `7a1b...`).
- **슬러그 (현재):** 프로젝트 폴더 이름을 기반으로 생성된 고유 문자열 (예: `gemini-cli`, `gemini-cli-1`).
- **결정 방식:** `ProjectRegistry` 클래스가 `projects.json`을 조회하여 매핑된 슬러그가 있는지 확인하고, 없으면 새로 생성하여 저장합니다.

### 2.2. 소유권 검증 (Ownership Marker)
각 프로젝트 스토리지 폴더 내에는 `.project_root` 파일이 존재합니다.
- **파일 경로:** `~/.gemini/tmp/<id>/.project_root`
- **내용:** 해당 스토리지 폴더가 속한 프로젝트의 **로컬 절대 경로** (텍스트 형식).
- **역할:** 타겟 기기로 세션 이동 시, 이 파일의 내용을 타겟 기기의 경로로 수정하지 않으면 Gemini CLI가 해당 폴더를 유효한 스토리지로 인식하지 못합니다.

## 3. 프로젝트별 세부 저장 구조

모든 프로젝트 데이터는 `~/.gemini/tmp/<id>/` 하위에 저장됩니다.

| 구성 요소 | 저장 경로 | 데이터 형식 및 특징 |
| :--- | :--- | :--- |
| **대화 내역 (Sessions)** | `chats/` | `session-<uuid>.json` 또는 `checkpoint-<name>.json`. JSON Object 형식 (JSONL 아님). `messages` 배열 내에 `role` (user/model)과 `parts` 포함. |
| **쉘 히스토리** | `shell_history` | 해당 프로젝트 내에서 실행된 터미널 명령어 이력. |
| **실행 계획 (Plans)** | `<sessionId>/plans/` | 모델이 수립한 다단계 실행 계획(Markdown/JSON). |
| **태스크 추적** | `<sessionId>/tasks/` | 세션 내 하위 태스크의 성공/실패 상태 정보. |
| **체크포인트** | `checkpoints/` | `checkpoint` 명령어로 명시적으로 저장된 상태. |
| **로그** | `logs/` | 도구 실행 결과 및 내부 디버깅 로그. |
| **지속적 히스토리** | `~/.gemini/history/<id>/` | Git 섀도우 레포지토리 등 영구적인 프로젝트 변경 이력. |

## 4. `work-bridge`를 위한 임포트/익스포트 전략

기기 간 또는 에이전트 간 세션 이동을 위해 `work-bridge`는 다음과 같은 로직을 수행해야 합니다.

### 4.1. 타겟 기기 주입 (Injection) 단계
1. **슬러그 생성:** 타겟 기기의 프로젝트 절대 경로를 기반으로 새로운 슬러그를 결정하거나, 기존 `projects.json`에 강제 주입합니다.
2. **소유권 패치 (Critical):** 복사된 `~/.gemini/tmp/<id>/.project_root` 파일의 내용을 타겟 머신의 절대 경로로 즉각 수정해야 합니다.
3. **경로 보정:** `chats/*.json` 파일 내부의 도구 실행 결과나 파일 경로가 포함된 메타데이터를 타겟 기기의 경로에 맞게 문자열 치환(Regex Patching)합니다.

### 4.2. 데이터 포맷 변환 (Normalization)
- **JSONL to JSON:** Claude Code나 Codex CLI의 JSON Lines 형식을 Gemini CLI의 단일 JSON 객체 트리로 변환하거나 그 반대의 과정을 수행해야 합니다.
- **스키마 매핑:**
    - `user_message` (Claude) -> `role: user` (Gemini)
    - `assistant_thinking` (Claude) -> `role: model` 내의 `thoughts` 또는 `parts` (Gemini)
    - `tool_use/result` -> Gemini의 도구 호출 입출력 스키마로 매핑

## 5. 주의사항
- **세션 유지 정책 (TTL):** `settings.json`의 `sessionRetention` 설정에 따라 30일이 지난 세션은 자동으로 삭제될 수 있으므로, 익스포트 시 이를 확인해야 합니다.
- **충돌 방지:** `proper-lockfile`을 사용하여 `projects.json`에 접근하므로, 수동 수정 시 파일 잠금 상태에 주의해야 합니다.
