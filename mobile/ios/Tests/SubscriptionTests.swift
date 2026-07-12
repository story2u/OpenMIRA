import XCTest
@testable import OpportunityRadar

final class SubscriptionTests: XCTestCase {
    func testSubscriptionDTOUsesSeparateBillingAndUsagePeriods() throws {
        let json = """
        {"planCode":"pro","subscriptionStatus":"active","periodStart":"2026-07-01T00:00:00Z",
        "periodEnd":"2026-08-01T00:00:00Z","cancelAtPeriodEnd":false,
        "entitlements":{"planCode":"pro","telegramGroupLimit":null,"wecomGroupLimit":null,
        "combinedGroupLimit":20,"piAgentAnalysisMonthlyLimit":1000},"telegramGroupsUsed":2,
        "wecomGroupsUsed":0,"combinedGroupsUsed":2,"aiAnalysesConsumed":3,"aiAnalysesReserved":1,
        "aiAnalysesRemaining":996,"effectiveStore":"app_store","billingInterval":"annual",
        "billingPeriodStart":"2026-05-10T00:00:00Z","billingPeriodEnd":"2027-05-10T00:00:00Z",
        "usagePeriodStart":"2026-07-01T00:00:00Z","usagePeriodEnd":"2026-08-01T00:00:00Z",
        "entitlementExpiresAt":"2027-05-10T00:00:00Z","willRenew":true,"billingIssue":false,
        "multipleActiveSubscriptions":false,"managementUrl":"https://apps.apple.com/account/subscriptions",
        "lastSyncedAt":"2026-07-12T00:00:00Z"}
        """
        let usage = try APIClient.makeDecoder().decode(SubscriptionUsage.self, from: Data(json.utf8))
        XCTAssertEqual(usage.planCode, .pro)
        XCTAssertEqual(usage.billingInterval, .annual)
        XCTAssertNotEqual(usage.billingPeriodEnd, usage.usagePeriodEnd)
    }

    @MainActor
    func testPurchaseSuccessSynchronizesBackend() async {
        let billing = FakeBillingService(result: .purchased)
        let api = FakeSubscriptionAPI()
        let model = SubscriptionViewModel(api: api, billing: billing, userID: UUID())
        await model.load()
        let option = try! XCTUnwrap(model.package(for: .plus))
        await model.purchase(option)
        XCTAssertEqual(api.syncCount, 1)
        XCTAssertEqual(model.usage?.planCode, .plus)
    }

    @MainActor
    func testPurchaseCancellationDoesNotSynchronizeBackend() async {
        let billing = FakeBillingService(result: .cancelled)
        let api = FakeSubscriptionAPI()
        let model = SubscriptionViewModel(api: api, billing: billing, userID: UUID())
        await model.load()
        await model.purchase(try! XCTUnwrap(model.package(for: .plus)))
        XCTAssertEqual(api.syncCount, 0)
        XCTAssertEqual(model.message, "已取消购买，未产生套餐变更。")
    }

    @MainActor
    func testRestoreSynchronizesBackendAndIdentity() async {
        let billing = FakeBillingService(result: .purchased)
        let api = FakeSubscriptionAPI()
        let userID = UUID()
        let model = SubscriptionViewModel(api: api, billing: billing, userID: userID)
        await model.restore()
        XCTAssertEqual(billing.identifiedUserID, userID)
        XCTAssertEqual(billing.restoreCount, 1)
        XCTAssertEqual(api.syncCount, 1)
    }

    @MainActor
    func testUnconfiguredBillingCannotPurchaseAnonymously() async {
        let billing = FakeBillingService(result: .purchased, configured: false)
        let api = FakeSubscriptionAPI()
        let model = SubscriptionViewModel(api: api, billing: billing, userID: UUID())
        await model.load()
        await model.purchase(BillingPackageOption(id: "plus_monthly", planCode: .plus, interval: .monthly, localizedPrice: "$1"))
        XCTAssertEqual(billing.purchaseCount, 0)
        XCTAssertEqual(api.syncCount, 0)
    }
}

@MainActor
private final class FakeBillingService: BillingService {
    let isConfigured: Bool
    var identifiedUserID: UUID?
    var restoreCount = 0
    var purchaseCount = 0
    let result: BillingPurchaseResult

    init(result: BillingPurchaseResult, configured: Bool = true) {
        self.result = result
        self.isConfigured = configured
    }
    func identify(userID: UUID) async throws { identifiedUserID = userID }
    func clearIdentity() async { identifiedUserID = nil }
    func fetchPackages() async throws -> [BillingPackageOption] {
        [BillingPackageOption(id: "plus_monthly", planCode: .plus, interval: .monthly, localizedPrice: "$1")]
    }
    func purchase(packageID: String) async throws -> BillingPurchaseResult { purchaseCount += 1; return result }
    func restorePurchases() async throws { restoreCount += 1 }
}

private final class FakeSubscriptionAPI: SubscriptionAPI, @unchecked Sendable {
    var syncCount = 0
    func subscription() async throws -> SubscriptionUsage { Self.usage(.free) }
    func subscriptionCatalog() async throws -> [SubscriptionCatalogPlan] {
        [SubscriptionCatalogPlan(planCode: .plus, displayName: "Plus", rank: 1,
          entitlements: Self.entitlements(.plus), availableIntervals: [.monthly, .annual],
          revenuecatPackageIdentifiers: ["plus_monthly", "plus_annual"])]
    }
    func syncSubscription() async throws -> SubscriptionUsage { syncCount += 1; return Self.usage(.plus) }
    func subscriptionManagement() async throws -> SubscriptionManagement {
        SubscriptionManagement(store: nil, managementUrl: nil, instruction: "", canOpenInCurrentClient: false)
    }
    static func entitlements(_ plan: PlanCode) -> PlanEntitlements {
        PlanEntitlements(planCode: plan, telegramGroupLimit: 1, wecomGroupLimit: 1,
                         combinedGroupLimit: 2, piAgentAnalysisMonthlyLimit: 10)
    }
    static func usage(_ plan: PlanCode) -> SubscriptionUsage {
        let now = Date()
        return SubscriptionUsage(planCode: plan, subscriptionStatus: plan == .free ? .inactive : .active,
          periodStart: now, periodEnd: now, cancelAtPeriodEnd: false, entitlements: entitlements(plan),
          telegramGroupsUsed: 0, wecomGroupsUsed: 0, combinedGroupsUsed: 0, aiAnalysesConsumed: 0,
          aiAnalysesReserved: 0, aiAnalysesRemaining: 10, effectiveStore: nil, billingInterval: nil,
          billingPeriodStart: nil, billingPeriodEnd: nil, usagePeriodStart: now, usagePeriodEnd: now,
          entitlementExpiresAt: nil, willRenew: false, billingIssue: false,
          multipleActiveSubscriptions: false, managementUrl: nil, lastSyncedAt: nil)
    }
}
