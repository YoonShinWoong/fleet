# keepty 설계

작성 2026-11-07. Orca 위에서 Claude Code와 Codex를 쓰다가 생긴 세 가지 불편에서 출발했다. 메모리와 MCP를 두 벌 관리하는 일, 무겁고 느린 세션, 맥북 두 대와 폰 사이의 싱크.

## 결론부터

가벼운 tmux를 만든다. 늘 켜져 있는 메인 맥북 한 대를 호스트로 두고, 그 위에 데몬 하나가 이름 붙은 PTY 터미널들을 소유한다. 터미널은 클라이언트가 끊겨도 산다. 다른 맥북의 CLI와 폰의 브라우저는 같은 WebSocket 프로토콜로 그 터미널에 붙는다. 모든 기기가 데몬 하나만 보기 때문에 싱크는 해결 대상이 아니라 설계상 존재하지 않는 문제가 된다.

## tmux에서 가져오는 것과 버리는 것

| 가져온다 | 버린다 |
|---|---|
| 세션이 클라이언트보다 오래 산다 | 창, 패널, 레이아웃 |
| 여러 클라이언트가 한 세션을 같이 본다 (크기는 가장 작은 쪽) | 키 테이블, prefix 키, 설정 언어 |
| 떼고 다시 붙기, 스크롤백 | 플러그인, 상태줄 스크립팅 |
| | 터미널 안의 터미널 중첩 |

한 터미널은 한 PTY에서 한 명령을 돌린다. 여러 작업은 여러 이름의 터미널이다. 이 단순함이 Orca와 tmux가 느려지는 층을 통째로 피하게 해준다.

## 역할 분담

keepty이 PTY, 스크롤백, 원격 접속, 상태, 설정, 메모리를 맡는다. 두 CLI가 모델 실행과 워크트리 격리와 폰에서의 풀 채팅(Claude Remote Control, Codex remote-control)을 맡는다. 폰의 keepty 화면은 보고 짧게 답하는 용도고 긴 대화는 두 앱으로 넘긴다.

## 비목표

새 터미널 에뮬레이터(렌더링은 xterm.js와 사용자의 터미널 앱이 한다), 모델 호출, 폰에서의 풀 채팅 UI, 두 CLI의 내부 트랜스크립트 포맷 해석.

## 구성

```
keepty daemon  (메인 맥북, 127.0.0.1:7677 또는 Tailscale 주소)
  ├─ terminal manager   이름 붙은 PTY, 1MiB 원시 스크롤백, 클라이언트 집합, 최소 크기 적용
  ├─ attach protocol    GET /v1/terms/{name}/ws   바이너리 프레임 = PTY 바이트, 텍스트 프레임 = JSON 제어
  ├─ hooks ingest       POST /v1/events/{agent}   Claude, Codex 훅이 보낸 JSON, KEEPTY_TERM으로 터미널과 연결
  ├─ state store        ~/.keepty/state.json       에이전트 세션 상태
  ├─ launcher           git worktree add + 터미널 생성 + 에이전트 실행
  ├─ live               GET /v1/stream (SSE)      터미널과 세션 스냅샷
  └─ web client         /  목록, /term.html  xterm.js

keepty CLI  (어느 기기든, KEEPTY_ADDR와 KEEPTY_TOKEN)
  sh, new, attach, ls, send, kill, hook
```

### 워크스페이스

워크스페이스는 이름 붙은 폴더다. 종류는 둘, 레포에서 만든 git 워크트리와 임의의 폴더. `~/.keepty/workspaces.json`에 남아 데몬을 재시작해도 유지된다. 터미널은 워크스페이스 안에서 에이전트(설정의 agents) 또는 셸로 열리고, PTY 환경에 KEEPTY_WORKSPACE, KEEPTY_TASK, KEEPTY_REPO, KEEPTY_AGENT가 심긴다. 훅은 이 값으로 세션과 터미널을 연결한다. Orca의 워크스페이스에서 가져온 건 "작업 하나에 폴더 하나, 그 안에 에이전트 여럿"이라는 모양뿐이다.

### attach 프로토콜

클라이언트가 붙으면 첫 바이너리 프레임으로 스크롤백을 받고, 그 뒤로 PTY 출력이 흐른다. 클라이언트가 보내는 바이너리는 PTY 입력이고, 텍스트 프레임 `{"t":"resize","cols":..,"rows":..}`는 크기 변경이다. 서버는 프로세스가 끝나면 `{"t":"exit","code":..}`를 보낸다. CLI와 브라우저가 같은 프로토콜을 쓴다. 토큰은 Authorization 헤더나 `?token=`으로 전달하고, loopback 요청은 토큰을 묻지 않는다.

### 스크롤백 재생의 한계

원시 바이트를 다시 틀어주는 방식은 전체 화면 TUI를 완전히 복원하지 못한다. 그래서 새 클라이언트가 붙으면 PTY 폭을 한 칸 줄였다 돌려 SIGWINCH로 TUI가 다시 그리게 한다. 가상 터미널 파서로 화면 모델을 유지해 깨끗한 스냅샷을 주는 것이 v0.3 항목이다.

### 웹 앱

한 페이지 앱이다. 왼쪽 목록은 SSE 스냅샷으로 갱신되고, 오른쪽은 1개에서 4개까지의 칸(pane)이며 칸마다 xterm.js 하나와 WebSocket 하나를 가진다. 칸을 닫으면 WebSocket만 끊기고 서버의 터미널은 그대로다. 열려 있지 않은 터미널은 연결이 없으므로 비용이 없다.

알림은 서버가 아니라 클라이언트가 스냅샷의 상태 전이를 보고 만든다. needs_input 진입, working에서 idle로의 전이(턴 종료), 터미널 종료, error가 대상이다. 서버에 알림 채널을 두지 않은 이유는 두 가지다. 클라이언트가 이미 스냅샷을 받고 있고, 어떤 전이를 알릴지는 보는 사람의 취향이기 때문이다. 폰 푸시(ntfy)는 서버 쪽 일이고 v0.3에 둔다.

### 훅

`keepty hook <agent>`는 stdin JSON에 호스트명, KEEPTY_TERM(keepty이 PTY 환경에 심어둔 터미널 이름), KEEPTY_TASK, KEEPTY_REPO를 덧붙여 데몬에 보내고 항상 `{}`를 출력한 뒤 0으로 끝난다. 데몬이 없어도 1.5초 안에 포기한다. 제어판 장애가 에이전트를 막는 일은 없어야 한다.

## 상태 모델

| 이벤트 | 상태 |
|---|---|
| SessionStart | idle |
| UserPromptSubmit, PreToolUse, PostToolUse | working |
| PermissionRequest, Elicitation, Notification(permission_prompt, idle_prompt, elicitation_dialog) | needs_input |
| Stop, Interrupt | idle |
| StopFailure | error |
| SessionEnd | ended |

SubagentStart와 SubagentStop은 카운터만 움직인다. 모르는 이벤트와 모르는 필드는 버리지 않고 무시한다. 두 CLI의 훅 스키마가 바뀌어도 수집이 깨지지 않게 하려는 선택이다.

Codex는 Notification 훅이 부분 지원이라 입력 대기 판정이 늦을 수 있다. v0.2에서 PTY 출력이 멎은 시간을 보조 신호로 쓴다.

## 단계

v0.1 가벼운 tmux. PTY 매니저, WebSocket attach, CLI(sh, new, attach, ls, send, kill), 양쪽 훅 수집, xterm.js 웹 클라이언트, 토큰 인증. 이것만으로 Orca와 tmux 없이 메인에 붙어 쓸 수 있다.

v0.2 설정 한 벌. `registry/mcp.yaml`을 Claude user scope, Codex `config.toml`, 레포 `.mcp.json`으로 렌더링한다. 서버마다 public, intranet, local 태그를 달고, doctor가 이 호스트에서 안 닿는 intranet 서버를 끈다. 메모리 디렉토리를 하나로 모아 Claude는 `autoMemoryDirectory`로, Codex는 글로벌 AGENTS.md로 가리킨다. 지금 네 곳에 흩어진 메모리 36개를 병합한다.

v0.3 폰. PWA 매니페스트와 설치, ntfy 푸시, Remote Control과 ChatGPT 딥링크, 가상 터미널 파서로 깨끗한 재접속 화면.

v0.4 재미. 머신당 MCP 서버를 한 번만 띄우는 프록시, 밤마다 트랜스크립트에서 OKF 메모리 후보를 뽑아 PR로 올리는 증류기, 트랜스크립트 청소기. 이 세 개는 다른 어떤 도구도 두 에이전트에 걸쳐 해주지 않는다.

## 결정 기록

2026-11-07. 스택은 Go. 단일 바이너리, 웹 자산 임베드가 쉽다. 클라이언트는 웹과 터미널 둘 다 만든다. 데몬 API가 먼저이고 둘은 그 위의 뷰다. 호스트는 메인 맥북이며 늘 켜둔다.

2026-11-07 (2). 처음엔 PTY를 tmux에 맡기고 그 위에 제어판만 두려 했다. 원하는 것이 "가벼운 tmux 같은 것을 만들어 다른 맥북과 폰에서 쉽게 쓰는 것"으로 정리되면서, PTY 층을 keepty이 직접 소유하기로 바꿨다. tmux 의존이 사라지고 CLI는 SSH 없이 WebSocket으로 붙는다.

## 열린 질문

관리형 노트북이라면 Tailscale 설치 가능 여부 확인. Codex 관리형 워크트리(0.162, 기능 플래그)를 쓸지 keepty이 만든 워크트리에 그냥 들어갈지. 데몬 재시작은 v0.2에서 "프로세스를 살리는" 대신 "세션을 되살리는" 쪽으로 풀었다. 에이전트는 훅이 준 세션 id로 resume, 셸은 마지막 출력 재생. 프로세스 자체를 살리는 shpool 식 구조는 필요가 생기면 그때.
