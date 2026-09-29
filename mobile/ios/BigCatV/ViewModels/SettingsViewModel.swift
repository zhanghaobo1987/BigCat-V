import Foundation

/// 设置页的数据层：API 地址 / 内核选择 / 端口，均写入 App Group 共享区。
@MainActor
final class SettingsViewModel: ObservableObject {
    @Published var apiBaseURL: String = SharedConfig.apiBaseURL {
        didSet { SharedConfig.apiBaseURL = apiBaseURL }
    }
    @Published var kernel: String = SharedConfig.kernel {
        didSet { SharedConfig.kernel = kernel }
    }
    @Published var mixedPortText: String = String(SharedConfig.mixedPort) {
        didSet {
            if let v = Int(mixedPortText), v >= 1024, v <= 65535 {
                SharedConfig.mixedPort = v
            }
        }
    }
    @Published var kernels: [KernelInfo] = []
    @Published var coreVersion: String = ""
    @Published var engineStatus: EngineStatus?
    @Published var configText: String = ""
    @Published var showingConfig = false
    @Published var errorMessage: String?

    private var client: ApiClient { ApiClient(baseURL: SharedConfig.apiBaseURLValue) }

    func load() async {
        do {
            async let k = client.kernels()
            async let s = client.status()
            kernels = try await k
            let st = try await s
            engineStatus = st
            coreVersion = st.version ?? GoCore.version()
            errorMessage = nil
        } catch {
            // Go 核心未启动时 API 不可达：显示本地版本占位
            coreVersion = GoCore.version()
            errorMessage = error.localizedDescription
        }
    }

    /// 查看当前内核配置 JSON。
    func loadConfig() async {
        do {
            configText = try await client.engineConfig()
            showingConfig = true
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
