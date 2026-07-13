import Observation
import SwiftUI

@MainActor
@Observable
final class DashboardViewModel {
    private let api: APIClient
    private let pageSize = 20

    /// 页面状态：区分首屏加载 / 刷新 / 加载更多，保证错误时不清空旧数据。
    enum LoadPhase: Equatable {
        case idle
        case initialLoading
        case refreshing
        case loadingMore
    }

    var items: [Opportunity] = []
    var total = 0
    var pendingCount = 0
    var attentionItems: [Opportunity] = []
    var keywordOptions: [String] = []
    var usesLegacyAPI = false

    var query = DashboardQuery() {
        didSet { if query != oldValue { queryDidChange() } }
    }

    var phase: LoadPhase = .idle
    var initialError: String?
    var pageError: String?

    private var offset = 0
    private var canLoadMore = false
    private var loadGeneration = 0

    init(api: APIClient) {
        self.api = api
    }

    var isInitialLoading: Bool { phase == .initialLoading }
    var isRefreshing: Bool { phase == .refreshing }
    var isLoadingMore: Bool { phase == .loadingMore }

    /// 当前筛选下总数（来自服务端，非当前已加载条数）。
    var totalLabel: String {
        if usesLegacyAPI && canLoadMore {
            return String(localized: "dashboard.loaded_count", defaultValue: "已加载 \(items.count) 条商机")
        }
        return String(localized: "dashboard.result_count", defaultValue: "共 \(total) 条商机")
    }

    private func queryDidChange() {
        Task { await refresh() }
    }

    func refresh() async {
        loadGeneration += 1
        let generation = loadGeneration
        offset = 0
        phase = items.isEmpty ? .initialLoading : .refreshing
        await fetchPage(reset: true, generation: generation)
    }

    func loadMoreIfNeeded(current item: Opportunity) async {
        guard canLoadMore, phase == .idle, item.id == items.last?.id else { return }
        loadGeneration += 1
        let generation = loadGeneration
        phase = .loadingMore
        await fetchPage(reset: false, generation: generation)
    }

    func retryInitial() async {
        initialError = nil
        await refresh()
    }

    private func fetchPage(reset: Bool, generation: Int) async {
        let pagedQuery = query
        do {
            let page = try await api.dashboard(
                pagedQueryItems(base: pagedQuery, offset: reset ? 0 : offset)
            )
            let response = page.response
            // 慢响应竞态防护：仅接受最新一次请求的结果。
            guard generation == loadGeneration else { return }
            if reset {
                items = response.items
            } else {
                items += response.items
            }
            usesLegacyAPI = page.usesLegacyAPI
            if page.usesLegacyAPI {
                // 旧接口没有聚合字段；只根据当前已加载数据展示，不伪造服务端总数。
                total = items.count
                pendingCount = items.filter { $0.status == .pending }.count
                attentionItems = items.filter(\.attentionRequired)
                keywordOptions = Array(Set(items.flatMap(\.matchedKeywords))).sorted()
            } else {
                total = response.total
                pendingCount = response.pendingCount
                attentionItems = response.attentionItems
                keywordOptions = response.keywordOptions
            }
            offset = items.count
            canLoadMore = page.hasMore && !response.items.isEmpty
            initialError = nil
            pageError = nil
        } catch is CancellationError {
            return
        } catch {
            guard generation == loadGeneration else { return }
            // 已有数据时刷新失败 → 保留旧数据，仅提示；首屏失败才走空态。
            if items.isEmpty {
                initialError = error.localizedDescription
            } else {
                pageError = error.localizedDescription
            }
        }
        if generation == loadGeneration { phase = .idle }
    }

    /// 组合分页 offset 到查询（query 本身不含分页）。
    private func pagedQueryItems(base: DashboardQuery, offset: Int) -> DashboardQueryPaged {
        DashboardQueryPaged(query: base, limit: pageSize, offset: offset)
    }
}

/// 把 DashboardQuery 与分页参数合并成一次请求的 query items。
struct DashboardQueryPaged: Sendable {
    let query: DashboardQuery
    let limit: Int
    let offset: Int

    var queryItems: [URLQueryItem] {
        query.queryItems + [
            URLQueryItem(name: "limit", value: String(limit)),
            URLQueryItem(name: "offset", value: String(offset)),
        ]
    }
}

extension APIClient {
    func dashboard(_ paged: DashboardQueryPaged) async throws -> DashboardPage {
        do {
            let response: DashboardResponse = try await get(
                "opportunities/dashboard",
                query: paged.queryItems
            )
            return DashboardPage(
                response: response,
                usesLegacyAPI: false,
                hasMore: response.offset + response.items.count < response.total
            )
        } catch APIError.server(let status, _) where [404, 422].contains(status)
            && paged.query.supportsLegacyEndpoint {
            // 生产后端滚动升级期间，新路径可能尚未注册：404，或被旧的
            // /opportunities/{opportunity_id} 动态路由当作 UUID 而返回 422。
            let items = try await opportunities(
                status: paged.query.status,
                platform: paged.query.platform,
                limit: paged.limit,
                offset: paged.offset
            )
            let response = DashboardResponse(
                items: items,
                total: paged.offset + items.count,
                limit: paged.limit,
                offset: paged.offset,
                pendingCount: items.filter { $0.status == .pending }.count,
                attentionItems: items.filter(\.attentionRequired),
                keywordOptions: Array(Set(items.flatMap(\.matchedKeywords))).sorted()
            )
            return DashboardPage(
                response: response,
                usesLegacyAPI: true,
                hasMore: items.count == paged.limit
            )
        }
    }
}

struct DashboardPage: Sendable {
    let response: DashboardResponse
    let usesLegacyAPI: Bool
    let hasMore: Bool
}
