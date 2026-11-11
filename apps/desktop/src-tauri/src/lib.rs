//! keepty desktop: a thin native shell around the keepty web app.
//!
//! The window is a WebView pointed at your keepty daemon. The Rust side only
//! does what a web page cannot: a menu bar item with the number of agents
//! waiting for you, a Dock badge, native notifications while the window is
//! hidden, a global shortcut, and remembering the host and token.

use std::collections::HashMap;
use std::sync::Mutex;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tauri::menu::{Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Manager, WindowEvent};
use tauri_plugin_global_shortcut::ShortcutState;
use tauri_plugin_notification::NotificationExt;
use tauri_plugin_store::StoreExt;

#[derive(Clone, Serialize, Deserialize, Default, Debug)]
struct Conn {
    host: String,
    token: String,
}

#[derive(Default)]
struct State {
    conn: Mutex<Option<Conn>>,
    prev: Mutex<HashMap<String, String>>, // terminal name -> last status we showed
    polled_once: Mutex<bool>,
}

#[derive(Deserialize)]
struct Session {
    #[serde(default)]
    term: String,
    #[serde(default)]
    agent: String,
    #[serde(default)]
    status: String,
    #[serde(rename = "lastMessage", default)]
    last_message: String,
    #[serde(rename = "updatedAt", default)]
    updated_at: String,
}

#[derive(Deserialize)]
struct TermInfo {
    name: String,
    #[serde(default)]
    alive: bool,
}

fn normalize_host(h: &str) -> String {
    let h = h.trim().trim_end_matches('/');
    if h.starts_with("http://") || h.starts_with("https://") {
        h.to_string()
    } else {
        format!("http://{h}")
    }
}

fn load_conn(app: &AppHandle) -> Option<Conn> {
    let store = app.store("conn.json").ok()?;
    let host = store.get("host")?.as_str()?.to_string();
    let token = store.get("token").and_then(|v| v.as_str().map(String::from)).unwrap_or_default();
    if host.is_empty() {
        return None;
    }
    Some(Conn { host, token })
}

fn percent(s: &str) -> String {
    let mut out = String::new();
    for b in s.bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => out.push(b as char),
            _ => out.push_str(&format!("%{:02X}", b)),
        }
    }
    out
}

fn navigate(app: &AppHandle, conn: &Conn, hash: &str) {
    if let Some(w) = app.get_webview_window("main") {
        let url = format!("{}/?token={}{}", conn.host, percent(&conn.token), hash);
        if let Ok(u) = url.parse::<url::Url>() {
            let _ = w.navigate(u);
        }
        let _ = w.show();
        let _ = w.set_focus();
    }
}

fn show(app: &AppHandle) {
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.show();
        let _ = w.unminimize();
        let _ = w.set_focus();
    }
}

fn go_waiting(app: &AppHandle) {
    let conn = app.state::<State>().conn.lock().unwrap().clone();
    match conn {
        Some(c) => navigate(app, &c, "#waiting"),
        None => show(app),
    }
}

fn change_host(app: &AppHandle) {
    if let Some(w) = app.get_webview_window("main") {
        if let Ok(u) = "tauri://localhost/index.html".parse::<url::Url>() {
            let _ = w.navigate(u);
        }
        let _ = w.show();
        let _ = w.set_focus();
    }
}

#[tauri::command]
fn saved(app: AppHandle) -> Option<Conn> {
    load_conn(&app)
}

#[tauri::command]
fn connect(app: AppHandle, host: String, token: String) -> Result<(), String> {
    let conn = Conn { host: normalize_host(&host), token: token.trim().to_string() };
    let store = app.store("conn.json").map_err(|e| e.to_string())?;
    store.set("host", serde_json::json!(conn.host));
    store.set("token", serde_json::json!(conn.token));
    store.save().map_err(|e| e.to_string())?;
    *app.state::<State>().conn.lock().unwrap() = Some(conn.clone());
    app.state::<State>().prev.lock().unwrap().clear();
    *app.state::<State>().polled_once.lock().unwrap() = false;
    navigate(&app, &conn, "");
    Ok(())
}

async fn fetch<T: for<'de> Deserialize<'de>>(client: &reqwest::Client, conn: &Conn, path: &str) -> Result<T, String> {
    let mut req = client.get(format!("{}{}", conn.host, path));
    if !conn.token.is_empty() {
        req = req.bearer_auth(&conn.token);
    }
    let resp = req.send().await.map_err(|e| e.to_string())?;
    if !resp.status().is_success() {
        return Err(format!("{} {}", resp.status(), path));
    }
    resp.json::<T>().await.map_err(|e| e.to_string())
}

/// One poll: per-terminal status, tray title, dock badge, notifications on transitions.
async fn poll(app: &AppHandle, client: &reqwest::Client, conn: &Conn) -> Result<(), String> {
    let terms: Vec<TermInfo> = fetch(client, conn, "/v1/terms").await?;
    let sessions: Vec<Session> = fetch(client, conn, "/v1/sessions").await?;
    let mut latest: HashMap<String, &Session> = HashMap::new();
    for s in &sessions {
        if s.term.is_empty() {
            continue;
        }
        match latest.get(&s.term) {
            Some(cur) if cur.updated_at >= s.updated_at => {}
            _ => {
                latest.insert(s.term.clone(), s);
            }
        }
    }
    let mut now: HashMap<String, (String, String, String)> = HashMap::new();
    for t in terms.iter().filter(|t| t.alive) {
        if let Some(s) = latest.get(&t.name) {
            now.insert(t.name.clone(), (s.status.clone(), s.agent.clone(), s.last_message.clone()));
        }
    }
    let waiting = now.values().filter(|v| v.0 == "needs_input").count();

    if let Some(tray) = app.tray_by_id("main") {
        let _ = tray.set_title(Some(if waiting == 0 { String::new() } else { format!(" {waiting}") }));
        let _ = tray.set_tooltip(Some(if waiting == 0 { "keepty: nothing waiting".to_string() } else { format!("keepty: {waiting} waiting for you") }));
    }
    if let Some(w) = app.get_webview_window("main") {
        let _ = w.set_badge_count(if waiting == 0 { None } else { Some(waiting as i64) });
    }

    let focused = app.get_webview_window("main").and_then(|w| w.is_focused().ok()).unwrap_or(false);
    let state = app.state::<State>();
    let first = {
        let mut p = state.polled_once.lock().unwrap();
        let f = !*p;
        *p = true;
        f
    };
    let mut prev = state.prev.lock().unwrap();
    if !first && !focused {
        for (term, (status, agent, msg)) in &now {
            let was = prev.get(term).cloned().unwrap_or_default();
            if &was == status {
                continue;
            }
            let title = match status.as_str() {
                "needs_input" => format!("{term} needs your input"),
                "idle" if was == "working" => format!("{term} finished a turn"),
                "error" => format!("{term} hit an error"),
                _ => continue,
            };
            let body = if msg.is_empty() { agent.clone() } else { msg.clone() };
            let _ = app.notification().builder().title(title).body(body).show();
        }
    }
    *prev = now.into_iter().map(|(k, v)| (k, v.0)).collect();
    Ok(())
}

fn start_polling(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        let client = reqwest::Client::builder().timeout(Duration::from_secs(5)).build().unwrap();
        loop {
            let conn = app.state::<State>().conn.lock().unwrap().clone();
            if let Some(c) = conn {
                if let Err(e) = poll(&app, &client, &c).await {
                    if let Some(tray) = app.tray_by_id("main") {
                        let _ = tray.set_title(Some(" !".to_string()));
                        let _ = tray.set_tooltip(Some(format!("keepty: cannot reach {} ({e})", c.host)));
                    }
                }
            }
            tokio::time::sleep(Duration::from_secs(4)).await;
        }
    });
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_store::Builder::new().build())
        .plugin(tauri_plugin_notification::init())
        .plugin(
            tauri_plugin_global_shortcut::Builder::new()
                .with_shortcuts(["alt+super+f"])
                .expect("shortcut")
                .with_handler(|app, _shortcut, event| {
                    if event.state() == ShortcutState::Pressed {
                        show(app);
                    }
                })
                .build(),
        )
        .manage(State::default())
        .invoke_handler(tauri::generate_handler![saved, connect])
        .setup(|app| {
            let handle = app.handle().clone();
            let open = MenuItem::with_id(app, "open", "Open keepty", true, Some("Alt+Super+F"))?;
            let next = MenuItem::with_id(app, "next", "Next agent waiting", true, None::<&str>)?;
            let host = MenuItem::with_id(app, "host", "Change host…", true, None::<&str>)?;
            let quit = MenuItem::with_id(app, "quit", "Quit keepty", true, None::<&str>)?;
            let menu = Menu::with_items(app, &[&open, &next, &PredefinedMenuItem::separator(app)?, &host, &quit])?;
            let tray_icon = tauri::image::Image::from_bytes(include_bytes!("../icons/tray.png"))?;
            TrayIconBuilder::with_id("main")
                .icon(tray_icon)
                .icon_as_template(true)
                .menu(&menu)
                .show_menu_on_left_click(true)
                .tooltip("keepty")
                .on_menu_event(|app, e| match e.id.as_ref() {
                    "open" => show(app),
                    "next" => go_waiting(app),
                    "host" => change_host(app),
                    "quit" => app.exit(0),
                    _ => {}
                })
                .build(app)?;
            if let Some(conn) = load_conn(&handle) {
                *app.state::<State>().conn.lock().unwrap() = Some(conn.clone());
                navigate(&handle, &conn, "");
            }
            start_polling(handle);
            Ok(())
        })
        .on_window_event(|window, event| {
            // closing the window keeps keepty in the menu bar; Quit is in the tray menu
            if let WindowEvent::CloseRequested { api, .. } = event {
                let _ = window.hide();
                api.prevent_close();
            }
        })
        .run(tauri::generate_context!())
        .expect("error while running keepty desktop");
}
