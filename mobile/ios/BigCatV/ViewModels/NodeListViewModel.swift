import Foundation

/// 节点列表页的数据层：节点 CRUD、延迟测速、当前选中节点。
@MainActor
final class NodeListViewModel: ObservableObject {
    @Published var nodes: [ProxyNode] = []
    @Published var isLoading = false
    @Published var isTesting = false
    @Published var errorMessage: String?
    @Published var linkText = "" // 批量导入文本框（每行一条分享链接）
    @Published var sortByLatency = false
    @Published var selectedNodeID: String? = SharedConfig.selectedNodeID

    private var client: ApiClient { ApiClient(baseURL: SharedConfig.apiBaseURLValue) }

    /// 按延迟排序展示（未测排最后）
    var displayedNodes: [ProxyNode] {
        sortByLatency ? nodes.sorted { $0.sortLatency < $1.sortLatency } : nodes
    }

    func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            nodes = try await client.listNodes()
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    /// 批量导入：按行切分，逐条 POST /api/v1/nodes。
    func addFromLinks() async {
        let links = linkText
            .components(separatedBy: .newlines)
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .filter { !$0.isEmpty }
        guard !links.isEmpty else { return }
        isLoading = true
        defer { isLoading = false }
        var failed = 0
        for link in links {
            do { _ = try await client.addNode(link: link) }
            catch { failed += 1 }
        }
        linkText = ""
        await load()
        if failed > 0 { errorMessage = "\(failed) 条链接导入失败" }
    }

    func delete(_ node: ProxyNode) async {
        do {
            try await client.deleteNode(id: node.id)
            nodes.removeAll { $0.id == node.id }
            if selectedNodeID == node.id { select(nil) }
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    /// 单节点测速，回写列表中的延迟。
    func test(_ node: ProxyNode) async {
        do {
            let ms = try await client.testNode(id: node.id)
            if let i = nodes.firstIndex(where: { $0.id == node.id }) {
                nodes[i].latencyMs = ms
            }
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    /// 全部测速：并发执行，逐个回写。
    func testAll() async {
        guard !isTesting else { return }
        isTesting = true
        defer { isTesting = false }
        await withTaskGroup(of: (String, Int64?).self) { group in
            for n in nodes {
                group.addTask { [client] in
                    let ms = try? await client.testNode(id: n.id)
                    return (n.id, ms)
                }
            }
            for await (id, ms) in group {
                if let i = self.nodes.firstIndex(where: { $0.id == id }), let ms {
                    self.nodes[i].latencyMs = ms
                }
            }
        }
    }

    /// 选中节点：写入 App Group，Tunnel 启动引擎时读取。
    func select(_ node: ProxyNode?) {
        selectedNodeID = node?.id
        SharedConfig.selectedNodeID = node?.id
    }
}
