import SwiftUI

/// 日志页：SSE 实时日志流（GET /api/v1/events）。
struct LogView: View {
    @StateObject private var vm = LogViewModel()

    var body: some View {
        NavigationStack {
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 4) {
                        ForEach(Array(vm.streamer.lines.enumerated()), id: \.offset) { _, line in
                            Text(line)
                                .font(.system(.caption, design: .monospaced))
                                .textSelection(.enabled)
                        }
                    }
                    .padding(.horizontal)
                    .id("logEnd")
                }
                .onReceive(vm.streamer.$lines) { _ in
                    // 新日志到来时自动滚到底
                    withAnimation { proxy.scrollTo("logEnd", anchor: .bottom) }
                }
            }
            .navigationTitle("日志")
            .toolbar {
                ToolbarItem(placement: .navigationBarLeading) {
                    Text(vm.streamer.connected ? "● 已连接" : "○ 未连接")
                        .font(.caption)
                        .foregroundColor(vm.streamer.connected ? .green : .secondary)
                }
                ToolbarItem(placement: .navigationBarTrailing) {
                    HStack {
                        Button("清空") { vm.streamer.clear() }
                        Button(vm.streamer.connected ? "停止" : "开始") {
                            vm.streamer.connected ? vm.stop() : vm.start()
                        }
                    }
                }
            }
            .onAppear { vm.start() }
            .onDisappear { vm.stop() }
        }
    }
}
