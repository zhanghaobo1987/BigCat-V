import SwiftUI

/// App 入口。
@main
struct BigCatVApp: App {
    @StateObject private var vpn = VPNManager()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(vpn)
                .onAppear { vpn.load() }
        }
    }
}

/// 主界面：四个 Tab。
struct ContentView: View {
    @EnvironmentObject private var vpn: VPNManager
    @StateObject private var nodesVM = NodeListViewModel()
    @StateObject private var subsVM = SubscriptionViewModel()

    var body: some View {
        TabView {
            NodeListView()
                .tabItem { Label("节点", systemImage: "list.bullet") }
            SubscriptionView()
                .tabItem { Label("订阅", systemImage: "arrow.triangle.2.circlepath") }
            LogView()
                .tabItem { Label("日志", systemImage: "terminal") }
            SettingsView()
                .tabItem { Label("设置", systemImage: "gear") }
        }
        .environmentObject(nodesVM)
        .environmentObject(subsVM)
        .onAppear {
            // 订阅变更后刷新节点列表
            subsVM.onChanged = { Task { await nodesVM.load() } }
        }
    }
}
