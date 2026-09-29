import SwiftUI

/// 设置页：API 地址、内核选择、端口、版本与当前配置查看。
struct SettingsView: View {
    @EnvironmentObject private var vpn: VPNManager
    @StateObject private var vm = SettingsViewModel()

    var body: some View {
        NavigationStack {
            Form {
                Section("控制 API") {
                    TextField("API 地址", text: $vm.apiBaseURL)
                        .keyboardType(.URL)
                        .autocapitalization(.none)
                    Text("主 App 与 Tunnel 扩展共用此地址（默认 http://127.0.0.1:17890）")
                        .font(.caption).foregroundColor(.secondary)
                }

                Section("引擎") {
                    Picker("内核", selection: $vm.kernel) {
                        ForEach(vm.kernels) { k in
                            Text("\(k.name)\(k.available ? "" : "（不可用）")")
                                .tag(k.name)
                        }
                    }
                    TextField("本地混合端口", text: $vm.mixedPortText)
                        .keyboardType(.numberPad)
                    HStack {
                        Text("VPN 状态")
                        Spacer()
                        Text(vpn.statusText).foregroundColor(.secondary)
                    }
                    if !vpn.isInstalled {
                        Button("安装 VPN 配置") { vpn.install() }
                    }
                }

                Section("版本") {
                    HStack {
                        Text("核心版本")
                        Spacer()
                        Text(vm.coreVersion).foregroundColor(.secondary)
                    }
                    if let st = vm.engineStatus {
                        HStack {
                            Text("引擎")
                            Spacer()
                            Text("\(st.kernel ?? "-") / \(st.engine?.status ?? "-")")
                                .foregroundColor(.secondary)
                        }
                        HStack {
                            Text("节点数")
                            Spacer()
                            Text("\(st.nodeCount ?? 0)").foregroundColor(.secondary)
                        }
                    }
                    Button("查看当前内核配置") { Task { await vm.loadConfig() } }
                }

                if let err = vm.errorMessage {
                    Section { Text(err).font(.caption).foregroundColor(.red) }
                }
            }
            .navigationTitle("设置")
            .sheet(isPresented: $vm.showingConfig) {
                NavigationStack {
                    ScrollView {
                        Text(vm.configText)
                            .font(.system(.caption, design: .monospaced))
                            .textSelection(.enabled)
                            .padding()
                    }
                    .navigationTitle("内核配置")
                    .toolbar {
                        ToolbarItem(placement: .confirmationAction) {
                            Button("关闭") { vm.showingConfig = false }
                        }
                    }
                }
            }
            .task { await vm.load() }
        }
    }
}
