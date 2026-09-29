import Foundation
#if canImport(Bigcatv)
import Bigcatv
#endif

// MARK: - gomobile 对接点
//
// 另一个 agent 以 `gomobile bind -target=ios` 生成 Bigcatv.xcframework 后，
// 本文件 `#if canImport(Bigcatv)` 分支自动启用真实现；xcframework 未接入时
// 走 #else 占位分支，保证主 App / Tunnel target 可独立编译。
//
// 假设的 Go 侧签名（由 gomobile agent 最终确认）：
//   func Start(apiAddr string) error  // 在本进程内启动 bigcatvd，REST 监听 apiAddr（如 "127.0.0.1:17890"）
//   func Stop()
//   func Version() string
//
// 若实际产出为包级函数（MobileStart / MobileStop / MobileVersion），只改本文件三处调用。

enum GoCoreError: LocalizedError {
    /// xcframework 尚未接入
    case notIntegrated
    case startFailed(String)

    var errorDescription: String? {
        switch self {
        case .notIntegrated: return "Bigcatv.xcframework 未集成"
        case .startFailed(let msg): return "Go 核心启动失败：\(msg)"
        }
    }
}

enum GoCore {
#if canImport(Bigcatv)
    /// 启动进程内 Go 核心（含 REST 控制 API）。
    static func start(apiAddr: String) throws {
        // gomobile 把返回 error 的 Go 函数映射为 Swift throws
        try Bigcatv.Mobile.start(apiAddr)
    }

    static func stop() {
        Bigcatv.Mobile.stop()
    }

    static func version() -> String {
        Bigcatv.Mobile.version()
    }
#else
    static func start(apiAddr: String) throws {
        throw GoCoreError.notIntegrated
    }

    static func stop() {}

    static func version() -> String {
        "未集成 Bigcatv.xcframework"
    }
#endif
}

// MARK: - 数据面收发包对接点（待 gomobile agent 确认）
//
// iOS 上 sing-box 跑在 tunnel 进程内的 Go 侧（gVisor 用户态栈），IP 包需在
// Swift packetFlow 与 Go 之间流转。假设 Go 侧提供：
//   - Swift → Go：Bigcatv.Mobile.inputPacket(_ data: Data)
//     // 把 packetFlow.readPackets 读到的出站包交给 Go
//   - Go → Swift：Go 侧注册输出回调，回调内调
//     packetFlow.writePackets([data], withProtocols: [protoNumber])
//     // 把 Go 回包写回虚拟网卡
//
// 确认签名后，在 PacketTunnelProvider.startPacketLoop() 与回调处接上即可。
enum GoPacketBridge {
    static func input(_ packet: Data) {
        // TODO(gomobile)：接到 Go 侧入包接口
        _ = packet
    }
}
