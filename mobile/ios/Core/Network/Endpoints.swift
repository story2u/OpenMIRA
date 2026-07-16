import Foundation

/// 资源方法：路径与参数以 backend/app/api/v1/routes/ 为准。
extension APIClient {
    // MARK: Auth

    func me() async throws -> AuthUser {
        try await get("auth/me")
    }

    /// 原生登录：后端端点待实现（P0 计划步骤 2），落地前返回 404。
    func nativeLogin(provider: String, idToken: String) async throws -> AuthToken {
        try await post("auth/oauth/\(provider)/native", body: NativeLoginRequest(idToken: idToken))
    }

    func passwordLogin(email: String, password: String) async throws -> AuthToken {
        try await post(
            "auth/password/login",
            body: PasswordLoginRequest(email: email, password: password)
        )
    }

    func requestPasswordReset(email: String) async throws -> PasswordActionResponse {
        try await post("auth/password/reset/request", body: PasswordResetRequest(email: email))
    }

    func confirmPasswordReset(
        email: String,
        code: String,
        newPassword: String
    ) async throws -> PasswordActionResponse {
        try await post(
            "auth/password/reset/confirm",
            body: PasswordResetConfirmRequest(
                newPassword: newPassword,
                token: nil,
                email: email,
                code: code
            )
        )
    }

    func changePassword(currentPassword: String, newPassword: String) async throws -> PasswordActionResponse {
        try await post(
            "auth/password/change",
            body: PasswordChangeRequest(
                currentPassword: currentPassword,
                newPassword: newPassword
            )
        )
    }

    // MARK: Opportunities

    func opportunities(
        status: FrontendOpportunityStatus? = nil,
        platform: IMChannel? = nil,
        limit: Int = 50,
        offset: Int = 0
    ) async throws -> [Opportunity] {
        var query = [
            URLQueryItem(name: "limit", value: String(limit)),
            URLQueryItem(name: "offset", value: String(offset)),
        ]
        if let status { query.append(URLQueryItem(name: "status", value: status.rawValue)) }
        if let platform { query.append(URLQueryItem(name: "platform", value: platform.rawValue)) }
        return try await get("opportunities", query: query)
    }

    func opportunity(id: UUID) async throws -> Opportunity {
        try await get("opportunities/\(id.uuidString)")
    }

    func messages(opportunityID: UUID) async throws -> [ChatMessage] {
        try await get(
            "messages",
            query: [URLQueryItem(name: "opportunity_id", value: opportunityID.uuidString)]
        )
    }

    func sendManualReply(opportunityID: UUID, text: String, operatorID: String) async throws -> Opportunity {
        try await post(
            "opportunities/\(opportunityID.uuidString)/manual-reply",
            body: ManualReplyRequest(text: text, operatorId: operatorID)
        )
    }

    func generateAIDraft(opportunityID: UUID) async throws -> AIDraft {
        try await post("opportunities/\(opportunityID.uuidString)/ai-draft")
    }

    func updateStatus(opportunityID: UUID, to status: OpportunityStatus) async throws -> Opportunity {
        try await patch(
            "opportunities/\(opportunityID.uuidString)/status",
            body: OpportunityStatusUpdate(status: status)
        )
    }

    func claim(opportunityID: UUID, operatorID: String) async throws -> Opportunity {
        try await post(
            "opportunities/\(opportunityID.uuidString)/claim",
            query: [URLQueryItem(name: "operator_id", value: operatorID)]
        )
    }

    // MARK: Job discovery

    func jobs(query: [URLQueryItem] = []) async throws -> JobsPage {
        try await get("jobs", query: query)
    }

    func job(id: UUID, profileID: UUID? = nil) async throws -> JobOpportunityDetail {
        let query = profileID.map { [URLQueryItem(name: "profile_id", value: $0.uuidString)] } ?? []
        return try await get("jobs/\(id.uuidString)", query: query)
    }

    func jobSearchProfiles() async throws -> [JobSearchProfile] {
        try await get("job-search-profiles")
    }

    func createJobSearchProfile(_ body: JobSearchProfileWrite) async throws -> JobSearchProfile {
        try await post("job-search-profiles", body: body)
    }

    func updateJobSearchProfile(id: UUID, body: JobSearchProfileWrite) async throws -> JobSearchProfile {
        try await patch("job-search-profiles/\(id.uuidString)", body: body)
    }

    func deleteJobSearchProfile(id: UUID) async throws {
        try await delete("job-search-profiles/\(id.uuidString)")
    }

    func parseJobSearchProfile(_ text: String) async throws -> JobSearchProfilePreview {
        try await post("job-search-profiles/parse", body: JobProfileParseRequest(text: text))
    }

    func submitJobFeedback(id: UUID, type: JobFeedbackType) async throws -> JobFeedbackResponse {
        try await post(
            "jobs/\(id.uuidString)/feedback",
            body: JobFeedbackRequest(feedbackType: type, note: nil)
        )
    }

    // MARK: Templates

    func templates() async throws -> [ReplyTemplate] {
        try await get("templates")
    }

    // MARK: Subscriptions

    func subscription() async throws -> SubscriptionUsage {
        try await get("subscriptions/me")
    }

    func subscriptionCatalog() async throws -> [SubscriptionCatalogPlan] {
        try await get("subscriptions/catalog")
    }

    func syncSubscription() async throws -> SubscriptionUsage {
        try await post("subscriptions/sync")
    }

    func subscriptionManagement() async throws -> SubscriptionManagement {
        try await get("subscriptions/management", query: [URLQueryItem(name: "client", value: "ios")])
    }

    // MARK: Settings

    func settings() async throws -> SettingsBundle {
        try await get("settings/me")
    }

    func updateDetectionSettings(_ body: DetectionSettingsUpdate) async throws -> DetectionSettings {
        try await patch("settings/detection", body: body)
    }

    func updateWorkSchedule(_ body: WorkScheduleUpdate) async throws -> WorkSchedule {
        try await patch("settings/work-schedule", body: body)
    }

    func updateNotificationSettings(_ body: NotificationSettingsUpdate) async throws -> NotificationSettings {
        try await patch("settings/notifications", body: body)
    }

    // MARK: Telegram 连接

    func telegramHealth() async throws -> TelegramConnectionHealth {
        try await get("integrations/telegram/health")
    }

    func telegramConnections() async throws -> [TelegramConnectionDTO] {
        try await get("integrations/telegram/connections")
    }

    func setTelegramConnectionEnabled(id: UUID, enabled: Bool) async throws -> TelegramConnectionDTO {
        try await patch("integrations/telegram/connections/\(id.uuidString)", body: ["enabled": enabled])
    }
}
