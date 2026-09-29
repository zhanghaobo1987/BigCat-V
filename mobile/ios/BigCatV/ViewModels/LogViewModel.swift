import Combine
import Foundation

/// 日志页的数据层：封装 LogStreamer（SSE /api/v1/events）。
/// 转发内层 streamer 的 objectWillChange，保证 SwiftUI 能感知行数变化。
@MainActor
final class LogViewModel: ObservableObject {
    let streamer = LogStreamer()
    private var cancellables = Set<AnyCancellable>()

    init() {
        streamer.objectWillChange
            .sink { [weak self] _ in self?.objectWillChange.send() }
            .store(in: &cancellables)
    }

    func start() {
        streamer.start(baseURL: SharedConfig.apiBaseURLValue)
    }

    func stop() {
        streamer.stop()
    }
}
