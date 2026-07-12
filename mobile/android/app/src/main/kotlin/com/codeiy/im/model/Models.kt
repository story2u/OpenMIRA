package com.codeiy.im.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * 后端 DTO 镜像（backend/app/application/dto.py），与 iOS `Models.swift` 对齐。
 *
 * 容错枚举：所有枚举属性声明 `= XX.UNKNOWN` 默认值，配合 [RadarJson] 的
 * `coerceInputValues = true`，后端新增枚举值时旧版本 app 解码落到 UNKNOWN 而不是整个
 * 请求失败（对齐 iOS TolerantEnum 语义）。
 */
val RadarJson: Json = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true
    encodeDefaults = true
}

// MARK: 领域枚举（镜像 backend/app/domain/enums.py）

@Serializable
enum class IMChannel(val label: String) {
    @SerialName("telegram") TELEGRAM("Telegram"),
    @SerialName("wecom") WECOM("企业微信"),
    UNKNOWN("未知渠道"),
}

@Serializable
enum class FrontendOpportunityStatus(val label: String) {
    @SerialName("pending") PENDING("待处理"),
    @SerialName("replied") REPLIED("已回复"),
    @SerialName("ignored") IGNORED("已忽略"),
    UNKNOWN("未知"),
}

@Serializable
enum class OpportunityStatus(val label: String) {
    @SerialName("pending_human") PENDING_HUMAN("待人工"),
    @SerialName("ai_auto_reply") AI_AUTO_REPLY("AI 自动回复"),
    @SerialName("replied") REPLIED("已回复"),
    @SerialName("following") FOLLOWING("跟进中"),
    @SerialName("ignored") IGNORED("已忽略"),
    @SerialName("closed") CLOSED("已关闭"),
    UNKNOWN("未知"),
}

@Serializable
enum class Priority(val label: String) {
    @SerialName("low") LOW("低"),
    @SerialName("normal") NORMAL("普通"),
    @SerialName("high") HIGH("高"),
    @SerialName("urgent") URGENT("紧急"),
    UNKNOWN("未知"),
}

@Serializable
enum class MessageSource {
    @SerialName("human") HUMAN,
    @SerialName("ai") AI,
    UNKNOWN,
}

@Serializable
enum class AgentAnalysisStatus(val label: String) {
    @SerialName("not_requested") NOT_REQUESTED("未分析"),
    @SerialName("quota_exceeded") QUOTA_EXCEEDED("额度耗尽"),
    @SerialName("queued") QUEUED("排队中"),
    @SerialName("running") RUNNING("分析中"),
    @SerialName("completed") COMPLETED("已完成"),
    @SerialName("failed") FAILED("失败"),
    UNKNOWN("未知"),
}

@Serializable
enum class AgentActionType(val label: String) {
    @SerialName("send_email") SEND_EMAIL("发送邮件"),
    @SerialName("add_friend") ADD_FRIEND("添加好友"),
    @SerialName("private_message") PRIVATE_MESSAGE("发送私信"),
    @SerialName("notify_user") NOTIFY_USER("内部提醒"),
    UNKNOWN("未知动作"),
}

// MARK: DTO

@Serializable
data class AgentAction(
    val actionType: AgentActionType = AgentActionType.UNKNOWN,
    val reason: String = "",
    val target: String? = null,
    val draft: String? = null,
    val requiresApproval: Boolean = true,
)

/** 列表与详情共用：aiReplyDraft / finalReply / detectionReason 仅详情返回。 */
@Serializable
data class Opportunity(
    val id: String,
    val platform: IMChannel = IMChannel.UNKNOWN,
    val contactName: String = "",
    val contactAvatar: String = "",
    val summary: String = "",
    val matchedKeywords: List<String> = emptyList(),
    val confidenceScore: Double = 0.0,
    val status: FrontendOpportunityStatus = FrontendOpportunityStatus.UNKNOWN,
    val internalStatus: OpportunityStatus = OpportunityStatus.UNKNOWN,
    val priority: Priority = Priority.UNKNOWN,
    val lastMessagePreview: String = "",
    val createdAt: String = "",
    val updatedAt: String = "",
    val sourceType: String = "private",
    val groupName: String? = null,
    val groupMemberRole: String = "member",
    val rawMessageLinks: List<String> = emptyList(),
    val linkVerification: Map<String, JsonElement> = emptyMap(),
    val extractedContacts: Map<String, JsonElement> = emptyMap(),
    val friendRequestStatus: String = "not_sent",
    val sopStage: String = "detected",
    val trustScore: Int = 70,
    val agentActions: List<AgentAction> = emptyList(),
    val agentAnalysisStatus: AgentAnalysisStatus = AgentAnalysisStatus.NOT_REQUESTED,
    val agentAnalysisError: String? = null,
    val agentAnalyzedAt: String? = null,
    val attentionRequired: Boolean = false,
    val aiReplyDraft: String? = null,
    val finalReply: String? = null,
    val detectionReason: String? = null,
)

@Serializable
data class ChatMessage(
    val id: String,
    val senderName: String = "",
    val content: String = "",
    val isFromContact: Boolean = false,
    val sentAt: String = "",
    val source: MessageSource? = null,
)

@Serializable
data class ReplyTemplate(
    val id: String,
    val title: String = "",
    val content: String = "",
    val category: String = "",
)

@Serializable
data class AuthUser(
    val id: String,
    val email: String = "",
    val displayName: String = "",
    val avatarUrl: String = "",
    val isAdmin: Boolean = false,
)

@Serializable
data class AuthToken(
    val accessToken: String,
    val tokenType: String = "bearer",
    val user: AuthUser,
)

@Serializable
data class AIDraft(
    @SerialName("opportunity_id") val opportunityId: String,
    val draft: String,
)

// MARK: 请求体

@Serializable
data class PasswordLoginRequest(val email: String, val password: String)

/** 预留：Google 原生登录（后端 `POST /auth/oauth/google/native` 已就绪）。 */
@Serializable
data class NativeLoginRequest(val idToken: String)

@Serializable
data class ManualReplyRequest(
    val text: String,
    @SerialName("operator_id") val operatorId: String,
    @SerialName("mark_following") val markFollowing: Boolean = true,
)

@Serializable
data class OpportunityStatusUpdate(val status: OpportunityStatus)

// MARK: 展示辅助

/** linkVerification / extractedContacts 是无固定 schema 的 dict，按通用 JSON 渲染。 */
fun JsonElement.displayText(): String = when (this) {
    is JsonNull -> "—"
    is JsonPrimitive -> if (isString) content else when (content) {
        "true" -> "是"
        "false" -> "否"
        else -> content
    }
    is JsonArray -> joinToString("、") { it.displayText() }
    is JsonObject -> entries.sortedBy { it.key }
        .joinToString("；") { "${it.key}: ${it.value.displayText()}" }
}
