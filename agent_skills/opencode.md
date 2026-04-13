콘텐츠로 이동
OpenCode

검색
⌘
K
소개
Config
공급자
네트워크
엔터프라이즈
문제 해결
Windows
Go
TUI
CLI
Web
IDE
Zen
공유
GitHub
GitLab
도구
규칙
Agents
모델
테마
키바인드
명령어
포매터
권한
LSP 서버
MCP 서버
ACP Support
에이전트 스킬
Custom Tools
목차
개요
파일 위치
검색 이해
Frontmatter 작성
유효한 이름
길이 규칙 준수
사용 예제
도구 설명 인식
권한 구성
에이전트별 재정의
스킬 도구 비활성화
로딩 문제 해결
에이전트 스킬
SKILL.md 정의를 통해 재사용 가능한 동작을 정의합니다.

Agent Skill let opencode discover reusable instruction from your repo 또는 홈 디렉토리. Skills are loaded on-demand via native skill tool-agents see available skills and can loaded full content when needed.

파일 위치
기술 이름 당 하나의 폴더를 만들고 내부 SKILL.md를 넣어. opencode 이 위치를 검색:

프로젝트 구성: .opencode/skills/<name>/SKILL.md
글로벌 구성: ~/.config/opencode/skills/<name>/SKILL.md
프로젝트 클로드 호환 : .claude/skills/<name>/SKILL.md
글로벌 클로드 호환 : ~/.claude/skills/<name>/SKILL.md
프로젝트 에이전트 호환 : .agents/skills/<name>/SKILL.md
글로벌 에이전트 호환 : ~/.agents/skills/<name>/SKILL.md
검색 이해
Project-local paths의 경우, opencode는 git worktree에 도달 할 때까지 현재 작업 디렉토리에서 걷습니다. 그것은 skills/*/SKILL.md에 있는 어떤 어울리는 .opencode/ 및 어떤 어울리는 .claude/skills/*/SKILL.md 또는 .agents/skills/*/SKILL.md를 방법 적재합니다.

세계적인 정의는 또한 ~/.config/opencode/skills/*/SKILL.md, ~/.claude/skills/*/SKILL.md 및 ~/.agents/skills/*/SKILL.md에서 적재됩니다.

Frontmatter 작성
각 SKILL.md는 YAML frontmatter로 시작해야 합니다. 이 필드는 인식됩니다:

name (필수)
description (필수)
(선택) license
(선택) compatibility
metadata (선택 사항, 문자열에 문자열 맵)
알려진 frontmatter 필드는 무시됩니다.

유효한 이름
name는 해야 합니다:

1–64자
단 하나 hyphen 분리기를 가진 더 낮은 케이스 alphanumeric가 있으십시오
-로 시작 또는 끝 아닙니다
연속 -- 포함하지
SKILL.md를 포함하는 디렉토리 이름을 일치
동등한 regex:

^[a-z0-9]+(-[a-z0-9]+)*$

길이 규칙 준수
description는 1-1024 특성이어야 합니다. 제대로 선택하기 위해 에이전트에 대해 충분히 유지하십시오.

사용 예제
이처럼 .opencode/skills/git-release/SKILL.md 만들기:

---
name: git-release
description: Create consistent releases and changelogs
license: MIT
compatibility: opencode
metadata:
  audience: maintainers
  workflow: github
---

## What I do

- Draft release notes from merged PRs
- Propose a version bump
- Provide a copy-pasteable `gh release create` command

## When to use me

Use this when you are preparing a tagged release.
Ask clarifying questions if the target versioning scheme is unclear.

도구 설명 인식
opencode는 skill 도구 설명에서 사용할 수있는 기술을 나열합니다. 각 항목에는 기술 이름 및 설명이 포함됩니다.

<available_skills>
  <skill>
    <name>git-release</name>
    <description>Create consistent releases and changelogs</description>
  </skill>
</available_skills>

에이전트는 도구를 호출하여 기술을로드 :

skill({ name: "git-release" })

권한 구성
기술 에이전트가 opencode.json의 패턴 기반 권한을 사용하여 액세스 할 수있는 제어 :

{
  "permission": {
    "skill": {
      "*": "allow",
      "pr-review": "allow",
      "internal-*": "deny",
      "experimental-*": "ask"
    }
  }
}

권한	동작
allow	기술이 즉시 로드됨
deny	에이전트에서 기술 숨김, 접근 거부
ask	로드 전에 사용자에게 승인 요청
패턴 지원 와일드 카드: internal-* 경기 internal-docs, internal-tools, 등.

에이전트별 재정의
글로벌 디폴트보다 특정 에이전트 다른 권한을 부여합니다.

**사용자 지정 에이전트 ** ( 에이전트 frontmatter):

---
permission:
  skill:
    "documents-*": "allow"
---

** 내장 에이전트 ** (opencode.json에서):

{
  "agent": {
    "plan": {
      "permission": {
        "skill": {
          "internal-*": "allow"
        }
      }
    }
  }
}

스킬 도구 비활성화
그들을 사용하지 않는 에이전트을위한 완전히 비활성화 된 기술 :

사용자 지정 에이전트:

---
tools:
  skill: false
---

** 내장 에이전트 **:

{
  "agent": {
    "plan": {
      "tools": {
        "skill": false
      }
    }
  }
}

비활성화 할 때, <available_skills> 섹션은 완전히 부유합니다.

로딩 문제 해결
기술이 나타나지 않는 경우:

SKILL.md는 모든 모자에서 spelled
name와 description를 포함하는 검사
기술 이름은 모든 위치에서 독특합니다.
deny를 가진 허가를 검사하십시오 에이전트에서 숨겨집니다
페이지 편집
Found a bug? Open an issue
Join our Discord community
언어 선택
한국어
© Anomaly

마지막 업데이트: 2026. 4. 11.