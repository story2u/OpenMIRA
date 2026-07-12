import XCTest
@testable import OpportunityRadar

/// 最小可运行检查：DTO 镜像与后端 JSON 形状一致（含分数秒时间、未知枚举容错、snake_case 请求体）。
final class ModelsDecodingTests: XCTestCase {
    func testOpportunityDetailDecoding() throws {
        let json = """
        {
          "id": "0b6a4f5e-8f9f-4a1a-9d3e-3a2b1c0d9e8f",
          "platform": "telegram",
          "contactName": "张三",
          "contactAvatar": "",
          "summary": "想采购一批设备",
          "matchedKeywords": ["采购"],
          "confidenceScore": 0.87,
          "status": "pending",
          "internalStatus": "pending_human",
          "priority": "high",
          "lastMessagePreview": "预算大概 50 万",
          "createdAt": "2026-07-11T08:30:00.123456+00:00",
          "updatedAt": "2026-07-11T08:31:00+00:00",
          "sourceType": "group",
          "groupName": "华东采购群",
          "groupMemberRole": "member",
          "rawMessageLinks": ["https://example.com"],
          "linkVerification": {"https://example.com": {"status": "safe", "hops": 1}},
          "extractedContacts": {"email": "zhang@example.com"},
          "friendRequestStatus": "not_sent",
          "sopStage": "detected",
          "trustScore": 70,
          "agentActions": [
            {"actionType": "send_email", "reason": "对方留了邮箱", "target": "zhang@example.com",
             "draft": "您好", "requiresApproval": true},
            {"actionType": "some_future_action", "reason": "未知类型应容错", "requiresApproval": true}
          ],
          "agentAnalysisStatus": "completed",
          "agentAnalysisError": null,
          "agentAnalyzedAt": "2026-07-11T08:32:10.5+00:00",
          "attentionRequired": true,
          "aiReplyDraft": "您好，感谢咨询",
          "finalReply": null,
          "detectionReason": "命中采购关键词"
        }
        """
        let decoder = APIClient.makeDecoder()
        let detail = try decoder.decode(Opportunity.self, from: Data(json.utf8))

        XCTAssertEqual(detail.platform, .telegram)
        XCTAssertEqual(detail.internalStatus, .pendingHuman)
        XCTAssertEqual(detail.priority, .high)
        XCTAssertEqual(detail.agentActions.count, 2)
        XCTAssertEqual(detail.agentActions[1].actionType, .unknown, "未知枚举值必须落到 unknown 而不是解码失败")
        XCTAssertEqual(detail.linkVerification["https://example.com"]?.displayText.isEmpty, false)
        XCTAssertEqual(detail.aiReplyDraft, "您好，感谢咨询")
        XCTAssertTrue(detail.attentionRequired)
    }

    func testManualReplyRequestUsesSnakeCase() throws {
        let body = ManualReplyRequest(text: "你好", operatorId: "bruce")
        let data = try JSONEncoder().encode(body)
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(object["text"] as? String, "你好")
        XCTAssertEqual(object["operator_id"] as? String, "bruce")
        XCTAssertEqual(object["mark_following"] as? Bool, true)
    }

    func testAuthTokenDecoding() throws {
        let json = """
        {"accessToken": "jwt", "tokenType": "bearer",
         "user": {"id": "1b6a4f5e-8f9f-4a1a-9d3e-3a2b1c0d9e8f", "email": "a@b.c",
                  "displayName": "Bruce", "avatarUrl": "", "isAdmin": false}}
        """
        let token = try JSONDecoder().decode(AuthToken.self, from: Data(json.utf8))
        XCTAssertEqual(token.accessToken, "jwt")
        XCTAssertEqual(token.user.displayName, "Bruce")
    }

    func testPasswordLoginRequestEncoding() throws {
        let request = PasswordLoginRequest(email: "member@example.com", password: "secret")
        let data = try JSONEncoder().encode(request)
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: String])

        XCTAssertEqual(object["email"], "member@example.com")
        XCTAssertEqual(object["password"], "secret")
    }
}
