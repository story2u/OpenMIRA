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

    // MARK: Templates

    func templates() async throws -> [ReplyTemplate] {
        try await get("templates")
    }
}
