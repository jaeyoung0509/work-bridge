좋다. 이걸로는 이제 방향이 꽤 선명해졌어.

내가 다시 정리한 최종 제품 방향은 이거야:

CLI 기반의 “agent session portability” 도구
Claude Code, Gemini CLI, Codex CLI의 로컬에 저장된 세션/설정/지침 파일을 읽어서, 내부 정규화 포맷으로 변환하고, 다시 다른 툴이 먹기 좋은 형태로 export / rehydrate 해주는 제품. 세 툴 모두 프로젝트별 지침 파일, 설정 계층, 세션 재개 같은 공통 구조를 갖고 있어서 이 문제는 실제로 존재하고, 특히 Gemini와 Codex는 세션/로컬 저장 관련 문서가 비교적 명확하다. Claude도 CLAUDE.md, auto memory, settings, session resume를 공식 문서로 제공한다.  ￼

⸻

PRD v1

Product name

sessionport

Tagline

Normalize, import, and rehydrate coding-agent sessions across Claude Code, Gemini CLI, and Codex CLI.  ￼

One-line summary

로컬에 저장된 Claude Code, Gemini CLI, Codex CLI의 세션/설정/지침 자산을 읽어 내부 공통 포맷으로 정규화하고, 다른 툴로 옮길 수 있는 portable session bundle로 export하는 CLI. Gemini는 세션 저장과 resume를 공식적으로 문서화하고 있고, Codex는 로컬 세션 재개와 설정 파일, AGENTS.md를 지원한다. Claude는 CLAUDE.md와 auto memory를 통해 세션 간 지식을 유지한다.  ￼

Problem

지금 coding agent 사용자는 툴을 바꾸는 순간 매번 같은 프로젝트 규칙, 작업 배경, 최근 시도, 실패한 접근, 중요 결정사항을 다시 설명해야 한다. 각 툴은 자체적으로 세션 재개와 기억 방식을 갖고 있지만, 그것들이 서로 호환되지 않는다. Gemini CLI는 세션/히스토리 관리 기능을 제공하고, Codex CLI는 resume과 로컬 세션 지속성을 제공하며, Claude Code는 fresh context로 시작하되 CLAUDE.md와 auto memory로 지식을 이어간다. 즉 문제는 “기억이 없다”가 아니라 기억이 툴마다 갇혀 있다는 점이다.  ￼

Product thesis

우리가 만드는 것은 범용 RAG 플랫폼이 아니고, 또 단순한 설정 변환기만도 아니다.
핵심은:
	•	각 툴의 로컬 상태를 안전하게 읽고
	•	공통 primitive로 정규화하고
	•	다른 툴이 시작할 때 필요한 최소 작업 상태로 재수화하는 것

즉, “transcript copier”가 아니라 working-state portability layer다. Codex는 AGENTS.md를 읽고, Gemini는 설정 파일과 GEMINI.md를 계층적으로 사용하며, Claude는 CLAUDE.md와 auto memory를 사용하므로, 완전한 1:1 복제보다 “다음 세션을 잘 시작하게 하는 bundle”이 더 현실적이다.  ￼

Target users

주 사용자는 다음 세 부류다.
	1.	Claude Code, Gemini CLI, Codex CLI를 번갈아 쓰는 개인 개발자
	2.	한 repo에서 긴 작업을 여러 세션에 나눠 진행하는 개발자
	3.	프로젝트 규칙과 세션 히스토리를 잃지 않고 다른 agent로 넘기고 싶은 power user

이 포지션은 세 툴이 모두 터미널 기반 coding agent이고, 로컬 설정/지침/세션 재개 기능을 공식 제공한다는 점과 맞는다.  ￼

Core user job

“툴을 바꾸거나 세션을 다시 시작해도, 지금까지의 작업 맥락을 다시 설명하지 않고 이어서 일하고 싶다.”
이건 Codex의 resume, Gemini의 session/history 관리, Claude의 memory 모델이 각각 해결하려는 문제와 직접 겹친다.  ￼

⸻

Scope

In scope for v1

v1에서 다룰 자산은 아래로 제한한다.
	•	session metadata
	•	user/assistant turns의 요약 가능한 부분
	•	tool call / command / output summary
	•	task title / first prompt / current goal
	•	touched files
	•	중요한 decisions / failures
	•	persistent instruction files: CLAUDE.md, GEMINI.md, AGENTS.md
	•	일부 settings
	•	resume metadata

이 범위가 현실적인 이유는 세 툴 모두 지침 파일과 설정 계층은 공식화되어 있고, Gemini와 Codex는 세션 재개/관리도 비교적 명확히 문서화돼 있기 때문이다.  ￼

Out of scope for v1

v1에서는 아래는 의도적으로 제외한다.
	•	hidden internal model state의 완전 복원
	•	비공개 reasoning의 추출/이식
	•	secrets, API keys, OAuth tokens의 자동 export
	•	UI preference 완전 복원
	•	툴별 플러그인/훅의 완전 호환
	•	transcript 전체를 무손실로 다른 툴에 이식

이건 각 툴의 내부 포맷과 보안 모델이 다르고, 공식 문서가 보장하는 호환 범위를 넘어가기 때문이다. Codex와 Gemini는 설정/세션 인터페이스가 공개돼 있지만, 내부 상태 전체를 상호 호환하는 표준은 없다.  ￼

⸻

Product goals

Goal 1: Local-first

이 도구는 반드시 로컬 우선이어야 한다. 세 툴 모두 로컬 설정 파일과 로컬 워크스페이스를 중심으로 동작하므로, sessionport도 기본 동작은 로컬 파일 시스템을 source of truth로 삼는다. 클라우드 백업은 가능하되, 핵심 경험은 로컬 탐색, 로컬 추출, 로컬 export다.  ￼

Goal 2: Normalize first, export second

핵심 차별점은 exporter 개수가 아니라 중간 canonical schema의 품질이다. 각 툴의 포맷이 달라도, 사용자 입장에서는 “현재 작업 상태”라는 공통 개념이 있기 때문에, 먼저 정규화 계층을 만든 뒤 target-specific exporter를 붙여야 한다. Gemini의 settings/session 계층, Codex의 config/AGENTS.md, Claude의 settings/memory 구조가 이런 설계를 뒷받침한다.  ￼

Goal 3: Rehydrate, not clone

성공 기준은 “A툴 세션을 B툴에서 byte-for-byte 복원”이 아니라, B툴에서 다음 작업을 잘 시작할 수 있게 만드는 것이다. 따라서 산출물은 raw transcript dump가 아니라, condensed session bundle과 target-native starter artifacts여야 한다. Codex의 AGENTS.md, Claude의 CLAUDE.md, Gemini의 GEMINI.md가 모두 이 재수화 지점 역할을 할 수 있다.  ￼

⸻

Functional requirements

FR1. Detect local installations and project artifacts

CLI는 사용자의 머신과 현재 repo에서 아래를 자동 탐지해야 한다.
	•	Claude Code 관련 설정/지침 파일
	•	Gemini CLI 관련 설정/세션 파일
	•	Codex CLI 관련 설정/세션 파일
	•	현재 프로젝트 루트와 instruction files

Codex는 ~/.codex/config.toml을 기본 설정 파일로 사용하고, AGENTS.md를 프로젝트 지침으로 사용한다. Gemini는 settings 파일과 GEMINI.md를 문서화한다. Claude는 settings와 CLAUDE.md를 문서화한다.  ￼

FR2. Import sessions and related artifacts

CLI는 각 툴에서 가능한 범위의 세션 관련 데이터를 읽어와야 한다.
	•	session ids
	•	timestamps
	•	cwd / project root
	•	current task title
	•	prompt/response excerpts or summaries
	•	tool events
	•	file references
	•	token usage if available
	•	memory/instruction artifacts

Gemini는 세션 및 히스토리 관리를 별도 문서로 안내하고, Codex는 resume과 세션 재개 기능을 제공한다. Claude는 프로젝트 기억과 auto memory를 공식 문서로 설명한다.  ￼

FR3. Normalize into a canonical session bundle

모든 imported data는 내부 표준 포맷으로 변환되어야 한다. 예를 들면:
	•	source_tool
	•	session_id
	•	project_root
	•	task_title
	•	summary
	•	decisions
	•	failures
	•	tool_events
	•	touched_files
	•	instruction_files
	•	token_stats
	•	settings_snapshot
	•	resume_hints

이 canonical bundle은 제품의 핵심 자산이고, exporter는 이 포맷만 바라봐야 한다. 세 툴의 표면적 차이를 줄이는 유일한 방법이기 때문이다.  ￼

FR4. Export target-native starter artifacts

CLI는 canonical bundle을 타겟 툴이 시작할 때 활용 가능한 형태로 내보내야 한다.
	•	Claude target: CLAUDE.md supplement, starter prompt, memory note
	•	Gemini target: GEMINI.md supplement, settings patch, session bootstrap note
	•	Codex target: AGENTS.md supplement, starter prompt, config hints

즉 export의 본질은 “세션 복원”이 아니라 “타겟 툴이 잘 시작하도록 landing zone 생성”이다.  ￼

FR5. Doctor and compatibility reporting

CLI는 어떤 데이터가 성공적으로 이식되는지, 어떤 데이터가 누락되거나 축약되는지 리포트해야 한다.

예:
	•	raw transcript: partial
	•	tool outputs: summarized only
	•	secrets: excluded
	•	vendor-specific options: unsupported
	•	auto memory: converted to plain note only

이건 사용자 신뢰를 위해 필수다. Codex와 Claude 모두 다양한 설정/기능 계층을 갖고 있고, Gemini도 광범위한 설정 옵션을 제공하므로, “완전 자동 변환”보다 “명시적 진단”이 더 중요하다.  ￼

FR6. Optional cloud backup, but local source of truth

canonical bundles는 기본적으로 로컬에 저장되어야 하고, 옵션으로 클라우드 백업을 지원할 수 있다. 다만 v1에서 클라우드는 부가 기능이다. 문제의 본질은 vendor portability이지 hosted memory sync가 아니기 때문이다. 세 툴 모두 기본 사용 경험은 로컬 CLI와 로컬 워크스페이스 중심이다.  ￼

⸻

Non-functional requirements

Reliability

파일 포맷이 바뀌어도 import 단계가 깨지지 않도록 best-effort parsing과 graceful degradation이 필요하다. 특히 Claude는 공식 문서상 memory/settings는 잘 정리돼 있지만, 모든 로컬 세션 포맷이 완전히 공개된 것은 아니므로 importer는 방어적으로 설계해야 한다.  ￼

Security

기본 정책은 no secret export다. 환경변수명과 reference만 다루고 실제 값은 저장하지 않는다. Claude는 settings의 env 지원을 문서화하고, Gemini는 MCP 서버 설정에서 환경변수 expansion을 문서화하므로, secret ref 모델이 자연스럽다.  ￼

Explainability

사용자가 언제든 “왜 이 bundle이 이렇게 만들어졌는지” 볼 수 있어야 한다. 요약된 턴 수, 제외된 필드, 손실된 정보, 생성된 target files를 모두 보고해야 한다. 이건 portability tool의 신뢰성 핵심이다.  ￼

⸻

Canonical model

Core entities

v1 canonical schema는 아래 엔티티를 가진다.
	•	SessionBundle
	•	InstructionArtifact
	•	SettingsSnapshot
	•	ToolEvent
	•	Decision
	•	Failure
	•	ExportPlan
	•	CompatibilityReport

이 구조는 transcript DB가 아니라 “작업 상태 이동 포맷”에 가깝다. Claude의 memory, Gemini의 session management, Codex의 resume/AGENTS.md를 공통 primitive로 환원하기 위해서다.  ￼

⸻

CLI surface

Primary commands

v1 CLI는 아래 명령에 집중한다.

sessionport detect
sessionport inspect claude
sessionport inspect gemini
sessionport inspect codex

sessionport import --from gemini --session latest
sessionport import --from codex --session abc123
sessionport import --from claude --session latest

sessionport doctor --from codex --session abc123 --target claude
sessionport export --bundle bundle.json --target gemini --out ./rehydrated
sessionport pack --from codex --session abc123 --out task.spkg
sessionport unpack --file task.spkg --target claude

이 명령 구조는 import → normalize → doctor → export 흐름을 강제해서, 사용자가 변환 결과를 먼저 검토할 수 있게 한다.  ￼

⸻

MVP definition

v1 MVP

v1에서는 다음만 반드시 지원한다.
	•	Gemini importer
	•	Codex importer
	•	Claude partial importer
	•	canonical session bundle 생성
	•	doctor 리포트
	•	Claude/Gemini/Codex용 starter artifact export
	•	portable bundle pack/unpack

Gemini와 Codex를 우선하는 이유는 공식 문서상 세션/설정/재개 관련 surface가 더 선명하기 때문이다. Claude는 memory/settings/instruction import는 강하게, raw session portability는 보수적으로 들어가는 게 안전하다.  ￼

Explicitly not in MVP
	•	live sync daemon
	•	remote shared workspace SaaS
	•	plugin/hook full fidelity migration
	•	hidden memory extraction
	•	enterprise policy manager
	•	GUI

v1은 어디까지나 CLI portability engine이어야 한다.  ￼

⸻

Architecture direction

Recommended implementation language

이 프로젝트의 v1은 golang으로 

Internal layers

구현 계층은 이렇게 잡는 게 좋다.
	•	detectors/ : 로컬 설치와 파일 위치 탐지
	•	importers/ : 툴별 파서
	•	normalize/ : canonical model 변환
	•	validate/ : 손실/비호환 분석
	•	exporters/ : 타겟별 artifact 생성
	•	bundle/ : pack/unpack
	•	cli/ : user interface

이 구조는 세 툴별 변화와 공통 정책을 분리하기 좋다.  ￼

⸻

Success metrics

Product success

초기 성공 기준은 아래가 적절하다.
	•	import 성공률
	•	doctor 리포트 정확도
	•	export 후 사용자가 수동 수정 없이 바로 쓸 수 있는 비율
	•	같은 작업을 새 툴에서 시작할 때 필요한 재설명 감소
	•	GitHub stars / installs / repeat usage

이 제품의 가치는 “환상적인 정확도”보다 “툴 교체 비용 절감”에 있다.  ￼

⸻

Positioning

Final positioning statement

sessionport is a local-first CLI that imports coding-agent sessions, normalizes them into a portable working-state bundle, and rehydrates them for Claude Code, Gemini CLI, and Codex CLI.  ￼

Why this is sharp enough

이 포지션은 “universal memory platform”보다 훨씬 뾰족하다.
왜냐면 사용자 문제를 아주 직접적으로 겨냥하기 때문이다:
	•	agent 바꾸면 기억이 끊김
	•	같은 repo 맥락을 다시 설명해야 함
	•	긴 세션이 휘발됨
	•	프로젝트 규칙이 툴마다 중복됨

반면 sessionport는 이걸 import / normalize / doctor / rehydrate라는 아주 구체적인 작업 흐름으로 풀어낸다. 세 툴 모두 local CLI 기반, settings 계층, persistent instructions, resume/memory primitives를 갖고 있어서 wedge도 충분히 현실적이다.  ￼

원하면 다음 단계로 바로 schema v0 JSON 설계랑 CLI 명령 스펙까지 이어서 구체화하겠다.