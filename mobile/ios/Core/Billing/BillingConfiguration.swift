import Foundation

struct BillingConfiguration: Sendable {
    let publicAPIKey: String?
    let offeringID: String

    static let current = BillingConfiguration(
        publicAPIKey: AppConfig.revenueCatIOSPublicAPIKey,
        offeringID: "default"
    )
}
