use std::sync::Mutex;

use tauri::{AppHandle, Manager};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

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
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    Ok(dir.join("config.json"))
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
