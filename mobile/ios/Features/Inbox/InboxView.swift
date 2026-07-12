import Observation
import SwiftUI

@MainActor
@Observable
final class InboxModel {
    private let api: APIClient
    private let pageSize = 50

    var items: [Opportunity] = []
    var statusFilter: FrontendOpportunityStatus?
    var platformFilter: IMChannel?
    var isLoading = false
    var canLoadMore = false
    var errorMessage: String?

    init(api: APIClient) {
        self.api = api
    }

    func refresh() async {
        await load(reset: true)
    }

    func loadMoreIfNeeded(current item: Opportunity) async {
        guard canLoadMore, !isLoading, item.id == items.last?.id else { return }
        await load(reset: false)
    }

    private func load(reset: Bool) async {
        isLoading = true
        defer { isLoading = false }
        do {
            let offset = reset ? 0 : items.count
            let page = try await api.opportunities(
                status: statusFilter,
                platform: platformFilter,
                limit: pageSize,
                offset: offset
            )
            items = reset ? page : items + page
            canLoadMore = page.count == pageSize
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

struct InboxView: View {
    @Environment(SessionStore.self) private var session
    @State private var model: InboxModel?

    var body: some View {
        NavigationStack {
            Group {
                if let model {
                    InboxListView(model: model)
                } else {
                    ProgressView()
                }
            }
            .navigationTitle("商机收件箱")
            .navigationDestination(for: Opportunity.self) { opportunity in
                OpportunityDetailView(opportunityID: opportunity.id)
            }
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        if let user = session.currentUser {
                            NavigationLink {
                                SubscriptionView(api: session.api, billing: session.billing, userID: user.id)
                            } label: {
                                Label("套餐与用量", systemImage: "creditcard")
                            }
                        }
                        Button("退出登录", role: .destructive) { session.logout() }
                    } label: {
                        Label(session.currentUser?.displayName ?? "我", systemImage: "person.circle")
                    }
                }
            }
        }
        .task {
            if model == nil {
                model = InboxModel(api: session.api)
            }
        }
    }
}

private struct InboxListView: View {
    @Environment(SessionStore.self) private var session
    @Bindable var model: InboxModel

    var body: some View {
        List {
            if let error = model.errorMessage {
                Label(error, systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.orange)
                    .onTapGesture { Task { await model.refresh() } }
            }
            ForEach(model.items) { opportunity in
                NavigationLink(value: opportunity) {
                    InboxRow(opportunity: opportunity)
                }
                .task { await model.loadMoreIfNeeded(current: opportunity) }
            }
            if model.isLoading {
                HStack { Spacer(); ProgressView(); Spacer() }
            } else if model.items.isEmpty && model.errorMessage == nil {
                ContentUnavailableView("暂无商机", systemImage: "tray")
            }
        }
        .refreshable { await model.refresh() }
        .safeAreaInset(edge: .top) { filterBar }
        .task(id: "\(model.statusFilter?.rawValue ?? "all")-\(model.platformFilter?.rawValue ?? "all")") {
            // 筛选变化即重载；同一 id 下首次进入触发轮询。
            await model.refresh()
            while !Task.isCancelled {
                // ponytail: P0 用 30s 轮询对齐 Web 行为，推送通道（计划步骤 8-9）落地后改为推送触发刷新。
                try? await Task.sleep(for: .seconds(30))
                guard !Task.isCancelled else { break }
                await model.refresh()
            }
        }
    }

    private var filterBar: some View {
        HStack(spacing: 12) {
            Picker("状态", selection: $model.statusFilter) {
                Text("全部").tag(FrontendOpportunityStatus?.none)
                ForEach([FrontendOpportunityStatus.pending, .replied, .ignored], id: \.self) {
                    Text($0.label).tag(FrontendOpportunityStatus?.some($0))
                }
            }
            .pickerStyle(.segmented)

            Menu {
                Button("全部渠道") { model.platformFilter = nil }
                ForEach([IMChannel.telegram, .wecom], id: \.self) { channel in
                    Button(channel.label) { model.platformFilter = channel }
                }
            } label: {
                Image(systemName: "line.3.horizontal.decrease.circle")
                    .symbolVariant(model.platformFilter == nil ? .none : .fill)
            }
        }
        .padding(.horizontal)
        .padding(.vertical, 8)
        .background(.bar)
    }
}

private struct InboxRow: View {
    let opportunity: Opportunity

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                if opportunity.attentionRequired {
                    Image(systemName: "exclamationmark.circle.fill")
                        .foregroundStyle(.red)
                }
                Text(opportunity.contactName)
                    .font(.headline)
                Spacer()
                Text(opportunity.updatedAt, format: .relative(presentation: .named))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Text(opportunity.summary)
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .lineLimit(2)
            HStack(spacing: 8) {
                Badge(text: opportunity.platform.label, color: .blue)
                Badge(text: opportunity.internalStatus.label, color: .gray)
                if opportunity.priority == .high || opportunity.priority == .urgent {
                    Badge(text: opportunity.priority.label, color: .red)
                }
                if let group = opportunity.groupName {
                    Text(group)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
        }
        .padding(.vertical, 2)
    }
}

struct Badge: View {
    let text: String
    let color: Color

    var body: some View {
        Text(text)
            .font(.caption2.weight(.medium))
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(color.opacity(0.15), in: Capsule())
            .foregroundStyle(color)
    }
}
