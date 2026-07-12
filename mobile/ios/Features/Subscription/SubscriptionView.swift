import SwiftUI

struct SubscriptionView: View {
    @Environment(\.openURL) private var openURL
    @State private var model: SubscriptionViewModel

    init(api: APIClient, billing: BillingService, userID: UUID) {
        _model = State(initialValue: SubscriptionViewModel(api: api, billing: billing, userID: userID))
    }

    var body: some View {
        List {
            if let usage = model.usage {
                currentSection(usage)
                notices(usage)
                usageSection(usage)
                plansSection(usage)
            } else if model.isLoading {
                HStack { Spacer(); ProgressView(); Spacer() }
            }
            if let message = model.message { Text(message).foregroundStyle(.secondary) }
            if let error = model.errorMessage { Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(.orange) }
        }
        .navigationTitle("套餐与用量")
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button("恢复购买") { Task { await model.restore() } }
                    .disabled(model.isRestoring || !model.billingConfigured)
            }
        }
        .task { if model.usage == nil { await model.load() } }
        .refreshable { await model.load() }
    }

    private func currentSection(_ usage: SubscriptionUsage) -> some View {
        Section("当前套餐") {
            LabeledContent("套餐", value: usage.planCode.label)
            if let store = usage.effectiveStore { LabeledContent("购买渠道", value: store.label) }
            if usage.cancelAtPeriodEnd { Label("续费已取消，当前周期结束前权益保持有效", systemImage: "calendar.badge.exclamationmark") }
            if let management = model.management {
                Button("管理订阅") { if management.canOpenInCurrentClient, let url = management.managementUrl { openURL(url) } }
                    .disabled(!management.canOpenInCurrentClient || management.managementUrl == nil)
                if !management.canOpenInCurrentClient { Text(management.instruction).font(.footnote).foregroundStyle(.secondary) }
            }
        }
    }

    @ViewBuilder private func notices(_ usage: SubscriptionUsage) -> some View {
        if usage.multipleActiveSubscriptions {
            Section { Label("检测到多个渠道的有效订阅，你可能正在重复付费。请前往原购买渠道管理。", systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange) }
        }
        if usage.billingIssue {
            Section { Label("当前订阅存在付款问题，请前往原购买渠道处理。", systemImage: "creditcard.trianglebadge.exclamationmark").foregroundStyle(.red) }
        }
    }

    private func usageSection(_ usage: SubscriptionUsage) -> some View {
        Section("本月用量") {
            LabeledContent("AI 分析", value: "\(usage.aiAnalysesConsumed + usage.aiAnalysesReserved) / \(usage.entitlements.piAgentAnalysisMonthlyLimit)")
            LabeledContent("群监控", value: "\(usage.combinedGroupsUsed) / \(usage.entitlements.combinedGroupLimit)")
        }
    }

    private func plansSection(_ usage: SubscriptionUsage) -> some View {
        Section("套餐") {
            Picker("计费周期", selection: $model.selectedInterval) {
                Text("月付").tag(BillingInterval.monthly)
                Text("年付").tag(BillingInterval.annual)
            }.pickerStyle(.segmented)
            if !model.billingConfigured { Text("支付尚未配置").foregroundStyle(.secondary) }
            ForEach(model.catalog) { plan in
                let option = model.package(for: plan.planCode)
                VStack(alignment: .leading, spacing: 8) {
                    HStack { Text(plan.displayName).font(.headline); Spacer(); Text(plan.planCode == .free ? "免费" : option?.localizedPrice ?? "价格暂不可用") }
                    Text("每月 \(plan.entitlements.piAgentAnalysisMonthlyLimit) 次 AI 分析 · \(plan.entitlements.combinedGroupLimit) 个群").font(.footnote).foregroundStyle(.secondary)
                    if plan.planCode != .free && plan.planCode != usage.planCode {
                        Button(model.busyPackageID == option?.id ? "处理中…" : "购买") { if let option { Task { await model.purchase(option) } } }
                            .disabled(!model.purchasesAllowed || option == nil || model.busyPackageID != nil)
                    }
                }.padding(.vertical, 4)
            }
            if usage.planCode != .free { Text("已有有效订阅。请先在原购买渠道管理，避免重复付费。") }
        }
    }
}

private extension PlanCode {
    var label: String { switch self { case .free: "Free"; case .plus: "Plus"; case .pro: "Pro"; case .max: "Max"; case .unknown: "未知" } }
}

private extension BillingStore {
    var label: String { switch self { case .appStore: "Apple App Store"; case .playStore: "Google Play"; case .paddle: "Web / Paddle"; case .testStore: "测试商店"; case .unknown: "未知渠道" } }
}
