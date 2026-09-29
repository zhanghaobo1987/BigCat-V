import Foundation

/// 订阅管理页的数据层。
@MainActor
final class SubscriptionViewModel: ObservableObject {
    @Published var subscriptions: [Subscription] = []
    @Published var isLoading = false
    @Published var errorMessage: String?
    @Published var newName = ""
    @Published var newURL = ""
    @Published var showingAdd = false

    private var client: ApiClient { ApiClient(baseURL: SharedConfig.apiBaseURLValue) }
    /// 订阅变更后通知节点列表刷新
    var onChanged: (() -> Void)?

    func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            subscriptions = try await client.listSubscriptions()
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func add() async {
        let url = newURL.trimmingCharacters(in: .whitespaces)
        guard !url.isEmpty else { return }
        isLoading = true
        defer { isLoading = false }
        do {
            let name = newName.trimmingCharacters(in: .whitespaces)
            _ = try await client.addSubscription(name: name.isEmpty ? nil : name, url: url)
            newName = ""
            newURL = ""
            showingAdd = false
            await load()
            onChanged?()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func refresh(_ sub: Subscription) async {
        do {
            _ = try await client.refreshSubscription(id: sub.id)
            await load()
            onChanged?()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
