# BigCat V · Android 端（v0.3 骨架）

Kotlin + Jetpack Compose 客户端，通过 `VpnService` 建立系统 TUN，
并以 gomobile 产物拉起 Go 核心（`bigcatvd`），所有业务走核心的本地
REST + SSE API（默认 `http://127.0.0.1:17890`）。

包名：`com.bigcat.v`；minSdk 26，targetSdk 34。

## 目录结构

```
mobile/android/
├── settings.gradle / build.gradle / gradle.properties
├── README.md                        # 本文件
└── app/
    ├── build.gradle                 # Compose BOM、okhttp、aar 接入点
    ├── libs/                        # bigcatv.aar 放置处（见 libs/README.md）
    └── src/main/
        ├── AndroidManifest.xml      # VpnService 声明、BIND_VPN_SERVICE 等权限
        ├── java/com/bigcat/v/
        │   ├── MainActivity.kt      # 唯一 Activity；负责 VPN 授权流程
        │   ├── BigCatVpnService.kt  # VpnService：建 TUN → 拉核心 → 起引擎
        │   ├── api/
        │   │   ├── ApiClient.kt     # OkHttp 封装（org.json，与服务端字段一一对应）
        │   │   └── Models.kt        # Node / KernelInfo / StatusInfo / Subscription…
        │   ├── core/
        │   │   └── CoreBridge.kt    # gomobile 集成点（反射调用 mobile.Mobile）
        │   └── ui/
        │       ├── BigCatApp.kt     # 导航壳（底部 4 Tab）
        │       ├── VpnViewModel.kt  # 全应用状态：节点/订阅/引擎轮询
        │       ├── NodesScreen.kt   # 节点列表、连接开关、测速、添加/删除
        │       ├── SubscriptionsScreen.kt
        │       ├── LogsScreen.kt    # SSE 实时日志（断线自动重连）
        │       ├── SettingsScreen.kt# API 地址、默认内核、查看内核配置
        │       └── Prefs.kt         # SharedPreferences 设置
        └── res/                     # 字符串、主题、通知图标
```

## 构建步骤

### 1. 安装 gomobile

```bash
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
```

### 2. 生成 bigcatv.aar

```bash
cd ~/workspace/bigcat-v
gomobile bind -target=android \
  -o mobile/android/app/libs/bigcatv.aar \
  ./core/mobile
```

约定：Go 包名为 `mobile`，导出 `Start(listenAddr, workDir string) error`、
`Stop()`、`Version() string`（`core/mobile/` 由另一个 agent 实现）。

### 3. 接入 aar

取消 `app/build.gradle` 中这一行的注释：

```gradle
implementation(name: 'bigcatv', ext: 'aar')
```

`CoreBridge.kt` 用反射调用 `mobile.Mobile`，Kotlin 侧无需改动。

### 4. Android Studio 构建

- 用 Android Studio 打开 `mobile/android/`（Gradle 项目根）。
- 等待 Gradle 同步完成（需下载 AGP 8.3.2、Kotlin 1.9.22、Compose BOM）。
- `Run > Run 'app'` 安装到真机/模拟器。

> 本机无 Android SDK 时无法编译；CI 或开发机需安装
> Android SDK Platform 34 + Build-Tools。

### 5. 真机调试

1. 手机开启「开发者选项 → USB 调试」，连接电脑。
2. 首次点「连接」会弹出系统 VPN 授权框，点确定。
3. 连接成功后状态栏出现钥匙图标，应用内显示「已连接」。
4. 日志页可实时查看核心输出；`adb logcat | grep bigcatv` 看应用日志。

注意：
- 调试阶段核心监听 `127.0.0.1:17890`，Android 9+ 默认禁明文 HTTP，
  但回环地址属于系统豁免，无需额外配置。
- sing-box / xray 二进制需随 aar 或 assets 一并打包（`core/mobile` 侧负责），
  否则 `/api/v1/kernels` 会显示内核不可用。

## 关键设计

- **控制面与数据面分离**：`BigCatVpnService` 只建系统 TUN + 拉起核心；
  引擎启停、节点管理全走本地 HTTP API，与桌面端（Tauri）共用同一契约。
- **tun 模式**：`POST /api/v1/engine/start {tun: true}` 生成的 sing-box
  配置使用 gVisor 用户态栈；Android 侧 TUN fd 交接见
  `BigCatVpnService.kt` 顶部的 TODO（待 `core/mobile` 暴露 `SetTunFd`）。
- **无 aar 也可编译**：`CoreBridge` 反射调用，缺失时抛明确异常，
  保证骨架在任何机器上都能通过编译检查。
