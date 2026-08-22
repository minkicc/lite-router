use std::sync::Mutex;

use tauri::menu::{CheckMenuItem, Menu, MenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Manager};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

const LEGACY_APP_IDENTIFIER: &str = "com.literouter.desktop";
const DATA_FILES: [&str; 2] = ["config.json", "usage.json"];

struct RouterState {
    child: Mutex<Option<CommandChild>>,
    tray_toggle: Mutex<Option<CheckMenuItem<tauri::Wry>>>,
    tray_quit: Mutex<Option<MenuItem<tauri::Wry>>>,
    locale: Mutex<String>,
}

#[derive(serde::Serialize)]
struct RouterStatus {
    running: bool,
    pid: Option<u32>,
}

fn router_config_path(app: &AppHandle) -> Result<std::path::PathBuf, String> {
    let dir = app.path().app_config_dir().map_err(|e| e.to_string())?;
    migrate_legacy_data(&dir)?;
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    Ok(dir.join("config.json"))
}

fn migrate_legacy_data(new_dir: &std::path::Path) -> Result<(), String> {
    let Some(parent) = new_dir.parent() else {
        return Ok(());
    };
    let old_dir = parent.join(LEGACY_APP_IDENTIFIER);
    if !old_dir.is_dir() {
        return Ok(());
    }

    std::fs::create_dir_all(new_dir).map_err(|e| e.to_string())?;
    for name in DATA_FILES {
        let source = old_dir.join(name);
        let destination = new_dir.join(name);
        if !source.is_file() || destination.exists() {
            continue;
        }
        if std::fs::rename(&source, &destination).is_err() {
            std::fs::copy(&source, &destination).map_err(|e| e.to_string())?;
            std::fs::remove_file(&source).map_err(|e| e.to_string())?;
        }
    }

    if std::fs::read_dir(&old_dir)
        .map_err(|e| e.to_string())?
        .next()
        .is_none()
    {
        std::fs::remove_dir(old_dir).map_err(|e| e.to_string())?;
    }
    Ok(())
}

fn spawn_router(app: &AppHandle) -> Result<CommandChild, String> {
    let config_path = router_config_path(app)?;
    let config_arg = config_path.to_string_lossy().to_string();

    let backend_command = app
        .shell()
        .sidecar("lite-router")
        .map_err(|e| e.to_string())?;

    let (mut rx, child) = backend_command
        .args([
            "--config".to_string(),
            config_arg,
            "--no-browser".to_string(),
        ])
        .spawn()
        .map_err(|e| e.to_string())?;

    tauri::async_runtime::spawn(async move {
        while let Some(event) = rx.recv().await {
            if let CommandEvent::Terminated(_) = event {
                break;
            }
        }
    });

    Ok(child)
}

#[tauri::command]
async fn start_router(
    app: AppHandle,
    state: tauri::State<'_, RouterState>,
) -> Result<RouterStatus, String> {
    {
        let guard = state.child.lock().unwrap();
        if let Some(child) = guard.as_ref() {
            sync_tray_status(&app);
            return Ok(RouterStatus {
                running: true,
                pid: Some(child.pid()),
            });
        }
    }

    let child = spawn_router(&app)?;
    let pid = child.pid();
    *state.child.lock().unwrap() = Some(child);
    sync_tray_status(&app);

    Ok(RouterStatus {
        running: true,
        pid: Some(pid),
    })
}

#[tauri::command]
async fn stop_router(
    app: AppHandle,
    state: tauri::State<'_, RouterState>,
) -> Result<RouterStatus, String> {
    stop_router_child(&state)?;
    sync_tray_status(&app);
    Ok(RouterStatus {
        running: false,
        pid: None,
    })
}

fn stop_router_child(state: &RouterState) -> Result<(), String> {
    let child = state.child.lock().unwrap().take();
    if let Some(child) = child {
        child.kill().map_err(|e| e.to_string())?;
    }
    Ok(())
}

fn tray_status_text(locale: &str, running: bool, pid: Option<u32>) -> String {
    if locale == "en-US" {
        if running {
            format!("Running (PID {})", pid.unwrap_or_default())
        } else {
            "Start".to_string()
        }
    } else if running {
        format!("运行中 (PID {})", pid.unwrap_or_default())
    } else {
        "启动".to_string()
    }
}

fn tray_quit_text(locale: &str) -> &'static str {
    if locale == "en-US" {
        "Quit"
    } else {
        "退出"
    }
}

fn sync_tray_status(app: &AppHandle) {
    let state = app.state::<RouterState>();
    let (running, pid) = {
        let child = state.child.lock().unwrap();
        (child.is_some(), child.as_ref().map(|child| child.pid()))
    };
    let locale = state.locale.lock().unwrap().clone();
    let item = state.tray_toggle.lock().unwrap().clone();
    let quit_item = state.tray_quit.lock().unwrap().clone();
    if let Some(item) = item {
        let _ = item.set_checked(running);
        let _ = item.set_text(tray_status_text(&locale, running, pid));
    }
    if let Some(item) = quit_item {
        let _ = item.set_text(tray_quit_text(&locale));
    }
}

#[tauri::command]
fn set_locale(app: AppHandle, locale: String) -> Result<(), String> {
    let normalized = if locale.to_lowercase().starts_with("zh") {
        "zh-CN"
    } else {
        "en-US"
    };
    *app.state::<RouterState>().locale.lock().unwrap() = normalized.to_string();
    sync_tray_status(&app);
    Ok(())
}

fn toggle_router(app: &AppHandle) {
    let running = app
        .state::<RouterState>()
        .child
        .lock()
        .unwrap()
        .is_some();
    if running {
        let state = app.state::<RouterState>();
        let _ = stop_router_child(&state);
        sync_tray_status(app);
        return;
    }

    match spawn_router(app) {
        Ok(child) => {
            *app.state::<RouterState>().child.lock().unwrap() = Some(child);
            sync_tray_status(app);
        }
        Err(_) => sync_tray_status(app),
    }
}

fn show_main_window(app: &AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

fn setup_tray(app: &AppHandle) -> tauri::Result<()> {
    let state = app.state::<RouterState>();
    let child = state.child.lock().unwrap();
    let running = child.is_some();
    let pid = child.as_ref().map(|child| child.pid());
    drop(child);
    let locale = state.locale.lock().unwrap().clone();
    let toggle_item = CheckMenuItem::with_id(
        app,
        "toggle-router",
        tray_status_text(&locale, running, pid),
        true,
        running,
        None::<&str>,
    )?;
    let quit_item = MenuItem::with_id(
        app,
        "quit",
        tray_quit_text(&locale),
        true,
        None::<&str>,
    )?;
    let menu = Menu::with_items(app, &[&toggle_item, &quit_item])?;
    app.state::<RouterState>()
        .tray_toggle
        .lock()
        .unwrap()
        .replace(toggle_item.clone());
    app.state::<RouterState>()
        .tray_quit
        .lock()
        .unwrap()
        .replace(quit_item.clone());
    let handle = app.clone();
    let mut tray = TrayIconBuilder::new()
        .tooltip("Lite Router")
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(|app, event| match event.id().as_ref() {
            "toggle-router" => toggle_router(app),
            "quit" => app.exit(0),
            _ => {}
        })
        .on_tray_icon_event(move |_tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                show_main_window(&handle);
            }
        });
    if let Some(icon) = app.default_window_icon().cloned() {
        tray = tray.icon(icon);
    }
    tray.build(app)?;

    Ok(())
}

#[tauri::command]
async fn router_status(state: tauri::State<'_, RouterState>) -> Result<RouterStatus, String> {
    let guard = state.child.lock().unwrap();
    Ok(RouterStatus {
        running: guard.is_some(),
        pid: guard.as_ref().map(|c| c.pid()),
    })
}

#[tauri::command]
fn get_router_base_url(app: AppHandle) -> Result<String, String> {
    let path = router_config_path(&app)?;
    let listen = std::fs::read_to_string(&path)
        .ok()
        .and_then(|raw| {
            serde_json::from_str::<serde_json::Value>(&raw)
                .ok()
                .and_then(|v| {
                    v.get("listen_addr")
                        .and_then(|x| x.as_str())
                        .map(String::from)
                })
        })
        .unwrap_or_else(|| "127.0.0.1:8787".to_string());

    Ok(base_url_from_listen(&listen))
}

fn base_url_from_listen(listen: &str) -> String {
    let host_port = listen.trim();
    let host_port = if let Some(rest) = host_port.strip_prefix("0.0.0.0") {
        format!("127.0.0.1{rest}")
    } else if let Some(rest) = host_port.strip_prefix(':') {
        format!("127.0.0.1:{rest}")
    } else {
        host_port.to_string()
    };
    format!("http://{host_port}")
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .manage(RouterState {
            child: Mutex::new(None),
            tray_toggle: Mutex::new(None),
            tray_quit: Mutex::new(None),
            locale: Mutex::new("zh-CN".to_string()),
        })
        .setup(|app| {
            let handle = app.handle().clone();
            match spawn_router(&handle) {
                Ok(child) => {
                    *handle
                        .state::<RouterState>()
                        .child
                        .lock()
                        .unwrap() = Some(child);
                }
                Err(_) => {
                    // 前端轮询状态时会显示未运行，并允许用户手动启动。
                }
            }
            setup_tray(&handle)?;
            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                api.prevent_close();
                let _ = window.hide();
            }
        })
        .invoke_handler(tauri::generate_handler![
            start_router,
            stop_router,
            router_status,
            get_router_base_url,
            set_locale
        ])
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app, event| match event {
            tauri::RunEvent::ExitRequested { .. } | tauri::RunEvent::Exit => {
                let state = app.state::<RouterState>();
                let _ = stop_router_child(&state);
            }
            _ => {}
        });
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_config_root() -> std::path::PathBuf {
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        std::env::temp_dir().join(format!("lite-router-migration-{}-{nonce}", std::process::id()))
    }

    #[test]
    fn migrates_legacy_data_without_overwriting_new_files() {
        let root = temp_config_root();
        let old_dir = root.join(LEGACY_APP_IDENTIFIER);
        let new_dir = root.join("cc.minki.literouter");
        std::fs::create_dir_all(&old_dir).unwrap();
        std::fs::create_dir_all(&new_dir).unwrap();
        std::fs::write(old_dir.join("config.json"), "old config").unwrap();
        std::fs::write(old_dir.join("usage.json"), "old usage").unwrap();
        std::fs::write(new_dir.join("config.json"), "new config").unwrap();

        migrate_legacy_data(&new_dir).unwrap();

        assert_eq!(
            std::fs::read_to_string(new_dir.join("config.json")).unwrap(),
            "new config"
        );
        assert_eq!(
            std::fs::read_to_string(new_dir.join("usage.json")).unwrap(),
            "old usage"
        );
        assert!(old_dir.join("config.json").exists());
        assert!(!old_dir.join("usage.json").exists());

        std::fs::remove_dir_all(root).unwrap();
    }
}
