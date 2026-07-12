package com.codeiy.im

import com.codeiy.im.model.AgentActionType
import com.codeiy.im.model.AuthToken
import com.codeiy.im.model.FrontendOpportunityStatus
import com.codeiy.im.model.IMChannel
import com.codeiy.im.model.ManualReplyRequest
import com.codeiy.im.model.Opportunity
import com.codeiy.im.model.OpportunityStatus
import com.codeiy.im.model.Priority
import com.codeiy.im.model.RadarJson
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** DTO 镜像契约检查，与 iOS `ModelsDecodingTests` 同一夹具与断言。 */
class ModelsDecodingTest {
    @Test
    fun opportunityDetailDecoding() {
        val json = """
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
        """.trimIndent()

        val detail = RadarJson.decodeFromString(Opportunity.serializer(), json)

        assertEquals(IMChannel.TELEGRAM, detail.platform)
        assertEquals(FrontendOpportunityStatus.PENDING, detail.status)
        assertEquals(OpportunityStatus.PENDING_HUMAN, detail.internalStatus)
        assertEquals(Priority.HIGH, detail.priority)
        assertEquals(2, detail.agentActions.size)
        assertEquals(
            "未知枚举值必须落到 UNKNOWN 而不是解码失败",
            AgentActionType.UNKNOWN,
            detail.agentActions[1].actionType,
        )
        assertTrue(detail.linkVerification.containsKey("https://example.com"))
        assertEquals("您好，感谢咨询", detail.aiReplyDraft)
        assertTrue(detail.attentionRequired)
    }

    @Test
    fun manualReplyRequestUsesSnakeCase() {
        val body = RadarJson.encodeToString(
            ManualReplyRequest.serializer(),
            ManualReplyRequest(text = "你好", operatorId = "bruce"),
        )
        val obj = RadarJson.parseToJsonElement(body) as JsonObject
        assertEquals("你好", (obj["text"] as JsonPrimitive).content)
        assertEquals("bruce", (obj["operator_id"] as JsonPrimitive).content)
        assertEquals("true", (obj["mark_following"] as JsonPrimitive).content)
    }

    @Test
    fun authTokenDecoding() {
        val json = """
        {"accessToken": "jwt", "tokenType": "bearer",
         "user": {"id": "1b6a4f5e-8f9f-4a1a-9d3e-3a2b1c0d9e8f", "email": "a@b.c",
                  "displayName": "Bruce", "avatarUrl": "", "isAdmin": false}}
        """.trimIndent()
        val token = RadarJson.decodeFromString(AuthToken.serializer(), json)
        assertEquals("jwt", token.accessToken)
        assertEquals("Bruce", token.user.displayName)
    }
}
