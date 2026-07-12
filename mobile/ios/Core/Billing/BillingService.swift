import Foundation

struct BillingPackageOption: Sendable, Hashable, Identifiable {
    let id: String
    let planCode: PlanCode
    let interval: BillingInterval
    let localizedPrice: String
}

enum BillingServiceError: LocalizedError, Equatable {
    case notConfigured
    case notAuthenticated
    case offeringUnavailable
    case packageUnavailable

    var errorDescription: String? {
        switch self {
        case .notConfigured: "支付尚未配置"
        case .notAuthenticated: "登录完成后才能购买套餐"
        case .offeringUnavailable: "当前没有可购买的套餐"
        case .packageUnavailable: "所选套餐暂不可购买"
        }
    }
}

enum BillingPurchaseResult: Sendable, Equatable { case purchased, cancelled }

@MainActor
protocol BillingService: AnyObject {
    var isConfigured: Bool { get }
    func identify(userID: UUID) async throws
    func clearIdentity() async
    func fetchPackages() async throws -> [BillingPackageOption]
    func purchase(packageID: String) async throws -> BillingPurchaseResult
    func restorePurchases() async throws
}
