# BigCat V 架构设计

版本：v0.1（核心先行）| 日期：2026-09-29

## 1. 设计目标

做一款与 Shadowrocket / ClashMate / v2rayN 同形态的代理客户端，
命名 BigCat V。首阶段聚焦**跨平台核心**：协议解析与引擎调度能力先行，
UI 层（桌面 / 移动）后续通过稳定 API 对接，做到一次实现、多端复用。

非目标：自研代理协议栈（复用 sing-box 内核）、自研规则数据库（复用
GeoIP / Geosite 社区数据）。

## 2. 总体分层

```
┌─────────────────────────────────────────────────────┐
│ UI 层（后续）  Tauri 桌面 / iOS / Android            │  只调本地 API
├─────────────────────────────────────────────────────┤
│ 控制 API       REST（127.0.0.1） + WebSocket 事件流   │  bigcatvd 对外唯一界面
├─────────────────────────────────────────────────────┤
│ 核心服务 bigcatvd（Go，跨平台编译）                   │
│  ┌────────────┐ ┌──────────────┐ ┌────────────────┐  │
│  │ subscription│ │ generator    │ │ engine.Core    │  │
│  │ 订阅抓取解析 │ │ IR→内核配置  │ │ 内核抽象接口    │  │
│  │            │ │ sing-box/xray │ │ sing-box+xray  │  │
│  └────────────┘ └──────────────┘ └────────────────┘  │
│  ┌──────────────────────────────────────────────┐   │
│  │ model：统一节点模型（内部 IR，所有协议归一）    │   │
│  └──────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────┤
│ 数据面 双内核（二选一，官方 release 二进制）            │
│   sing-box：全协议（含 hysteria2/tuic）/ 新特性快      │
│   xray：    vless/vmess 生态成熟，缺 hy2/tuic         │
└─────────────────────────────────────────────────────┘
```

双内核设计（`engine.Core` 接口）：

- sing-box 与 xray 各自实现 `Core`（查找二进制 / check / run / 日志 / 重启），
  上层只依赖接口；
- generator 为每种内核各一个翻译器：`Marshal`（sing-box）、`MarshalXray`；
- 协议兼容矩阵：xray 不支持 hysteria2 / tuic，`generator.XraySupported`
  表达，节点列表 API 直接返回 `xray_compatible` 标记，启动 xray 内核时
  自动过滤并告知跳过了哪些节点；
- API 启动参数 `POST /api/v1/engine/start {"kernel": "sing-box"|"xray"}`，
  `GET /api/v1/kernels` 查询本机可用性（缺二进制只告警、不致命）。

为什么 sing-box 独立进程而不是 library：

- v2rayN + Xray / Clash Verge + Mihomo 都是成熟验证的形态，升级内核只需换二进制；
- 崩溃隔离：数据面崩溃不拖垮控制面；
- 避免把 sing-box 庞大依赖树编译进核心，保持 `bigcatvd` 轻量。

## 3. 统一节点模型（model）

所有输入（分享链接、Clash YAML、sing-box JSON、手动录入）先归一化为
`model.Node`（内部 IR），再由 generator 翻译为 sing-box outbound。
新增协议只需加 parser + generator 分支，不动 API 与 UI。

字段分组：基础（id/name/protocol/server/port）/ 认证（uuid/password/method）
/ 传输（network=tcp|ws|grpc|h2|http，tls/sni/alpn/ws path-host/grpc service）
/ 协议特有（vless flow、hy2/tuic obfs、wireguard key 套件）/ 元信息（group、
latency、raw 原始链接）。

`Node.ID` 为归一化关键字段的稳定哈希，订阅更新时用于去重与保留用户备注。

## 4. 订阅解析（subscription）

输入格式与解析器：

| 输入 | 解析器 | 说明 |
|---|---|---|
| base64 批量分享链接 | `ParseShareLinks` | ss/vmess/vless/trojan/hysteria2/tuic/wireguard/socks/http |
| Clash YAML | `ParseClash` | `proxies:` 列表，支持全部主流类型 |
| sing-box JSON | `ParseSingBox` | `outbounds[]` 直接映射 |
| HTTP(S) 订阅地址 | `Fetch` | 自定义 UA，自动识别上述三种载荷 |

`Fetch` 只做传输层（超时、UA、状态码检查），解析层对字节流做嗅探：
先试 JSON（sing-box / vmess 单条），再试 YAML（proxies:），最后按 base64
批量链接处理。

## 5. 配置生成（generator）

`Generate(nodes, opts)` 输出完整 sing-box 配置：

- inbounds：`mixed`（127.0.0.1:7890，HTTP+SOCKS5 二合一）为默认入口；
  `tun` 入口为可选项（需要系统权限，由 UI 侧申请）。
- outbounds：每个节点一个 outbound（tag = `node.ID`），另带
  `selector`（tag=`proxy`，手动选路）、`urltest`（tag=`auto`，自动低延迟）、
  `direct`、`block`。
- route：默认规则集——局域网/回环直连、常见 DNS 劫持防护、final=proxy；
  规则文件（GeoIP/Geosite）路径由 `opts` 注入，引擎负责定时更新。

生成后必经 `sing-box check -c` 校验，失败则拒绝热加载并保留上一个可用配置。

## 6. 引擎管理（engine）

`Engine` 负责 sing-box 二进制的查找（`./bin/sing-box` → `PATH`）、
`check` 校验、`run -c` 启动、stdout/stderr 日志采集、优雅停止与崩溃重启
（退避策略）。同一时刻只允许一个实例持有数据面；`Reload` 走“生成→校验→
重启”三段式，保证不断连语义尽力而为。

延迟测试：优先复用 sing-box 的 `urltest`；单节点 TCP 握手延迟由核心自行实现，
供 UI 排序展示。

## 7. 控制 API（api）

监听 `127.0.0.1:17890`（端口可在启动参数覆盖），本机回环、无鉴权
（后续加 token）：

- `GET /api/v1/status` —— 引擎状态、当前节点、版本
- `GET/POST /api/v1/nodes` —— 节点列表 / 手动添加
- `DELETE /api/v1/nodes/{id}` —— 删除节点
- `POST /api/v1/nodes/{id}/test` —— 延迟测试
- `GET/POST /api/v1/subscriptions` —— 订阅列表 / 新增
- `POST /api/v1/subscriptions/{id}/refresh` —— 手动刷新
- `POST /api/v1/engine/start {node_id, tun}` —— 启动
- `POST /api/v1/engine/stop` —— 停止
- `GET /api/v1/engine/config` —— 查看当前生成的 sing-box 配置
- `WS /api/v1/events` —— 日志 / 流量 / 节点状态推送

## 8. 构建矩阵

`GOOS`×`GOARCH` 交叉编译：windows-amd64、darwin-amd64/arm64、
linux-amd64/arm64、android-arm64（后接 VpnService）、ios-arm64（后接
Network Extension）。sing-box 二进制按平台配对分发，`bigcatvd` 启动时校验
两者版本兼容性。

## 9. 本阶段交付

- [x] model：统一节点模型
- [x] subscription：分享链接解析器（含单测）、Clash YAML、sing-box JSON、订阅抓取
- [x] generator：IR → sing-box / xray 双配置生成器（含单测与 xray 兼容矩阵）
- [x] engine：`Core` 双内核抽象，sing-box + xray 进程生命周期管理
- [x] api：REST（含内核选择 / 可用性查询 / mixed_port）+ SSE 事件流 + daemon 入口
- [x] desktop v0.2：Tauri 外壳（bigcatvd sidecar 拉起/回收）+ 零依赖 Web 前端
      （节点管理/延迟测速/订阅/日志/SSE/设置），`node --check` 与 API 联调通过
- [x] 移动端绑定 v0.3（一期）：`core/mobile` gomobile 包 +
      进程内双引擎（`mobile || android || ios` 标签隔离，不影响默认构建）+
      `internal/bootstrap` 启动逻辑复用；详见 `mobile/ARCHITECTURE.md`
- [ ] 下一阶段：v0.3 真机验证（Android .aar / iOS .xcframework 打包与流量测试）
