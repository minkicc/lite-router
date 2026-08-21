use std::sync::Mutex;

use tauri::{AppHandle, Manager};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

const LEGACY_APP_IDENTIFIER: &str = "com.literouter.desktop";
const DATA_FILES: [&str; 2] = ["config.json", "usage.json"];

struct RouterState {
    child: Mutex<Option<CommandChild>>,
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
            return Ok(RouterStatus {
                running: true,
                pid: Some(child.pid()),
            });
        }
    }

    let child = spawn_router(&app)?;
    let pid = child.pid();
    *state.child.lock().unwrap() = Some(child);

    Ok(RouterStatus {
        running: true,
        pid: Some(pid),
    })
}

#[tauri::command]
async fn stop_router(state: tauri::State<'_, RouterState>) -> Result<RouterStatus, String> {
    let child = state.child.lock().unwrap().take();
    if let Some(child) = child {
        child.kill().map_err(|e| e.to_string())?;
    }
    Ok(RouterStatus {
        running: false,
        pid: None,
    })
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
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            start_router,
            stop_router,
            router_status,
            get_router_base_url
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
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
