import Foundation
import Observation

protocol SubscriptionAPI: AnyObject, Sendable {
    func subscription() async throws -> SubscriptionUsage
    func subscriptionCatalog() async throws -> [SubscriptionCatalogPlan]
    func syncSubscription() async throws -> SubscriptionUsage
    func subscriptionManagement() async throws -> SubscriptionManagement
}

extension APIClient: SubscriptionAPI {}

@MainActor
@Observable
final class SubscriptionViewModel {
    private let api: any SubscriptionAPI
    private let billing: BillingService
    private let userID: UUID

    var usage: SubscriptionUsage?
    var catalog: [SubscriptionCatalogPlan] = []
    var management: SubscriptionManagement?
    var packages: [BillingPackageOption] = []
    var selectedInterval: BillingInterval = .monthly
    var isLoading = false
    var busyPackageID: String?
    var isRestoring = false
    var message: String?
    var errorMessage: String?

    init(api: any SubscriptionAPI, billing: BillingService, userID: UUID) {
        self.api = api
        self.billing = billing
        self.userID = userID
    }

    var purchasesAllowed: Bool { usage?.planCode == .free && billing.isConfigured }
    var billingConfigured: Bool { billing.isConfigured }

    func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            async let usage = api.subscription()
            async let catalog = api.subscriptionCatalog()
            async let management = api.subscriptionManagement()
            self.usage = try await usage
            self.catalog = try await catalog
            self.management = try await management
            if billing.isConfigured {
                try await billing.identify(userID: userID)
                packages = try await billing.fetchPackages()
            }
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func package(for plan: PlanCode) -> BillingPackageOption? {
        packages.first { $0.planCode == plan && $0.interval == selectedInterval }
    }

    func purchase(_ option: BillingPackageOption) async {
        guard purchasesAllowed else { return }
        busyPackageID = option.id
        message = nil
        errorMessage = nil
        defer { busyPackageID = nil }
        do {
            if try await billing.purchase(packageID: option.id) == .cancelled {
                message = "已取消购买，未产生套餐变更。"
                return
            }
            message = "支付已完成，正在确认订阅权益。"
            usage = try await api.syncSubscription()
            management = try await api.subscriptionManagement()
            if usage?.planCode == option.planCode { message = "订阅权益已生效。" }
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func restore() async {
        guard billing.isConfigured else { errorMessage = BillingServiceError.notConfigured.localizedDescription; return }
        isRestoring = true
        defer { isRestoring = false }
        do {
            try await billing.identify(userID: userID)
            try await billing.restorePurchases()
            usage = try await api.syncSubscription()
            management = try await api.subscriptionManagement()
            message = "购买记录已同步。"
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
