import Foundation
import NetworkExtension

/// 主 App 侧的 VPN 开关：管理 NETunnelProviderManager（配置的增删、隧道启停）。
///
/// 数据面实际跑在 PacketTunnel 扩展进程里（见 PacketTunnelProvider），
/// 这里只负责：安装 VPN 配置、startVPNTunnel / stopVPNTunnel、监听状态变化。
@MainActor
final class VPNManager: ObservableObject {
    /// 与 tunnel target 的 Bundle ID 保持一致
    static let tunnelBundleID = "com.bigcat.v.tunnel"

    @Published private(set) var status: NEVPNStatus = .invalid
    @Published private(set) var isInstalled = false
    @Published var lastError: String?

    private var manager: NETunnelProviderManager?

    init() {
        NotificationCenter.default.addObserver(
            forName: .NEVPNStatusDidChange, object: nil, queue: .main
        ) { [weak self] _ in
            Task { @MainActor in self?.refreshStatus() }
        }
    }

    /// 从系统偏好中加载已安装的配置。
    func load() {
        NETunnelProviderManager.loadAllFromPreferences { [weak self] managers, _ in
            Task { @MainActor in
                guard let self else { return }
                if let m = managers?.first(where: {
                    ($0.protocolConfiguration as? NETunnelProviderProtocol)?
                        .providerBundleIdentifier == Self.tunnelBundleID
                }) {
                    self.manager = m
                    self.isInstalled = true
                } else {
                    self.manager = nil
                    self.isInstalled = false
                }
                self.refreshStatus()
            }
        }
    }

    /// 安装 VPN 配置（首次需用户在系统弹窗中授权）。
    func install() {
        let manager = NETunnelProviderManager()
        let proto = NETunnelProviderProtocol()
        proto.providerBundleIdentifier = Self.tunnelBundleID
        proto.serverAddress = "BigCat V"
        manager.protocolConfiguration = proto
        manager.localizedDescription = "BigCat V"
        manager.isEnabled = true
        manager.saveToPreferences { [weak self] error in
            Task { @MainActor in
                guard let self else { return }
                if let error {
                    self.lastError = "安装 VPN 配置失败：\(error.localizedDescription)"
                } else {
                    self.manager = manager
                    self.isInstalled = true
                    self.lastError = nil
                }
                self.load() // 重新加载以拿到系统侧最新状态
            }
        }
    }

    /// 连接开关：已连接则断开，否则启动隧道。
    func toggle() {
        guard let manager else { install(); return }
        let conn = manager.connection
        switch conn.status {
        case .connected, .connecting:
            conn.stopVPNTunnel()
        default:
            do {
                try conn.startVPNTunnel()
                lastError = nil
            } catch {
                lastError = "启动隧道失败：\(error.localizedDescription)"
            }
        }
    }

    func disconnect() {
        manager?.connection.stopVPNTunnel()
    }

    var isConnected: Bool { status == .connected }

    var statusText: String {
        switch status {
        case .invalid: return "未配置"
        case .disconnected: return "未连接"
        case .connecting: return "连接中…"
        case .connected: return "已连接"
        case .reasserting: return "重连中…"
        case .disconnecting: return "断开中…"
        @unknown default: return "未知"
        }
    }

    private func refreshStatus() {
        status = manager?.connection.status ?? .invalid
    }
}
