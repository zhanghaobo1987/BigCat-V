import Foundation

// MARK: - 节点（对应 Go 侧 model.Node + nodeView）

/// 代理节点。JSON 来自 GET /api/v1/nodes，内嵌 model.Node 字段 + xray 兼容标记。
struct ProxyNode: Codable, Identifiable, Hashable {
    var id: String
    var name: String
    var proto: String // JSON 键 "protocol"，Swift 关键字故改名
    var server: String
    var port: Int
    var group: String?
    var latencyMs: Int64?
    var xrayCompatible: Bool
    var xrayNote: String?

    enum CodingKeys: String, CodingKey {
        case id, name, server, port, group
        case proto = "protocol"
        case latencyMs = "latency_ms"
        case xrayCompatible = "xray_compatible"
        case xrayNote = "xray_note"
    }

    /// 延迟展示：nil / -1 视为未测
    var latencyText: String {
        guard let ms = latencyMs, ms >= 0 else { return "未测" }
        return "\(ms) ms"
    }

    /// 排序用延迟：未测排最后
    var sortLatency: Int64 { (latencyMs ?? -1) < 0 ? Int64.max : latencyMs! }

    var displayName: String {
        guard let g = group, !g.isEmpty else { return name }
        return "[\(g)] \(name)"
    }
}

struct NodesResponse: Codable { var nodes: [ProxyNode] }
struct AddNodeRequest: Codable { var link: String }
struct DeleteNodeResponse: Codable { var deleted: String }
struct TestNodeResponse: Codable {
    var id: String
    var latencyMs: Int64
    enum CodingKeys: String, CodingKey { case id; case latencyMs = "latency_ms" }
}

// MARK: - 订阅（对应 Go 侧 api.Subscription）

struct Subscription: Codable, Identifiable, Hashable {
    var id: String
    var name: String
    var url: String
    var count: Int
}

struct SubscriptionsResponse: Codable { var subscriptions: [Subscription] }
struct AddSubRequest: Codable { var name: String?; var url: String }
struct AddSubResponse: Codable { var subscription: Subscription; var added: Int }
struct RefreshSubResponse: Codable { var refreshed: String; var added: Int }

// MARK: - 内核与引擎状态（对应 engine.Snapshot / API status）

struct KernelInfo: Codable, Identifiable, Hashable {
    var name: String
    var available: Bool
    var binary: String?
    var version: String?
    var id: String { name }
}

struct KernelsResponse: Codable { var kernels: [KernelInfo] }

struct EngineSnapshot: Codable {
    var status: String? // "stopped" | "running" | "error"
    var binary: String?
    var restarts: Int?
    var lastError: String?

    enum CodingKeys: String, CodingKey {
        case status, binary, restarts
        case lastError = "last_error"
    }

    var isRunning: Bool { status == "running" }
}

struct EngineStatus: Codable {
    var version: String?
    var kernel: String?
    var engine: EngineSnapshot?
    var activeNode: String?
    var nodeCount: Int?

    enum CodingKeys: String, CodingKey {
        case version, kernel, engine
        case activeNode = "active_node"
        case nodeCount = "node_count"
    }
}

struct EngineStartRequest: Codable {
    var nodeID: String?
    var kernel: String?
    var tun: Bool
    var mixedPort: Int?

    enum CodingKeys: String, CodingKey {
        case kernel, tun
        case nodeID = "node_id"
        case mixedPort = "mixed_port"
    }
}

struct EngineStartResponse: Codable {
    var started: Bool?
    var kernel: String?
    var config: String?
    var nodes: Int?
    var skipped: [String]?
}

struct EngineStopResponse: Codable { var stopped: Bool }

// MARK: - 服务端错误体 {"error": "..."}

struct ServerError: Codable { var error: String? }
