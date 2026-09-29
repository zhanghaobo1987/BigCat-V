import SwiftUI

/// 订阅管理页：列表 + 添加 + 手动刷新。
struct SubscriptionView: View {
    @EnvironmentObject private var vm: SubscriptionViewModel

    var body: some View {
        NavigationStack {
            List {
                ForEach(vm.subscriptions) { sub in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(sub.name).font(.body).lineLimit(1)
                        Text(sub.url).font(.caption).foregroundColor(.secondary).lineLimit(1)
                        Text("\(sub.count) 个节点").font(.caption2).foregroundColor(.secondary)
                    }
                    .swipeActions(edge: .trailing) {
                        Button("刷新") { Task { await vm.refresh(sub) } }.tint(.blue)
                    }
                }
                if let err = vm.errorMessage {
                    Section { Text(err).font(.caption).foregroundColor(.red) }
                }
            }
            .navigationTitle("订阅")
            .toolbar {
                ToolbarItem(placement: .navigationBarTrailing) {
                    Button { vm.showingAdd = true } label: {
                        Image(systemName: "plus")
                    }
                }
            }
            .sheet(isPresented: $vm.showingAdd) {
                NavigationStack {
                    Form {
                        TextField("名称（可选）", text: $vm.newName)
                        TextField("订阅地址", text: $vm.newURL)
                            .keyboardType(.URL)
                            .autocapitalization(.none)
                    }
                    .navigationTitle("添加订阅")
                    .toolbar {
                        ToolbarItem(placement: .confirmationAction) {
                            Button("确定") { Task { await vm.add() } }
                        }
                        ToolbarItem(placement: .cancellationAction) {
                            Button("取消") { vm.showingAdd = false }
                        }
                    }
                }
            }
            .refreshable { await vm.load() }
            .task { await vm.load() }
        }
    }
}
