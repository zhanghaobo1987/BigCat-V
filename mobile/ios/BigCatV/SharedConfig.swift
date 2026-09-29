import Foundation

/// App Group 共享配置：主 App 与 PacketTunnel 扩展通过它交换配置。
///
/// 共享区 Key 清单（读写双方必须一致）：
/// | Key              | 类型   | 写入方  | 读取方   | 说明                  |
/// |------------------|--------|---------|----------|-----------------------|
/// | api_base_url     | String | 主 App  | 主 App   | 控制 API 地址，默认 http://127.0.0.1:17890 |
/// | selected_node_id | String | 主 App  | Tunnel   | 当前选中节点 ID，Tunnel 启动引擎时用 |
/// | kernel           | String | 主 App  | Tunnel   | "sing-box" / "xray"，默认 sing-box |
/// | tun_enabled      | Bool   | 主 App  | Tunnel   | 是否启用 tun（iOS 上恒为 true，走 gVisor 用户态栈）|
/// | mixed_port       | Int    | 主 App  | Tunnel   | 本地混合入站端口，默认 7890 |
enum SharedConfig {
    static let suiteName = "group.com.bigcat.v"
    static let defaultAPIBaseURL = "http://127.0.0.1:17890"
    static let defaultKernel = "sing-box"
    static let defaultMixedPort = 7890

    static let apiBaseURLKey = "api_base_url"
    static let selectedNodeKey = "selected_node_id"
    static let kernelKey = "kernel"
    static let tunEnabledKey = "tun_enabled"
    static let mixedPortKey = "mixed_port"

    /// 共享 UserDefaults。App Group 未配置时回退到 standard（仅调试）。
    static var defaults: UserDefaults {
        UserDefaults(suiteName: suiteName) ?? .standard
    }

    static var apiBaseURL: String {
        get { defaults.string(forKey: apiBaseURLKey) ?? defaultAPIBaseURL }
        set { defaults.set(newValue, forKey: apiBaseURLKey) }
    }

    static var apiBaseURLValue: URL {
        URL(string: apiBaseURL) ?? URL(string: defaultAPIBaseURL)!
    }

    static var selectedNodeID: String? {
        get { defaults.string(forKey: selectedNodeKey) }
        set { defaults.set(newValue, forKey: selectedNodeKey) }
    }

    static var kernel: String {
        get { defaults.string(forKey: kernelKey) ?? defaultKernel }
        set { defaults.set(newValue, forKey: kernelKey) }
    }

    static var mixedPort: Int {
        get {
            let v = defaults.integer(forKey: mixedPortKey)
            return v >= 1024 ? v : defaultMixedPort
        }
        set { defaults.set(newValue, forKey: mixedPortKey) }
    }
}
