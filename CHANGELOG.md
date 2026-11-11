# Changelog

[Keep a Changelog](https://keepachangelog.com/) 형식, [SemVer](https://semver.org/) 버전. 1.0 전까지는 minor가 기능 추가, patch가 수정이다. `vX.Y.Z` 태그를 푸시하면 GitHub Actions가 바이너리를 빌드해 이 파일의 해당 절을 릴리스 노트로 올린다.

## Unreleased

### Changed
- 이름을 fleet에서 keepty로 바꿈. 레포와 Go 모듈 `github.com/YoonShinWoong/keepty`, 명령 `keepty`, 환경변수 `KEEPTY_*`, 설정 폴더 `~/.keepty`, 레포별 `.keepty/`, launchd `dev.keepty.daemon`, 데스크톱 앱 `dev.keepty.desktop`. 예전 이름은 읽지 않는다.
- 기존 설치 옮기기: `fleet service uninstall`, `mv ~/.fleet ~/.keepty`, 새 바이너리를 `~/.local/bin/keepty`에 두고 `keepty service install`, `keepty hooks install claude`(codex도), 에이전트 훅 설정에 남은 `fleet hook` 항목 삭제, 다른 기기의 `FLEET_*` 환경변수를 `KEEPTY_*`로. 웹 앱과 데스크톱 앱은 저장 키가 바뀌어 토큰을 한 번 다시 넣는다.

## v0.3.0 (2026-11-08)

### Added
- macOS 데스크톱 앱 `apps/desktop` (Tauri 2, 6MB). 웹 앱을 네이티브 창에 담고, 메뉴 막대에 입력 대기 수, Dock 배지, 창이 숨겨져 있을 때의 네이티브 알림, 전역 단축키 ⌥⌘F, 호스트와 토큰 기억, 창을 닫아도 메뉴 막대에 상주. 릴리스 워크플로가 태그마다 `.dmg`를 함께 올린다.
- 웹 앱이 `?token=`으로 넘겨받은 토큰을 저장하고 주소에서 지운다. `#waiting`으로 열면 입력 대기 중인 터미널로 바로 간다.

### Changed
- 로드맵을 버전 번호 대신 Next, Then, Later로 적는다.

## v0.2.0 (2026-11-08)

### Added
- 데몬 재시작 뒤 터미널 복구. `~/.fleet/terminals.json`에 30초마다, 변경 때마다, 종료 때 저장하고, 시작할 때 다시 만든다. 에이전트는 훅이 보고한 세션 id로 `resumeCommand`(`claude --remote-control --resume {session}`, `codex resume {session}`)를 실행하고, 셸은 마지막 출력 200줄을 스크롤백에 깔아 둔다. SIGTERM을 받으면 저장 후 자식에 SIGHUP.
- OSC 7501 상태 보고. 어떤 프로그램이든 `\e]7501;state=blocked:app=my-agent\e\\`를 출력하면 훅과 같은 상태, 배지, 알림을 받는다. lmux가 정의한 규약과 호환.
- 목록에 "복구됨" 표시.
- README에 유휴 측정치. 터미널 20개, 데몬 RSS 12MB, 1분간 측정 가능한 CPU 사용 없음.

### Changed
- 로드맵 번호를 한 칸씩 미룸. MCP 레지스트리와 메모리 번들은 v0.3.

## v0.1.2 (2026-11-08)

### Added
- `fleet info`가 같은 와이파이용 `.local` 주소, Tailscale IP와 MagicDNS 이름, `tailscale serve` 안내를 보여준다.
- README에 Tailscale 연결 절.

### Fixed
- `tailscale serve` 같은 로컬 리버스 프록시 뒤에서 온 요청(X-Forwarded-For)은 loopback으로 보지 않고 토큰을 요구한다.

## v0.1.1 (2026-11-08)

### Added
- 웹 앱 UI가 영어 기본, 브라우저 언어가 한국어면 한국어. `localStorage.fleet.lang`으로 고정 가능.
- 영어 README와 스크린샷, 구조 다이어그램. 한국어 README는 `README.ko.md`.

### Fixed
- 워크스페이스 머리의 긴 이름과 경로가 여러 줄로 꺾이던 문제, 접힌 머리가 스크롤 시 겹치던 문제.
- 칸 머리의 긴 터미널 이름 줄바꿈, 셸 터미널 목록에 전체 경로가 보이던 문제, 폰 화면에서 행 버튼이 내용을 밀어내던 문제.

## v0.1.0 (2026-11-08)

### Added
- 이름 붙은 PTY 터미널. 클라이언트가 끊겨도 살고, 스크롤백을 다시 틀어주며, 여러 클라이언트가 같은 터미널을 본다 (가벼운 tmux 코어).
- WebSocket attach 프로토콜과 CLI attach (Ctrl-\ 로 떼기). loopback 밖 요청은 토큰 인증.
- 워크스페이스 (git 워크트리 또는 아무 폴더), 그 안의 에이전트 또는 셸 터미널, 시작 시 모델 선택과 세션 중 모델 변경.
- Claude Code와 Codex 훅 수집. 작업 중, 입력 대기, 대기, 종료 상태.
- 에이전트 간 넘겨주기. 이전 터미널의 출력과 git 상태를 `.fleet/handoff-*.md`에 남기고 새 에이전트를 시작.
- 웹 앱. 워크스페이스별 접히는 목록, 최대 4칸 분할, 상태 전이 알림, 단축키, 터미널 내 검색과 전체 출력 검색, PWA 설치.
- `fleet service` (launchd 상시 실행), `fleet info`, `fleet hooks install`, `fleet search`.
