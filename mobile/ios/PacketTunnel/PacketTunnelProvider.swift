import NetworkExtension

/// BigCat V 数据面：NEPacketTunnelProvider 子类，跑在独立的扩展进程中。
///
/// 启动顺序（startTunnel）：
///  1. 从 App Group 读共享配置（选中节点 / 内核 / 端口）；
///  2. 调 MobileStart 在本进程内启动 Go 核心 bigcatvd
///     （REST API 监听 127.0.0.1:17890；主 App 经同一设备回环访问同一地址）；
///  3. 配置 NEPacketTunnelNetworkSettings（虚拟网卡 198.18.0.1/24、全流量路由、DNS）；
///  4. 调本地 API POST /api/v1/engine/start {tun: true} 启动 sing-box；
///  5. 启动 packetFlow 读包循环，把 IP 包交给 Go 侧。
///
/// 关键约束（与桌面端不同）：
///  - iOS 不允许 spawn 子进程，因此不能像桌面端那样起 sing-box 独立进程；
///    sing-box 以 Go library 形式跑在 tunnel 进程内，tun 入口走 sing-box 的
///    gVisor 用户态协议栈（无需系统 tun 字符设备）。
///  - iOS 上内核固定为 sing-box（xray 无进程内 library 形态）；API 侧仍保留
///    kernel 参数，XraySelected 时此处强制回退并记录日志。
final class PacketTunnelProvider: NEPacketTunnelProvider {
    /// tunnel 进程内直连的 Go 核心 API
    private let apiAddr = "127.0.0.1:17890"
    private var reading = false

    private var client: ApiClient {
        ApiClient(baseURL: URL(string: "http://\(apiAddr)")!)
    }

    override func startTunnel(
        options: [String: NSObject]?,
        completionHandler: @escaping (Error?) -> Void
    ) {
        // 1. 启动进程内 Go 核心（含 REST 控制 API）
        do {
            try GoCore.start(apiAddr: apiAddr)
        } catch {
            completionHandler(error)
            return
        }

        // 2. 配置虚拟网卡：全流量走隧道
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "198.18.0.1")
        let v4 = NEIPv4Settings(addresses: ["198.18.0.1"], subnetMasks: ["255.255.255.0"])
        v4.includedRoutes = [NEIPv4Route.default()] // 0.0.0.0/0 全流量
        v4.excludedRoutes = []
        settings.ipv4Settings = v4
        // DNS 走隧道，避免本地 DNS 泄漏
        settings.dnsSettings = NEDNSSettings(servers: ["223.5.5.5", "119.29.29.29"])
        settings.mtu = 1500 as NSNumber

        setTunnelNetworkSettings(settings) { [weak self] error in
            guard let self else { return }
            if let error {
                GoCore.stop()
                completionHandler(error)
                return
            }
            // 3. 调 API 启动引擎（iOS 恒走 tun，gVisor 用户态栈）
            Task {
                do {
                    var kernel = SharedConfig.kernel
                    if kernel != "sing-box" {
                        // iOS 无 xray 进程内形态，强制回退
                        kernel = "sing-box"
                        NSLog("[BigCatV] iOS 不支持 xray 进程内运行，已回退到 sing-box")
                    }
                    let nodeID = SharedConfig.selectedNodeID
                    let resp = try await self.client.startEngine(
                        nodeID: nodeID,
                        kernel: kernel,
                        tun: true,
                        mixedPort: SharedConfig.mixedPort
                    )
                    if let skipped = resp.skipped, !skipped.isEmpty {
                        NSLog("[BigCatV] 引擎启动时跳过节点：\(skipped)")
                    }
                    self.startPacketLoop()
                    completionHandler(nil)
                } catch {
                    GoCore.stop()
                    completionHandler(error)
                }
            }
        }
    }

    override func stopTunnel(
        with reason: NEProviderStopReason,
        completionHandler: @escaping () -> Void
    ) {
        reading = false
        Task {
            // 先停引擎，再停 Go 核心；失败也不阻塞隧道关闭
            try? await client.stopEngine()
            GoCore.stop()
            completionHandler()
        }
    }

    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        // 预留：主 App 经 NETunnelProviderSession.sendProviderMessage 下发指令
        // （如切换节点），v0.3 骨架暂不实现。
        completionHandler?(nil)
    }

    // MARK: - 数据面收发包

    /// 从虚拟网卡读包 → 交给 Go 侧（gVisor netstack）。
    private func startPacketLoop() {
        reading = true
        packetFlow.readPackets { [weak self] packets, _ in
            guard let self else { return }
            for packet in packets {
                GoPacketBridge.input(packet)
            }
            if self.reading {
                self.startPacketLoop() // 持续读
            }
        }
    }

    /// Go 侧回包 → 写回虚拟网卡。
    /// TODO(gomobile)：在 Go 输出回调中调用本方法。
    func writePacketsBack(_ packets: [Data], protocols: [NSNumber]) {
        packetFlow.writePackets(packets, withProtocols: protocols)
    }
}
