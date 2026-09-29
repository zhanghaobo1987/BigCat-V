# BigCat V

跨平台代理客户端，目标对标 Shadowrocket / ClashMate / v2rayN。

## 架构（一句话）

Go 语言编写的本地守护进程 `bigcatvd` 为跨平台核心：统一节点模型 →
订阅解析 → 生成 sing-box 配置 → 进程级管理 sing-box 内核 → REST + WebSocket
控制 API 供各端 UI（Tauri 桌面 / iOS / Android）调用。

## 目录结构

```
bigcat-v/
  README.md
  docs/
    ARCHITECTURE.md      # 架构设计文档
    API.md               # 控制 API 契约（待补充）
  core/                  # Go 核心（本仓库当前阶段的全部工作）
    cmd/bigcatvd/        # 守护进程入口
    internal/
      model/             # 统一节点模型（内部 IR）
      subscription/      # 订阅抓取与解析：分享链接 / Clash YAML / sing-box JSON
      generator/         # IR -> sing-box 配置生成器
      engine/            # sing-box 进程生命周期管理
      api/               # 本地 REST + WebSocket 控制 API
    VERSION              # 核心版本号（semver，每次变更递增）
```

## 快速开始（core）

```bash
cd core
go mod tidy
go build ./...
./bigcatvd --help
```

需要本机或 `./bin/` 下有 `sing-box` 可执行文件（引擎会自动查找 PATH 与 `./bin/sing-box`）。

## 路线图

- [x] v0.1 核心：统一节点模型、订阅解析、双内核配置生成、引擎进程管理、控制 API
- [x] v0.2 桌面 UI：Tauri 外壳（bigcatvd sidecar）+ 零依赖 Web 前端（节点/订阅/日志/设置）
- [x] v0.3 移动端骨架：gomobile 绑定 + sing-box/xray 进程内引擎（mobile 构建标签）+ Android（VpnService+Compose）/ iOS（PacketTunnel+SwiftUI）应用骨架；真机打包联调待开发机进行
  - [x] Go 核心绑定（`core/mobile`，gomobile）：Start/Stop/Version/SetLogLevel
  - [x] 进程内双引擎（`mobile` 构建标签）：sing-box v1.12.16 / xray-core v1.250725.0 库集成
  - [ ] 真机验证（.aar/.xcframework 打包与流量测试，需 NDK/Xcode）
  - [ ] App 侧样例（VpnService / NEPacketTunnelProvider）与 TUN fd 注入
- [x] v0.4 规则集：GeoIP / GeoSite 定时更新、自定义分流（routing.json 规则 CRUD+排序+默认动作；geo 后台定时下载 .srs/.dat，API 全套；双内核生成：sing-box rule_set+xray field rules；桌面端分流 Tab）
