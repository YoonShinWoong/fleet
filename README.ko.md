<p align="center"><a href="README.md">English</a> | 한국어</p>

# keepty

코딩 에이전트를 위한 가벼운 tmux. 늘 켜져 있는 메인 맥북에 데몬 하나를 두고, 그 안에서 이름 붙은 터미널(PTY)들이 클라이언트가 끊겨도 계속 돌아간다. 다른 맥북에서는 CLI로, 폰에서는 브라우저로 같은 터미널에 붙는다. Claude Code와 Codex가 보내는 훅 이벤트로 어느 터미널이 입력을 기다리는지도 한눈에 보인다.

tmux에서 가져온 것은 "세션이 클라이언트보다 오래 산다"와 "여러 클라이언트가 같은 세션을 본다", 둘뿐이다. 창, 패널, 레이아웃, 키 테이블, 설정 언어는 없다. 한 터미널은 한 PTY에서 한 명령을 돌린다. 설계 배경은 [docs/DESIGN.md](docs/DESIGN.md)에 있다.

## 호스트 (메인 맥북)

Go 없이 바이너리만 옮겨도 된다. 이 레포에서 `make dist`로 만든 `dist/keepty-darwin-arm64`를 AirDrop이나 scp로 보내고 `~/.local/bin/keepty`에 둔다. Go가 있으면 `go install ./cmd/keepty`.

```sh
keepty daemon                 # 한 번 실행해서 ~/.keepty/config.json 과 토큰 생성, Ctrl-C
keepty info                   # 주소와 토큰, 다른 기기용 환경변수
```

다른 기기에서 쓰려면 `~/.keepty/config.json`의 `addr`를 `0.0.0.0:7677`(또는 Tailscale 주소)로 바꾸고, `repos`에 자주 쓰는 레포를 적는다. 그 다음 launchd에 올려 로그인 때 자동으로 뜨고 죽어도 되살아나게 한다.

```sh
keepty service install        # ~/Library/LaunchAgents/dev.keepty.daemon.plist
keepty service status
keepty service log
```

훅은 `keepty hooks install claude`와 `keepty hooks install codex`가 기존 훅을 건드리지 않고 각 이벤트에 keepty 항목을 덧붙인다. 백업은 같은 자리에 `.bak-keepty`로 남는다. Codex는 첫 실행 때 새 훅을 신뢰할지 한 번 묻는다. 두 CLI는 이벤트마다 `keepty hook <agent>`를 부르고 keepty은 항상 `{}`를 돌려주므로 에이전트 동작은 바뀌지 않는다. 데몬이 없어도 훅은 1.5초 안에 조용히 포기한다.

맥북이 잠들면 끝이니 전원을 꽂고 시스템 설정의 배터리 옵션에서 "디스플레이가 꺼져 있을 때 자동으로 잠자기 방지"를 켠다. loopback 밖 요청은 토큰이 있어야 통과하지만, 그래도 Tailscale 같은 사설망 안에서만 여는 게 맞다.

## 워크스페이스

워크스페이스는 이름 붙은 작업 폴더다. 레포 이름을 주면 `~/keepty/worktrees/<레포>/<이름>`에 git 워크트리를 만들고, 폴더 경로를 주면 그 폴더를 그대로 쓴다. 터미널은 워크스페이스 안에서 claude, codex, shell 중 하나로 띄우고, 한 워크스페이스에 여러 개를 띄울 수 있다. 터미널 이름은 비우면 `<워크스페이스>-<에이전트>`가 된다.

```sh
keepty ws new fix-login -repo myapp        # 워크트리 워크스페이스
keepty ws new notes -path ~/workspace/notes    # 폴더 워크스페이스
keepty open fix-login -agent codex -a          # 그 안에 codex 터미널, 바로 붙기
keepty open fix-login                          # 같은 곳에 셸 하나 더 (fix-login-shell)
keepty ws ls
keepty ws rm fix-login -delete                 # 잊기. -delete 면 워크트리 디렉토리도 제거
```

웹 앱의 + new 창도 같은 선택지다. 기존 워크스페이스를 고르거나 새로 만들고(레포 또는 폴더), 에이전트를 고르고, 원하면 터미널 이름을 적는다. 목록은 워크스페이스별로 묶이고 머리의 + 로 그 자리에서 claude, codex, shell을 추가한다.

## 쓰기

```sh
keepty sh work                                 # 워크스페이스 없이 셸 하나. 떼기는 Ctrl-\
keepty new myapp fix-login -agent codex -a # 워크스페이스 fix-login + codex, 한 줄로
keepty ls                                      # 터미널과 에이전트 상태
keepty attach fix-login
keepty send fix-login "응, 그대로 진행해"
keepty kill fix-login
```

레포는 `~/.keepty/config.json`의 `repos`에 이름을 등록하거나 경로를 그대로 넘긴다. 에이전트가 끝나면 터미널은 셸로 떨어져서 결과를 볼 수 있다.

## 다른 맥북에서

```sh
export KEEPTY_ADDR=100.x.y.z:7677 KEEPTY_TOKEN=<호스트 config.json의 token>
keepty ls
keepty attach fix-login
```

SSH가 필요 없다. CLI는 WebSocket으로 데몬에 직접 붙는다.

## 모델과 에이전트 바꾸기

모델은 시작할 때 고르거나, 돌고 있는 세션에 바꾸라고 할 수 있다. 시작할 때는 에이전트 설정의 `modelFlag`(claude는 `--model`, codex는 `-m`)를 붙여 실행하고, 도중에는 `modelCommand`(claude는 `/model {model}`, codex는 `/model`로 선택창)를 그 터미널에 타이핑한다.

```sh
keepty open fix-login -agent claude -model opus -a
keepty model fix-login-claude sonnet
```

에이전트를 바꾸는 건 넘겨주기(handoff)다. 같은 폴더에서 다른 에이전트를 새 터미널로 띄우되, 이전 터미널의 마지막 출력 200줄과 git 상태를 `.keepty/handoff-*.md`에 남기고, 새 에이전트에게 그 파일을 읽고 이어가라는 첫 프롬프트를 넣어 시작한다. `.keepty/`은 그 저장소의 `.git/info/exclude`에 자동으로 추가돼 git 상태를 더럽히지 않는다.

```sh
keepty handoff fix-login-claude -to codex -a          # 이전 터미널은 남겨둠
keepty handoff fix-login-claude -to codex -close      # 넘긴 뒤 이전 터미널 종료
```

웹 앱에서는 칸 머리의 model 버튼과 ↪ 버튼이 같은 일을 한다. 첫 프롬프트 문구는 `~/.keepty/config.json`의 `handoffPrompt`에서 바꾼다.

## 웹 앱

브라우저로 데몬 주소를 열면 왼쪽에 터미널 목록, 오른쪽에 터미널 화면이 있는 앱이 뜬다. Chrome의 "앱 설치"로 독립 창으로 띄울 수 있다.

목록은 워크스페이스별로 묶이고, 머리를 누르면 접힌다. 접힌 상태에서도 터미널 수와 입력 대기 수가 보인다. 입력을 기다리는 항목은 노란 점이 깜빡인다. 항목을 누르면 현재 칸에 열리고, 항목의 ⫼ 를 누르면 옆에 새 칸으로 열린다(최대 4칸, 2×2). 칸 머리의 ⤢ 는 그 칸만 크게, × 는 닫기다. 닫아도 세션은 살아 있다. 배치는 브라우저에 기억된다.

에이전트가 입력을 기다리기 시작하거나 턴을 끝내거나 터미널이 끝나면 오른쪽 아래에 토스트가 뜨고, 누르면 그 터미널로 간다. 종 아이콘을 켜면 데스크톱 알림도 온다. 스피커 아이콘은 짧은 소리.

검색은 두 겹이다. 칸 머리의 find 버튼이나 Alt+F로 그 터미널의 출력 안에서 찾고(Enter 다음, Shift+Enter 이전, 일치하는 곳이 칠해진다), 왼쪽 필터에 두 글자 이상 치면 모든 터미널의 스크롤백에서 찾아 아래에 "출력에서 찾음"으로 보여준다. 결과를 누르면 그 터미널이 열리면서 같은 검색어가 바로 걸린다. CLI는 `keepty search <텍스트>`.

키보드는 Alt+1부터 9로 목록 순서대로 전환, Alt+N 으로 다음 입력 대기 터미널, Alt+S 로 빈 칸 추가(분할), Alt+W 로 칸 닫기, Alt+M 으로 칸 최대화다.

## 맥 데스크톱 앱

릴리스의 `keepty-desktop-*.dmg`를 받아 `keepty.app`을 응용 프로그램에 넣고, 처음 열 때 `keepty info`의 주소와 토큰을 넣는다. 메뉴 막대에 입력 대기 수가 뜨고, 창을 닫아도 메뉴 막대에 남아 알림을 보내며, ⌥⌘K로 어디서든 창을 부른다. 아직 공증이 안 돼 첫 실행은 우클릭 열기. 폰은 그대로 웹이다.

## 폰에서

같은 앱이 폰 크기에선 목록 화면과 터미널 화면으로 나뉜다. 처음 한 번 토큰을 물어본다. 터미널 아래 키 바에 esc, tab, ctrl, 화살표, enter, 그리고 "y 엔터"가 있다. 홈 화면에 추가하면 전체 화면 앱처럼 열린다. 긴 대화는 Claude 앱의 Remote Control과 ChatGPT 앱으로 넘기고, 이 화면은 보고 짧게 답하는 용도다.

## 알아둘 것

재접속할 때 스크롤백을 바이트 그대로 다시 틀어준다. 전체 화면 TUI(Claude Code, Codex)는 그걸로 완전히 복원되지 않아서, keepty이 붙은 직후 터미널 폭을 한 칸 줄였다 돌려 TUI가 다시 그리게 만든다. 가상 터미널 파서를 넣어 깨끗한 스냅샷을 주는 건 v0.2 항목이다.

데몬을 재시작하면 터미널이 돌아온다. 에이전트는 훅이 알려준 세션 id로 `claude --resume`, `codex resume`을 다시 실행하고, 셸은 마지막 출력 200줄을 보여주며 새로 뜬다. 훅이 없으면 에이전트는 새로 시작한다.

## 버전과 릴리스

SemVer 태그로 관리한다. 변경은 `CHANGELOG.md`의 Unreleased 절에 적어두고, 릴리스할 때 버전 절로 옮긴 뒤 태그를 푸시한다.

```sh
git tag -a v0.2.0 -m "v0.2.0" && git push origin main v0.2.0
```

태그가 올라가면 GitHub Actions가 테스트를 돌리고 macOS(arm64, amd64)와 Linux(arm64, amd64) 바이너리를 빌드해 릴리스에 붙인다. 호스트에서는 릴리스 자산을 받기만 하면 된다.

```sh
gh release download -R YoonShinWoong/keepty -p keepty-darwin-arm64 -O ~/.local/bin/keepty
chmod +x ~/.local/bin/keepty
```

main으로 가는 푸시와 PR은 `go vet`과 `go test`를 통과해야 한다.

## 라이선스

MIT. [LICENSE](LICENSE) 참고.
