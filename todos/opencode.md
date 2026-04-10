# OpenCode: SQLite 기반 관계형 세션 아키텍처 및 정규화 분석

OpenCode는 고성능 전용 엔진인 Bun SQLite와 Drizzle ORM을 활용하여 세션 데이터를 관리한다. 다른 에이전트들이 파일 시스템의 디렉토리 구조나 개별 파일에 의존하는 것과 달리, OpenCode는 모든 데이터를 하나의 중앙 집중식 관계형 데이터베이스(`opencode.db`)에 구조화하여 저장하는 방식을 취한다.

## 1. 세션 저장소 위치 및 데이터베이스 구조

macOS 환경에서 OpenCode는 XDG Base Directory 규격을 준수하여 사용자의 애플리케이션 지원 폴더에 데이터를 저장한다.

*   **데이터베이스 경로:** `~/Library/Application Support/opencode/opencode.db`
*   **저장 형식:** SQLite 3 (WAL 모드 활성화)

OpenCode의 데이터베이스 스키마는 세 가지 핵심 테이블을 중심으로 정규화되어 있다:

| 테이블명 | 역할 | 주요 필드 |
| :--- | :--- | :--- |
| **`session`** | 세션 메타데이터 관리 | `id` (UUID), `project_id`, `slug`, `directory` (절대 경로), `title` |
| **`message`** | 메시지 헤더 및 순서 | `id`, `session_id`, `data` (JSON 데이터) |
| **`part`** | 메시지 상세 콘텐츠 | `id`, `message_id`, `session_id`, `data` (실제 텍스트 및 도구 결과 JSON) |

## 2. 프로젝트 식별 및 경로 결정론 (Path Determinism)

OpenCode는 프로젝트를 식별하기 위해 단순한 경로 해싱이 아닌, 소스 제어 시스템(VCS)의 불변성을 이용한 **루트 커밋 해싱(Root Commit Hashing)** 전략을 사용한다.

1.  **프로젝트 고유 ID 생성:**
    *   Git 저장소인 경우: `git rev-list --max-parents=0 HEAD` 명령어를 통해 해당 저장소의 첫 번째 커밋 해시를 추출한다.
    *   캐싱: 추출된 ID는 프로젝트 내의 `.git/opencode` 파일에 캐싱되어 이후 조회 성능을 높인다.
    *   비 Git 프로젝트: 고정된 문자열인 `"global"`을 ID로 사용한다.
2.  **절대 경로 의존성:**
    *   `ProjectTable`의 `worktree` 필드와 `SessionTable`의 `directory` 필드에 프로젝트의 로컬 절대 경로가 저장된다. 
    *   이 값은 CLI 실행 시 현재 디렉토리와 대조되어 해당 세션을 불러올지 결정하는 핵심 기준이 된다.

## 3. 세션 상호 운용성 및 마이그레이션 전략 (Import/Export)

OpenCode의 데이터를 다른 기기로 마이그레이션하거나 `work-bridge`를 통해 연동할 때 고려해야 할 핵심 요소는 다음과 같다.

### 3.1. SQLite 레벨의 데이터 주입 (Injection)
단순한 파일 복사가 아닌, `INSERT` 문을 통한 데이터 주입이 필요하다. 타겟 기기의 `opencode.db`에 소스 기기의 `session`, `message`, `part` 레코드를 순차적으로 삽입해야 하며, 외래 키(Foreign Key) 제약 조건을 준수해야 한다.

### 3.2. 절대 경로 보정 (Path Patching)
가장 중요한 단계는 **`directory` 및 `worktree` 필드의 업데이트**이다.
*   **문제:** 소스 기기(`/Users/source/project`)와 타겟 기기(`/Users/target/project`)의 경로가 다르면, 데이터베이스에 세션이 존재하더라도 OpenCode CLI는 현재 디렉토리와 일치하는 세션을 찾지 못한다.
*   **해결:** 마이그레이션 도구(work-bridge)는 타겟 기기의 실제 프로젝트 경로를 탐지하여, 주입되는 모든 `session` 및 `project` 레코드의 경로 관련 필드를 동적으로 치환해야 한다.

### 3.3. 프로젝트 ID 무결성 유지
Git 기반 프로젝트의 경우 루트 커밋 해시가 동일하므로 기기 간 ID 일관성이 유지되지만, 비 Git 프로젝트(`global`)의 경우 여러 프로젝트의 세션이 하나로 뒤섞일 위험이 있으므로 별도의 네임스페이스 관리가 필요할 수 있다.

---

**요약:** OpenCode는 강력한 SQLite 기반 아키텍처를 통해 데이터 무결성을 보장하지만, 기기 간 이동 시에는 **절대 경로의 물리적 매핑**을 데이터베이스 레벨에서 수동으로 보정해주어야만 세션 재개가 가능하다.
