import Foundation

/// bigcatvd 控制 API 错误。
enum APIError: LocalizedError {
    case transport(Error) // 网络层错误
    case server(status: Int, message: String) // 服务端返回非 2xx（{"error": "..."}）
    case decoding(Error) // 响应 JSON 解析失败

    var errorDescription: String? {
        switch self {
        case .transport(let e): return "网络错误：\(e.localizedDescription)"
        case .server(let s, let m): return "服务端错误 (\(s))：\(m)"
        case .decoding(let e): return "响应解析失败：\(e.localizedDescription)"
        }
    }
}

/// bigcatvd REST 客户端。端点契约见 core/internal/api/server.go。
/// 本文件只依赖 Foundation，主 App 与 PacketTunnel 两个 target 共用。
/// Sendable：支持在 TaskGroup 并发测速中共享。
final class ApiClient: Sendable {
    let baseURL: URL
    private let session: URLSession

    init(baseURL: URL, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.session = session
    }

    // MARK: - 通用请求

    /// 发起 JSON 请求并解码。非 2xx 时按 {"error": msg} 抛错。
    func request<T: Decodable>(
        _ path: String,
        method: String = "GET",
        body: (any Encodable)? = nil
    ) async throws -> T {
        var req = URLRequest(url: baseURL.appendingPathComponent(path))
        req.httpMethod = method
        req.timeoutInterval = 15
        if let body {
            req.httpBody = try JSONEncoder().encode(body)
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: req)
        } catch {
            throw APIError.transport(error)
        }
        guard let http = response as? HTTPURLResponse else {
            throw APIError.transport(URLError(.badServerResponse))
        }
        guard (200..<300).contains(http.statusCode) else {
            let msg = (try? JSONDecoder().decode(ServerError.self, from: data))?.error
                ?? "HTTP \(http.statusCode)"
            throw APIError.server(status: http.statusCode, message: msg)
        }
        do {
            return try JSONDecoder().decode(T.self, from: data)
        } catch {
            throw APIError.decoding(error)
        }
    }

    /// 返回原始文本的请求（用于 GET /api/v1/engine/config）。
    func requestText(_ path: String) async throws -> String {
        var req = URLRequest(url: baseURL.appendingPathComponent(path))
        req.timeoutInterval = 15
        let (data, response) = try await session.data(for: req)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw APIError.server(status: (response as? HTTPURLResponse)?.statusCode ?? -1,
                                 message: "读取配置失败")
        }
        return String(data: data, encoding: .utf8) ?? ""
    }

    // MARK: - 状态与内核

    func status() async throws -> EngineStatus {
        try await request("/api/v1/status")
    }

    func kernels() async throws -> [KernelInfo] {
        let r: KernelsResponse = try await request("/api/v1/kernels")
        return r.kernels
    }

    // MARK: - 节点

    func listNodes() async throws -> [ProxyNode] {
        let r: NodesResponse = try await request("/api/v1/nodes")
        return r.nodes
    }

    /// 手动添加节点（分享链接）。
    func addNode(link: String) async throws -> ProxyNode {
        try await request("/api/v1/nodes", method: "POST", body: AddNodeRequest(link: link))
    }

    func deleteNode(id: String) async throws {
        let _: DeleteNodeResponse = try await request("/api/v1/nodes/\(id)", method: "DELETE")
    }

    /// 单节点延迟测试，返回毫秒（-1 表示失败）。
    func testNode(id: String) async throws -> Int64 {
        let r: TestNodeResponse = try await request("/api/v1/nodes/\(id)/test", method: "POST")
        return r.latencyMs
    }

    // MARK: - 订阅

    func listSubscriptions() async throws -> [Subscription] {
        let r: SubscriptionsResponse = try await request("/api/v1/subscriptions")
        return r.subscriptions
    }

    func addSubscription(name: String?, url: String) async throws -> AddSubResponse {
        try await request("/api/v1/subscriptions", method: "POST",
                          body: AddSubRequest(name: name, url: url))
    }

    /// 刷新订阅，返回本次新增/更新的节点数。
    func refreshSubscription(id: String) async throws -> Int {
        let r: RefreshSubResponse = try await request(
            "/api/v1/subscriptions/\(id)/refresh", method: "POST")
        return r.added
    }

    // MARK: - 引擎

    /// 启动引擎。nodeID 为空表示用全部节点；tun 在 iOS 上恒为 true。
    func startEngine(nodeID: String?, kernel: String?, tun: Bool, mixedPort: Int?) async throws -> EngineStartResponse {
        try await request("/api/v1/engine/start", method: "POST",
                          body: EngineStartRequest(nodeID: nodeID, kernel: kernel,
                                                   tun: tun, mixedPort: mixedPort))
    }

    func stopEngine() async throws {
        let _: EngineStopResponse = try await request("/api/v1/engine/stop", method: "POST")
    }

    /// 查看当前生成的内核配置（原始 JSON 文本）。
    func engineConfig() async throws -> String {
        try await requestText("/api/v1/engine/config")
    }
}

// MARK: - SSE 日志流（GET /api/v1/events）

/// 用 URLSessionDataDelegate 流式读取 text/event-stream，
/// 按空行切事件，只处理 event: log（data 为 {"line": "..."}）。
final class LogStreamer: NSObject, ObservableObject, URLSessionDataDelegate {
    @Published private(set) var lines: [String] = []
    @Published private(set) var connected = false

    private var session: URLSession?
    private var task: URLSessionDataTask?
    private var buffer = ""
    private let maxLines = 500 // 日志页只保留最近 500 行

    /// 开始订阅日志流。
    func start(baseURL: URL) {
        stop()
        buffer = ""
        let session = URLSession(configuration: .default, delegate: self, delegateQueue: nil)
        self.session = session
        let url = baseURL.appendingPathComponent("/api/v1/events")
        let task = session.dataTask(with: url)
        self.task = task
        task.resume()
        DispatchQueue.main.async { self.connected = true }
    }

    func stop() {
        task?.cancel()
        task = nil
        session?.invalidateAndCancel()
        session = nil
        DispatchQueue.main.async { self.connected = false }
    }

    func clear() {
        DispatchQueue.main.async { self.lines = [] }
    }

    // MARK: URLSessionDataDelegate

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
        guard let chunk = String(data: data, encoding: .utf8) else { return }
        buffer += chunk
        // SSE 事件以空行分隔
        while let range = buffer.range(of: "\n\n") {
            let block = String(buffer[..<range.lowerBound])
            buffer = String(buffer[range.upperBound...])
            handleEventBlock(block)
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        // 连接断开（服务端只在客户端断开时结束流）；UI 侧可按需重连
        DispatchQueue.main.async { self.connected = false }
    }

    private func handleEventBlock(_ block: String) {
        var event = ""
        var data = ""
        for rawLine in block.components(separatedBy: "\n") {
            let line = rawLine.trimmingCharacters(in: .whitespaces)
            if line.hasPrefix("event:") {
                event = String(line.dropFirst(6)).trimmingCharacters(in: .whitespaces)
            } else if line.hasPrefix("data:") {
                data += String(line.dropFirst(5)).trimmingCharacters(in: .whitespaces)
            }
        }
        guard event == "log",
              let json = data.data(using: .utf8),
              let obj = try? JSONSerialization.jsonObject(with: json) as? [String: String],
              let line = obj["line"]
        else { return }
        DispatchQueue.main.async {
            self.lines.append(line)
            if self.lines.count > self.maxLines {
                self.lines.removeFirst(self.lines.count - self.maxLines)
            }
        }
    }
}
