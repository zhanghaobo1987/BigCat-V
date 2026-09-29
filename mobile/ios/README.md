# BigCat V iOS 端（v0.3 骨架）

Swift + SwiftUI，iOS 16+。主 App 只做控制面（调本地 REST API），
数据面跑在 NetworkExtension（PacketTunnel）扩展进程内。

## 目录结构

```
mobile/ios/
├── README.md                 # 本文件
├── BigCatV/                  # 主 App target
│   ├── BigCatVApp.swift       # @main 入口（在 Views/ContentView.swift 内）
│   ├── Models.swift           # API 数据模型（Codable）
│   ├── SharedConfig.swift     # App Group 共享配置（Key 清单见文件头注释）
│   ├── ApiClient.swift        # REST 封装 + SSE 日志流（主 App 与 Tunnel 共用）
│   ├── GomobileBridge.swift   # gomobile 对接点（canImport 自动切换真实现/占位）
│   ├── VPNManager.swift       # NETunnelProviderManager：安装配置、启停隧道
│   ├── ViewModels/            # NodeList / Subscription / Log / Settings
│   ├── Views/                # ContentView（Tab）/ NodeList / Subscription / Log / Settings
│   └── BigCatV.entitlements
└── PacketTunnel/             # NetworkExtension target
    ├── PacketTunnelProvider.swift  # NEPacketTunnelProvider：MobileStart → 配网卡 → 调 API 启动引擎
    └── PacketTunnel.entitlements
```

Bundle ID：主 App `com.bigcat.v`，Tunnel `com.bigcat.v.tunnel`，
App Group `group.com.bigcat.v`。

## 构建步骤

### 1. 生成 gomobile 绑定

由 core/mobile 产出（另一个 agent 负责），在本目录得到：

```bash
cd ~/workspace/bigcat-v/core/mobile
gomobile bind -target=ios -o Bigcatv.xcframework ./...
# 约定：模块名 Bigcatv，类 Mobile，静态方法 Start(apiAddr) / Stop() / Version()
```

### 2. 新建 Xcode 项目

1. Xcode → New Project → iOS App，Bundle Identifier 填 `com.bigcat.v`，
   Interface 选 SwiftUI，Language 选 Swift；
2. 把 `BigCatV/` 下全部 `.swift` 文件拖入项目（勾选主 App target）；
3. 把 `Bigcatv.xcframework` 拖入项目 → Target 的
   Frameworks, Libraries, and Embedded Content 中设为 **Embed & Sign**；
4. 把 `BigCatV.entitlements` 加入项目，Target → Signing & Capabilities
   确认 entitlements 文件被引用。

### 3. Signing & Capabilities（主 App target）

- **App Groups**：添加 `group.com.bigcat.v`（Xcode 会自动写 entitlements，
  与仓库内 `BigCatV.entitlements` 内容一致即可）；
- **Network Extensions**：勾选（主 App 需要该权限才能管理 VPN 配置）；
- **Background Modes**：如需后台保活可按需加，骨架未依赖。

### 4. 添加 NetworkExtension target

1. File → New → Target → **Network Extension** → Provider Type 选
   **Packet Tunnel Provider**；
2. Product Name 建议 `PacketTunnel`，Bundle Identifier 自动为
   `com.bigcat.v.tunnel`（如不是，手动改成它）；
3. 把 `PacketTunnel/PacketTunnelProvider.swift` 加入该 target，
   替换模板生成的同名文件；
4. 把 `GomobileBridge.swift`、`ApiClient.swift`、`Models.swift`、
   `SharedConfig.swift` **同时加入两个 target**
   （File Inspector → Target Membership 勾选主 App + PacketTunnel）；
5. `PacketTunnel.entitlements` 指定为该 target 的 entitlements 文件；
6. Signing & Capabilities（Tunnel target）：**App Groups**
  （同 `group.com.bigcat.v`）+ **Network Extensions**。

### 5. 真机调试

- Packet Tunnel Provider **必须在真机上调试**，模拟器不支持；
- 需要 Apple Developer 账号（免费账号可调试 7 天，NetworkExtension
  能力建议付费账号）；
- 首次点「安装 VPN 配置」/「连接」时，iOS 会弹系统授权框，
  用户必须在「设置 → 通用 → VPN 与设备管理」中允许；
- Xcode 菜单 Debug → Attach to Process 可附加到 `PacketTunnel` 扩展进程
  看 `NSLog` 输出（Console.app 中按子系统过滤）。

### 6. TestFlight / 发布注意

- App ID（`com.bigcat.v` 与 `com.bigcat.v.tunnel`）需在
  developer.apple.com 上分别开通 **Network Extensions** 与 **App Groups** 能力，
  并生成匹配的 Provisioning Profile；
- 主 App 与 Tunnel 的签名证书/Team 必须一致，否则扩展无法加载；
- 审核注意：VPN 类 App 需在 App Store Connect 备注中说明用途，
  且 packet-tunnel-provider 必须真实承载流量，不可仅作占位。

## 运行时架构

```
主 App 进程 (com.bigcat.v)
 ├─ SwiftUI → VPNManager → NETunnelProviderManager（启停隧道）
 └─ ApiClient → http://127.0.0.1:17890（Go 核心 REST API）
                        ↕ 同一设备回环，主 App 与扩展进程共享
PacketTunnel 进程 (com.bigcat.v.tunnel)
 ├─ GoCore.start("127.0.0.1:17890")   // 进程内启动 bigcatvd
 ├─ NEPacketTunnelNetworkSettings      // 198.18.0.1/24，全流量路由
 ├─ POST /api/v1/engine/start {tun:true} // sing-box，gVisor 用户态栈
 └─ packetFlow.readPackets ⇄ Go (gVisor netstack) ⇄ writePackets
```

设计假设（已在代码注释中标出，联调时确认）：
1. iOS 设备回环 127.0.0.1 在主 App 与扩展进程间共享，主 App 直连
   tunnel 进程内的 Go 核心 API；若实测不通，改用 App Group + Darwin 通知。
2. gomobile 产出 `Bigcatv.Mobile.start/stop/version`；收发包回调签名
   待 gomobile agent 确认（见 `GomobileBridge.swift` 尾部 TODO）。

## 本机无 Xcode 时的验证

本沙箱无 Xcode，以上 Swift 代码按 SwiftUI / NetworkExtension 公开文档的
标准 API 编写；iOS 16+ API（如 `NEIPv4Route.default()`、
`packetFlow.readPackets`、`NETunnelProviderManager.loadAllFromPreferences`）
均为稳定 API。在有 Xcode 的机器上按本文档步骤建项目即可编译。
