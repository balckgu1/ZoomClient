#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::path::PathBuf;
use std::sync::Mutex;
use tauri::Emitter;
use tauri::Manager;
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

/// Global sidecar child process handle (for writing to stdin).
struct SidecarState {
    child: Mutex<Option<CommandChild>>,
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .manage(SidecarState {
            child: Mutex::new(None),
        })
        .setup(|app| {
            let app_handle = app.handle().clone();

            // 移除 Windows DWM 窗口阴影，避免透明宠物窗口周围出现边框
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.set_shadow(false);
            }

            // Spawn sidecar: zoomcli --mode api --config-dir <project_root>/config
            // In dev mode, the binary runs from src-tauri/, so we go up 2 levels to reach project root.
            // In production, the sidecar binary sits next to the app, also with config alongside.
            let src_tauri_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")); // points to src-tauri/
            let project_root = src_tauri_dir
                .parent()  // desktop/
                .and_then(|p| p.parent())  // cc-learn/
                .map(|p| p.to_path_buf())
                .unwrap_or_else(|| std::env::current_dir().unwrap());
            let config_dir = project_root.join("config");

            eprintln!("sidecar project_root: {:?}", project_root);
            eprintln!("sidecar config_dir: {:?}", config_dir);

            let sidecar_cmd = app
                .shell()
                .sidecar("zoomcli")
                .expect("sidecar not configured")
                .current_dir(project_root.clone());
            let (mut rx, child) = sidecar_cmd
                .args(["--mode", "api", "--config-dir", &config_dir.to_string_lossy()])
                .spawn()
                .expect("failed to spawn sidecar");

            // Store child process handle for stdin writes
            let state = app_handle.state::<SidecarState>();
            *state.child.lock().unwrap() = Some(child);

            // Read sidecar events and emit stdout lines to frontend
            tauri::async_runtime::spawn(async move {
                while let Some(event) = rx.recv().await {
                    match event {
                        CommandEvent::Stdout(line_bytes) => {
                            let text = String::from_utf8_lossy(&line_bytes).to_string();
                            for line in text.lines() {
                                if !line.is_empty() {
                                    app_handle
                                        .emit("sidecar-event", line)
                                        .expect("failed to emit");
                                }
                            }
                        }
                        CommandEvent::Stderr(line_bytes) => {
                            let text = String::from_utf8_lossy(&line_bytes);
                            eprintln!("sidecar stderr: {text}");
                        }
                        CommandEvent::Terminated(payload) => {
                            eprintln!("sidecar terminated: {:?}", payload);
                            break;
                        }
                        _ => {}
                    }
                }
                eprintln!("sidecar event loop ended");
            });

            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::Destroyed = event {
                let state = window.state::<SidecarState>();
                let maybe_child = {
                    let mut guard = state.child.lock().unwrap();
                    guard.take()
                };
                if let Some(child) = maybe_child {
                    let _ = child.kill();
                }
            }
        })
        .invoke_handler(tauri::generate_handler![send_to_sidecar])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

/// Tauri command: write a JSON line to the sidecar's stdin.
#[tauri::command]
fn send_to_sidecar(message: String, state: tauri::State<SidecarState>) -> Result<(), String> {
    let mut guard = state.child.lock().unwrap();
    if let Some(ref mut child) = *guard {
        child
            .write(format!("{}\n", message).as_bytes())
            .map_err(|e| format!("failed to write to sidecar: {e}"))
    } else {
        Err("sidecar not running".into())
    }
}
