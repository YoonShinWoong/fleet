# keepty desktop

A thin native shell for the keepty web app, built with Tauri 2. The window is a WebView pointed at your keepty daemon; the Rust side adds what a web page cannot:

- a menu bar item showing how many agents are waiting for you, with a menu to open the window, jump to the next waiting agent, change host, or quit
- a Dock badge with the same number
- native notifications while the window is hidden (needs input, turn finished, error)
- `⌥⌘F` to bring the window up from anywhere
- the host and token remembered between launches (`~/Library/Application Support/dev.keepty.desktop/conn.json`)

Closing the window hides it; the app keeps running in the menu bar. Quit from the menu.

## Build

```sh
cd apps/desktop
npm install
npx tauri build        # .app and .dmg under src-tauri/target/release/bundle/
npx tauri dev          # live window during development
```

Needs Rust (stable) and the Xcode command line tools. The icon set is generated from `tools/icon` (a tiny Go program) with `npx tauri icon`.

The app is unsigned. The first launch needs a right-click → Open, or `xattr -d com.apple.quarantine keepty.app`.
