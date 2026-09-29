# BigCat V 移动端（v0.3）

与桌面端共用同一 Go 核心，移动端以 gomobile 方式把核心编译进 App 进程。

## 结构

```
mobile/
  ARCHITECTURE.md   # 整体架构：gomobile 绑定、进程内引擎、TUN 双路径、拉起流程
  VERSION           # 0.3.0
  android/          # Android 应用骨架（Kotlin + Compose + VpnService）
  ios/              # iOS 应用骨架（Swift + SwiftUI + PacketTunnelProvider）
```

## 关键设计

- **进程内引擎**：移动端（尤其 iOS）不能 spawn 子进程。`core/internal/engine/` 在
  `mobile` 构建标签下提供 sing-box / xray 的 Go 库直调实现（`inproc_singbox.go` /
  `inproc_xray.go`），实现同一 `engine.Core` 接口；桌面端仍用二进制进程方式。
- **gomobile 绑定**：`core/mobile/mobile.go` 导出 `Start/Stop/Running/Version`
  （仅 string/bool 签名，gomobile 兼容）。
  - Android：`gomobile bind -target=android -o bigcatv.aar`
  - iOS：`gomobile bind -target=ios -o Bigcatv.xcframework`
- **TUN**：v0.3 走 sing-box gVisor 用户态 TUN（无需向 Go 层传 fd）；
  系统 TUN fd 注入（路径 B）为后续工作，见 ARCHITECTURE.md。
- **控制面统一**：Android / iOS 的 UI 与业务逻辑全部走本地 REST API
 （`http://127.0.0.1:17890`），与桌面端同一契约，见 `docs/API.md`。

## 状态

骨架与核心绑定已完成并通过编译/单测（含 `-tags mobile` 与 Android 交叉编译）。
真机 `.aar`/`.xcframework` 打包与流量测试需 NDK/Xcode，按各目录 README 在开发机上进行。
