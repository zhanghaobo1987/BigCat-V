# BigCat V 移动端架构（v0.3 第一部分）

目标：把 Go 核心编译进 Android / iOS App，UI 层（Kotlin/Swift）与桌面端一样，
只调同一套本地 REST + SSE 接口，零重复业务逻辑。

## 总览

```
┌──────────────────────────── App 进程 ────────────────────────────┐
│  Android (Kotlin) / iOS (Swift)                                 │
│    │ ① gomobile 绑定调用                                        │
│    ▼                                                            │
│  bigcatv/mobile（Go）                                           │
│    Start(listenAddr, workDir) → bootstrap.StartMobile()         │
│    │                                                             │
│    ├─► 控制 API：127.0.0.1:17890（与桌面端同一套 api.Server）    │
│    │     App 内 WebView / HTTP 客户端直接调 REST + SSE           │
│    └─► 数据面：进程内引擎（无 fork/exec）                       │
│          ├─► InProcessSingBox：sagernet/sing-box 库              │
│          └─► InProcessXray：xtls/xray-core 库                    │
└─────────────────────────────────────────────────────────────────┘
```

## 关键设计

### 1. gomobile 绑定（`core/mobile/mobile.go`）

- 只导出 gomobile 兼容签名：`string/int/bool`。当前 API：
  - `Start(listenAddr, workDir string) string` —— 成功返回 `""`，失败返回错误文本
  - `Stop() string` —— 停引擎 + 停 API，幂等
  - `Running() bool`、`Version() string`（编译时嵌入 `core/VERSION`）
  - `SetLogLevel(level string) string` / `LogLevel() string`
    （框架阶段：级别仅存储，待日志系统接入后消费）
- 构建：
  ```bash
  go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
  cd core
  gomobile bind -o bigcatv.aar -target android ./mobile        # Android
  gomobile bind -o Bigcatv.xcframework -target ios ./mobile    # iOS
  ```

### 2. 引擎进程内化（`core/internal/engine/inproc_*.go`）

移动端（尤其 iOS）禁止 `fork/exec`，因此 `mobile || android || ios`
构建标签下提供两套 `engine.Core` 的进程内实现，与进程版接口完全一致，
API 层无需改动：

| 内核 | 库 | 解析/校验/启动路径（与官方 CLI 同源） |
|---|---|---|
| sing-box | `github.com/sagernet/sing-box v1.12.16` | `include.Context` 注入注册表 → `UnmarshalExtendedContext[option.Options]` → `box.New(box.Options{...})` → `Start()` / `Close()`；Check = New+Close（等价 `sing-box check`） |
| xray | `github.com/xtls/xray-core v1.250725.0` | `core.LoadConfig("json", reader)`（需 blank import `main/json` 注册 JSON loader、`main/distro/all` 注册全部组件）→ `core.New` → `Start()` / `Close()` |

差异（相对进程版，如实记录）：
- `Binary()` 返回 `"in-process"`；`/api/v1/kernels` 照常标记可用。
- 无崩溃退避重启：移动端生命周期由 App 管理，异常只记状态。
- sing-box 日志走库默认输出（stderr），暂未接入 SSE `logCh`
  （仅收发启停事件行）；后续可通过 `box.Options.PlatformLogWriter` 定制。

### 3. 内核选择（`core/internal/bootstrap`）

- `bootstrap.Start`：桌面/daemon 路径，探测外部二进制（原 main.go 逻辑，原样迁移）。
- `bootstrap.StartMobile`：`mobile/android/ios` 标签下优先进程内引擎，
  失败回退进程引擎；其他标签下等价于 `Start`。
- `cmd/bigcatvd/main.go` 瘦身为参数解析 + 信号等待；
  退出时（SIGINT/SIGTERM）优雅关闭：先停引擎再停 HTTP（此前为直接退出、子进程成孤儿）。

### 4. TUN 方案（v0.3 推荐）

两种路径，v0.3 默认走 **A**：

- **A. gVisor 用户态 TUN（推荐）**：sing-box 的 `tun` inbound 设
  `"stack": "gvisor"`，纯用户态网络栈，**无需从系统拿 TUN fd**，
  Android / iOS 通用，权限模型最简单。代价：相对系统 TUN 有少量性能损耗。
- **B. 系统 TUN fd 注入**：Android `VpnService.Builder.establish()` /
  iOS `NEPacketTunnelFlow` 拿到的 fd 传给 sing-box 的 tun inbound
  （`fd` / 系统栈）。需要 App 侧平台代码配合 + `box.Options` 扩展，
  列为 v0.3 后续优化项。

无论哪种，IP 包转发都在 Go 侧完成，App 侧只负责把 fd 交出来（路径 B）
或什么都不做（路径 A）。

### 5. App 侧拉起流程

**Android**
1. `VpnService` 子类 `onCreate` 调 `Mobile.start("127.0.0.1:17890", filesDir+"/bigcatv")`；
2. （路径 A）无需 `establish()` 即可工作；如需全局代理再建 VPN 通道把流量导给
   `tun` inbound 的 listen 地址；
3. `onDestroy` 调 `Mobile.stop()`。

**iOS**
1. `NEPacketTunnelProvider.startTunnel` 中调 `Mobile.start(...)`；
   注意 Network Extension 内存上限（约 50MB），xray/sing-box 库体积需做
   `-ldflags -s -w` 与裁剪评估；
2. （路径 A）`NEPacketTunnelFlow` 仅用于读写 IP 包时才需要；
   用户态 TUN 下可直接用 socket 方案，简化实现；
3. `stopTunnel` 调 `Mobile.stop()`。

### 6. 与桌面端的差异

| | 桌面（Tauri） | 移动端 |
|---|---|---|
| 核心形态 | `bigcatvd` 独立进程（sidecar） | 编译进 App 进程（gomobile） |
| 数据面 | 外部二进制子进程 | 进程内 Go 库 |
| TUN | 系统 TUN（root/管理员） | gVisor 用户态 TUN（推荐） |
| 控制面 | 完全相同：`127.0.0.1:17890` REST + SSE | 完全相同 |
| 日志 | 子进程 stdout/stderr → SSE | 启停事件行 → SSE（库内部日志待接入） |

## 当前状态与后续工作（诚实记录）

- [x] 进程内双引擎编译通过、Linux 下真实跑通（socks 入站冒烟）。
- [x] Android 交叉编译验证（`GOOS=android go build`，见下）。
- [ ] 真机验证：Android `.aar` / iOS `.xcframework` 的实际 bind 与流量测试
      （需 NDK / Xcode，本机未做）。
- [ ] sing-box 日志接入 SSE（`PlatformLogWriter`）。
- [ ] 路径 B（系统 TUN fd 注入）的 App 侧样例代码。
- [ ] 包体积优化（UPX / `-s -w` / 功能裁剪）与 NE 内存评估。

## 验证命令

```bash
export GOROOT=~/go PATH=~/go/bin:$PATH GOPATH=~/gopath
cd core
go build ./... && go vet ./... && go test ./...          # 默认（桌面）构建
go build -tags mobile ./...                              # mobile 标签构建
GOOS=android go build ./...                              # Android 交叉编译
```
