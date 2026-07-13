import SwiftUI

struct DashboardView: View {
    @Environment(SessionStore.self) private var session
    @State private var model: DashboardViewModel?
    @State private var showsFilterSheet = false

    var body: some View {
        NavigationStack {
            Group {
                if let model {
                    DashboardContent(model: model, showsFilterSheet: $showsFilterSheet)
                } else {
                    ProgressView()
                }
            }
            .navigationTitle(Text(String(localized: "dashboard.title", defaultValue: "商机")))
            .navigationDestination(for: Opportunity.self) { opportunity in
                OpportunityDetailView(opportunityID: opportunity.id)
            }
        }
        .task {
            if model == nil {
                let vm = DashboardViewModel(api: session.api)
                // 用用户时区计算"今天"边界（失败或未设置时用设备时区）。
                if let bundle = try? await session.api.settings() {
                    vm.query.timezoneIdentifier = bundle.workSchedule.timezone
                }
                model = vm
                await vm.refresh()
            }
        }
    }
}

private struct DashboardContent: View {
    @Bindable var model: DashboardViewModel
    @Binding var showsFilterSheet: Bool

    // iPad / 大屏自适应双列；手机单列。
    private let columns = [GridItem(.adaptive(minimum: 320), spacing: 12)]

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 12) {
                header
                if model.usesLegacyAPI { legacyAPINotice }
                if !model.attentionItems.isEmpty { attentionBanner }
                primaryFilters
                resultSummary
                content
            }
            .padding(.horizontal)
            .padding(.bottom, 24)
        }
        .refreshable { await model.refresh() }
        .sheet(isPresented: $showsFilterSheet) {
            DashboardFilterSheet(query: $model.query, keywordOptions: model.keywordOptions)
        }
        .overlay { if model.isInitialLoading { skeleton } }
    }

    // MARK: 头部

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: 2) {
                Text(String(localized: "dashboard.subtitle", defaultValue: "自动识别 Telegram 与企业微信中的潜在商机"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text(String(localized: "dashboard.pending", defaultValue: "\(model.pendingCount) 条待处理"))
                    .font(.headline)
            }
            Spacer()
            Button {
                Task { await model.refresh() }
            } label: {
                Image(systemName: "arrow.clockwise")
            }
            .accessibilityLabel(Text("action.refresh", bundle: .main))
        }
        .padding(.top, 8)
    }

    // MARK: 兼容提示

    private var legacyAPINotice: some View {
        Label(
            String(localized: "dashboard.legacy_notice", defaultValue: "服务端正在升级，当前显示基础商机列表；高级筛选暂不可用。"),
            systemImage: "server.rack"
        )
        .font(.caption)
        .foregroundStyle(.secondary)
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(10)
        .background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 10))
    }

    // MARK: 重大商机提醒

    private var attentionBanner: some View {
        AppCard {
            VStack(alignment: .leading, spacing: 8) {
                Label(
                    String(localized: "dashboard.attention_title", defaultValue: "pi Agent 发现 \(model.attentionItems.count) 条重大商机"),
                    systemImage: "exclamationmark.triangle.fill"
                )
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(AppColors.warning)
                Text(String(localized: "dashboard.attention_body", defaultValue: "请优先核对链接结论和后续行动建议，外部动作仍需人工批准。"))
                    .font(.caption)
                    .foregroundStyle(.secondary)
                ForEach(model.attentionItems.prefix(3)) { item in
                    NavigationLink(value: item) {
                        HStack {
                            Text(item.contactName).font(.subheadline)
                            Spacer()
                            Image(systemName: "chevron.right").font(.caption).foregroundStyle(.tertiary)
                        }
                    }
                    .buttonStyle(.plain)
                }
                if model.attentionItems.count > 3 {
                    Text(String(localized: "dashboard.attention_more", defaultValue: "查看全部"))
                        .font(.caption)
                        .foregroundStyle(AppColors.primary)
                }
            }
        }
        .overlay(RoundedRectangle(cornerRadius: 12).stroke(AppColors.warning.opacity(0.4)))
    }

    // MARK: 一级筛选

    private var primaryFilters: some View {
        VStack(spacing: 8) {
            Picker(String(localized: "filter.status", defaultValue: "状态"), selection: $model.query.status) {
                Text(String(localized: "status.all", defaultValue: "全部")).tag(FrontendOpportunityStatus?.none)
                ForEach([FrontendOpportunityStatus.pending, .replied, .ignored], id: \.self) {
                    Text($0.label).tag(FrontendOpportunityStatus?.some($0))
                }
            }
            .pickerStyle(.segmented)

            HStack {
                Menu {
                    Button(String(localized: "platform.all", defaultValue: "全部平台")) { model.query.platform = nil }
                    ForEach([IMChannel.telegram, .wecom], id: \.self) { channel in
                        Button(channel.label) { model.query.platform = channel }
                    }
                } label: {
                    filterChip(
                        title: model.query.platform?.label ?? String(localized: "platform.all", defaultValue: "全部平台"),
                        systemImage: "line.3.horizontal.decrease.circle"
                    )
                }

                Menu {
                    ForEach(DashboardQuery.Sort.allCases) { sort in
                        Button(sort.label) { model.query.sort = sort }
                    }
                } label: {
                    filterChip(title: model.query.sort.label, systemImage: "arrow.up.arrow.down")
                }
                .disabled(model.usesLegacyAPI)

                Spacer()

                Button {
                    showsFilterSheet = true
                } label: {
                    HStack(spacing: 4) {
                        Image(systemName: "slider.horizontal.3")
                        if model.query.activeAdvancedCount > 0 {
                            Text("\(model.query.activeAdvancedCount)")
                                .font(.caption2.bold())
                                .padding(4)
                                .background(AppColors.primary, in: Circle())
                                .foregroundStyle(.white)
                        }
                    }
                }
                .accessibilityLabel(Text("filter.advanced", bundle: .main))
                .disabled(model.usesLegacyAPI)
            }
        }
    }

    private func filterChip(title: String, systemImage: String) -> some View {
        HStack(spacing: 4) {
            Image(systemName: systemImage)
            Text(title).font(.subheadline)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(Color(.secondarySystemFill), in: Capsule())
    }

    // MARK: 结果摘要

    private var resultSummary: some View {
        HStack {
            Text(model.totalLabel).font(.caption).foregroundStyle(.secondary)
            Spacer()
            if model.query.activeAdvancedCount > 0 || model.query.status != nil || model.query.platform != nil {
                Button(String(localized: "action.clear_filters", defaultValue: "清空筛选")) {
                    model.query = DashboardQuery(timezoneIdentifier: model.query.timezoneIdentifier)
                }
                .font(.caption)
            }
        }
    }

    // MARK: 列表主体与状态

    @ViewBuilder private var content: some View {
        if let error = model.initialError, model.items.isEmpty {
            ContentUnavailableView {
                Label(String(localized: "state.load_failed", defaultValue: "加载失败"), systemImage: "wifi.exclamationmark")
            } description: {
                Text(error)
            } actions: {
                Button(String(localized: "action.retry", defaultValue: "重试")) { Task { await model.retryInitial() } }
            }
            .frame(maxWidth: .infinity, minHeight: 240)
        } else if model.items.isEmpty && !model.isInitialLoading {
            ContentUnavailableView(
                String(localized: "dashboard.empty", defaultValue: "暂无匹配的商机"),
                systemImage: "tray"
            )
            .frame(maxWidth: .infinity, minHeight: 240)
        } else {
            if let pageError = model.pageError {
                Button {
                    Task { await model.refresh() }
                } label: {
                    Label(pageError, systemImage: "exclamationmark.triangle")
                        .font(.caption)
                        .foregroundStyle(AppColors.destructive)
                }
            }
            LazyVGrid(columns: columns, spacing: 12) {
                ForEach(model.items) { opportunity in
                    NavigationLink(value: opportunity) {
                        OpportunityCardView(opportunity: opportunity)
                    }
                    .buttonStyle(.plain)
                    .task { await model.loadMoreIfNeeded(current: opportunity) }
                }
            }
            if model.isLoadingMore {
                HStack { Spacer(); ProgressView(); Spacer() }.padding(.vertical, 8)
            }
        }
    }

    private var skeleton: some View {
        VStack(spacing: 12) {
            ForEach(0..<3, id: \.self) { _ in
                RoundedRectangle(cornerRadius: 12)
                    .fill(Color(.secondarySystemFill))
                    .frame(height: 120)
                    .redacted(reason: .placeholder)
            }
            Spacer()
        }
        .padding(.horizontal)
        .padding(.top, 160)
        .allowsHitTesting(false)
    }
}
