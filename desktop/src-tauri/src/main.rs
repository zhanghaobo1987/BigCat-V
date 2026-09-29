//! BigCat V 桌面外壳（Tauri）。
//!
//! 只做两件事：
//! 1. 启动时以 sidecar 方式拉起 `bigcatvd`（Go 核心），退出时一并结束；
//! 2. 承载 `../src` 的前端页面。所有业务逻辑走 bigcatvd 的本地 HTTP API，
//!    Tauri 层不碰任何代理协议。

#![cfg_attr(
    all(not(debug_assertions), target_os = "windows"),
    windows_subsystem = "windows"
)]

use std::sync::Mutex;
use tauri::{Manager, State};
use tauri_plugin_shell::process::CommandChild;
use tauri_plugin_shell::ShellExt;

struct SidecarState(Mutex<Option<CommandChild>>);

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .manage(SidecarState(Mutex::new(None)))
        .setup(|app| {
            // bigcatvd 已在运行时（用户手动启动），sidecar 启动会因端口占用失败，
            // 属正常情况：直接复用已有的核心，前端照常连接 127.0.0.1:17890。
            match app.shell().sidecar("bigcatvd") {
                Ok(cmd) => match cmd.args(["--listen", "127.0.0.1:17890"]).spawn() {
                    Ok((_rx, child)) => {
                        *app.state::<SidecarState>().0.lock().unwrap() = Some(child);
                        println!("[bigcatv] bigcatvd sidecar 已启动");
                    }
                    Err(e) => eprintln!("[bigcatv] sidecar 启动失败（可能核心已在运行）: {e}"),
                },
                Err(e) => eprintln!("[bigcatv] 未找到 bigcatvd sidecar: {e}"),
            }
            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { .. } = event {
                let state: State<SidecarState> = window.state();
                if let Some(child) = state.0.lock().unwrap().take() {
                    let _ = child.kill();
                    println!("[bigcatv] bigcatvd sidecar 已结束");
                }
            }
        })
        .run(tauri::generate_context!())
        .expect("BigCat V 运行失败");
}
