# BigCat V 桌面端（v0.2）

Tauri 外壳 + 零依赖 Web 前端，对标 v2rayN / Clash Verge 的桌面形态。

## 结构

```
desktop/
  src/                  # 前端（index.html / styles.css / app.js，零依赖）
  src-tauri/
    src/main.rs         # Tauri 入口：拉起 bigcatvd sidecar，退出时回收
    tauri.conf.json
    capabilities/
    binaries/           # 打包时放入各平台的 bigcatvd（见下）
  VERSION               # 0.2.0
```

## 前端直连核心调试（无需 Tauri）

前端是纯静态页面，可直接连 `bigcatvd` 调试：

```bash
# 终端 1：启动核心
cd ../core && go run ./cmd/bigcatvd

# 终端 2：静态服务
cd src && python3 -m http.server 1420
# 浏览器打开 http://127.0.0.1:1420
```

## 打包桌面应用

1. 安装 Rust（https://rustup.rs）与 Tauri 系统依赖
   （Linux 需 `libwebkit2gtk-4.1-dev build-essential libssl-dev …`，
   详见 https://v2.tauri.app/start/prerequisites/）。
2. 生成图标：`npx @tauri-apps/cli icon desktop/src/icon.png`
   （或任意 1024px PNG）。
3. 把各平台 `bigcatvd` 二进制改名放入 `src-tauri/binaries/`，
   命名规则 `<name>-<target-triple>[.exe]`，例如：
   `binaries/bigcatvd-x86_64-unknown-linux-gnu`、
   `binaries/bigcatvd-x86_64-pc-windows-msvc.exe`、
   `binaries/bigcatvd-aarch64-apple-darwin`。
4. `npx @tauri-apps/cli build`

## 设计约定

- Tauri 层不碰代理协议：只负责拉起/回收 `bigcatvd` sidecar；
- 所有业务（节点、订阅、启停、日志）走 `http://127.0.0.1:17890` 的 REST + SSE；
- CSP 已关闭（`security.csp: null`），因前端需直连本地 HTTP 与 EventSource。
