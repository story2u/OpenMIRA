import Foundation
import RevenueCat

@MainActor
final class RevenueCatBillingService: BillingService {
    private let configuration: BillingConfiguration
    private var currentUserID: UUID?
    private var packages: [String: RevenueCat.Package] = [:]

    var isConfigured: Bool { configuration.publicAPIKey != nil }

    init(configuration: BillingConfiguration = .current) {
        self.configuration = configuration
    }

    func identify(userID: UUID) async throws {
        guard let key = configuration.publicAPIKey else { throw BillingServiceError.notConfigured }
        guard currentUserID != userID else { return }
        let userIDString = userID.uuidString.lowercased()
        if Purchases.isConfigured {
            _ = try await Purchases.shared.logIn(userIDString)
        } else {
            Purchases.configure(withAPIKey: key, appUserID: userIDString)
        }
        currentUserID = userID
        packages.removeAll()
    }

    func clearIdentity() async {
        packages.removeAll()
        currentUserID = nil
        guard Purchases.isConfigured, !Purchases.shared.isAnonymous else { return }
        _ = try? await Purchases.shared.logOut()
    }

    func fetchPackages() async throws -> [BillingPackageOption] {
        guard currentUserID != nil else { throw BillingServiceError.notAuthenticated }
        let offerings = try await Purchases.shared.offerings()
        guard let offering = offerings.all[configuration.offeringID] else {
            throw BillingServiceError.offeringUnavailable
        }
        let supported = offering.availablePackages.compactMap { package -> BillingPackageOption? in
            guard let identity = Self.packageIdentity(package.identifier) else { return nil }
            packages[package.identifier] = package
            return BillingPackageOption(
                id: package.identifier,
                planCode: identity.plan,
                interval: identity.interval,
                localizedPrice: package.localizedPriceString
            )
        }
        return supported
    }

    func purchase(packageID: String) async throws -> BillingPurchaseResult {
        guard currentUserID != nil else { throw BillingServiceError.notAuthenticated }
        guard let package = packages[packageID] else { throw BillingServiceError.packageUnavailable }
        let result = try await Purchases.shared.purchase(package: package)
        return result.userCancelled ? .cancelled : .purchased
    }

    func restorePurchases() async throws {
        guard currentUserID != nil else { throw BillingServiceError.notAuthenticated }
        _ = try await Purchases.shared.restorePurchases()
    }

    private static func packageIdentity(_ identifier: String) -> (plan: PlanCode, interval: BillingInterval)? {
        let values: [String: (PlanCode, BillingInterval)] = [
            "plus_monthly": (.plus, .monthly), "plus_annual": (.plus, .annual),
            "pro_monthly": (.pro, .monthly), "pro_annual": (.pro, .annual),
            "max_monthly": (.max, .monthly), "max_annual": (.max, .annual),
        ]
        return values[identifier]
    }
}
