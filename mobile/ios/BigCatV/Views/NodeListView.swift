import SwiftUI

/// 节点列表页：顶部连接开关 + 节点行（名称/协议/延迟/选中态）。
struct NodeListView: View {
    @EnvironmentObject private var vpn: VPNManager
    @EnvironmentObject private var vm: NodeListViewModel
    @State private var showingImport = false

    var body: some View {
        NavigationStack {
            List {
                // 连接控制区
                Section {
                    HStack {
                        VStack(alignment: .leading) {
                            Text(vpn.statusText).font(.headline)
                            if let err = vpn.lastError {
                                Text(err).font(.caption).foregroundColor(.red)
                            } else if let id = vm.selectedNodeID,
                                      let node = vm.nodes.first(where: { $0.id == id }) {
                                Text("当前：\(node.displayName)").font(.caption).foregroundColor(.secondary)
                            } else {
                                Text("未选择节点").font(.caption).foregroundColor(.secondary)
                            }
                        }
                        Spacer()
                        Button(vpn.isConnected ? "断开" : "连接") { vpn.toggle() }
                            .buttonStyle(.borderedProminent)
                            .tint(vpn.isConnected ? .red : .green)
                    }
                }

                // 节点行
                Section("节点（\(vm.nodes.count)）") {
                    ForEach(vm.displayedNodes) { node in
                        NodeRow(node: node, selected: vm.selectedNodeID == node.id)
                            .contentShape(Rectangle())
                            .onTapGesture { vm.select(node) }
                            .swipeActions(edge: .trailing) {
                                Button("测速") { Task { await vm.test(node) } }
                                    .tint(.blue)
                                Button("删除", role: .destructive) {
                                    Task { await vm.delete(node) }
                                }
                            }
                    }
                }

                if let err = vm.errorMessage {
                    Section { Text(err).font(.caption).foregroundColor(.red) }
                }
            }
            .navigationTitle("BigCat V")
            .toolbar {
                ToolbarItem(placement: .navigationBarLeading) {
                    Button(vm.sortByLatency ? "按延迟 ✓" : "按延迟") {
                        vm.sortByLatency.toggle()
                    }
                }
                ToolbarItem(placement: .navigationBarTrailing) {
                    HStack {
                        Button("导入") { showingImport = true }
                        Button("测速全部") { Task { await vm.testAll() } }
                            .disabled(vm.isTesting)
                    }
                }
            }
            .sheet(isPresented: $showingImport) {
                NavigationStack {
                    VStack(alignment: .leading) {
                        Text("每行一条分享链接（ss/vmess/vless/trojan/hy2/tuic/wireguard/socks/http）")
                            .font(.caption).foregroundColor(.secondary)
                        TextEditor(text: $vm.linkText)
                            .border(Color.secondary.opacity(0.3))
                        Spacer()
                    }
                    .padding()
                    .navigationTitle("导入节点")
                    .toolbar {
                        ToolbarItem(placement: .confirmationAction) {
                            Button("确定") {
                                Task { await vm.addFromLinks() }
                                showingImport = false
                            }
                        }
                        ToolbarItem(placement: .cancellationAction) {
                            Button("取消") { showingImport = false }
                        }
                    }
                }
            }
            .refreshable { await vm.load() }
            .task { await vm.load() }
        }
    }
}

/// 单行节点：选中勾选、协议徽标、延迟、xray 不兼容提示。
struct NodeRow: View {
    let node: ProxyNode
    let selected: Bool

    var body: some View {
        HStack {
            Image(systemName: selected ? "checkmark.circle.fill" : "circle")
                .foregroundColor(selected ? .green : .secondary)
            VStack(alignment: .leading, spacing: 2) {
                Text(node.displayName).font(.body).lineLimit(1)
                HStack(spacing: 6) {
                    Text(node.proto.uppercased())
                        .font(.caption2).padding(2)
                        .background(Color.blue.opacity(0.15)).cornerRadius(4)
                    Text("\(node.server):\(node.port)")
                        .font(.caption).foregroundColor(.secondary)
                    if !node.xrayCompatible, let note = node.xrayNote {
                        Text(note).font(.caption2).foregroundColor(.orange)
                    }
                }
            }
            Spacer()
            Text(node.latencyText)
                .font(.caption).monospacedDigit()
                .foregroundColor(latencyColor)
        }
    }

    private var latencyColor: Color {
        guard let ms = node.latencyMs, ms >= 0 else { return .secondary }
        if ms < 300 { return .green }
        if ms < 800 { return .orange }
        return .red
    }
}
