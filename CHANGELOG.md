# Changelog

[Keep a Changelog](https://keepachangelog.com/) 형식, [SemVer](https://semver.org/) 버전. 1.0 전까지는 minor가 기능 추가, patch가 수정이다. `vX.Y.Z` 태그를 푸시하면 GitHub Actions가 바이너리를 빌드해 이 파일의 해당 절을 릴리스 노트로 올린다.

## Unreleased

## v0.1.0 (2026-11-08)

### Added
- 이름 붙은 PTY 터미널. 클라이언트가 끊겨도 살고, 스크롤백을 다시 틀어주며, 여러 클라이언트가 같은 터미널을 본다 (가벼운 tmux 코어).
- WebSocket attach 프로토콜과 CLI attach (Ctrl-\ 로 떼기). loopback 밖 요청은 토큰 인증.
- 워크스페이스 (git 워크트리 또는 아무 폴더), 그 안의 에이전트 또는 셸 터미널, 시작 시 모델 선택과 세션 중 모델 변경.
- Claude Code와 Codex 훅 수집. 작업 중, 입력 대기, 대기, 종료 상태.
- 에이전트 간 넘겨주기. 이전 터미널의 출력과 git 상태를 `.fleet/handoff-*.md`에 남기고 새 에이전트를 시작.
- 웹 앱. 워크스페이스별 접히는 목록, 최대 4칸 분할, 상태 전이 알림, 단축키, 터미널 내 검색과 전체 출력 검색, PWA 설치.
- `fleet service` (launchd 상시 실행), `fleet info`, `fleet hooks install`, `fleet search`.
