import Foundation

// MARK: - 容错枚举
// 后端新增枚举值时旧版本 app 不应整个请求解码失败，统一落到 unknown。

protocol TolerantEnum: Codable, RawRepresentable, Sendable, Hashable where RawValue == String {
    static var unknown: Self { get }
}

extension TolerantEnum {
    init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = Self(rawValue: raw) ?? .unknown
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        try container.encode(rawValue)
    }
}

// MARK: - 领域枚举（镜像 backend/app/domain/enums.py）

enum IMChannel: String, TolerantEnum, CaseIterable {
    case telegram
    case wecom
    case unknown

    var label: String {
        switch self {
        case .telegram: "Telegram"
        case .wecom: "企业微信"
        case .unknown: "未知渠道"
        }
    }
}

enum FrontendOpportunityStatus: String, TolerantEnum, CaseIterable {
    case pending
    case replied
    case ignored
    case unknown

    var label: String {
        switch self {
        case .pending: "待处理"
        case .replied: "已回复"
        case .ignored: "已忽略"
        case .unknown: "未知"
        }
    }
}

enum OpportunityStatus: String, TolerantEnum {
    case pendingHuman = "pending_human"
    case aiAutoReply = "ai_auto_reply"
    case replied
    case following
    case ignored
    case closed
    case unknown

    var label: String {
        switch self {
        case .pendingHuman: "待人工"
        case .aiAutoReply: "AI 自动回复"
        case .replied: "已回复"
        case .following: "跟进中"
        case .ignored: "已忽略"
        case .closed: "已关闭"
        case .unknown: "未知"
        }
    }
}

enum Priority: String, TolerantEnum {
    case low, normal, high, urgent
    case unknown

    var label: String {
        switch self {
        case .low: "低"
        case .normal: "普通"
        case .high: "高"
        case .urgent: "紧急"
        case .unknown: "未知"
        }
    }
}

enum MessageSource: String, TolerantEnum {
    case human, ai
    case unknown
}

enum AgentAnalysisStatus: String, TolerantEnum {
    case notRequested = "not_requested"
    case quotaExceeded = "quota_exceeded"
    case queued, running, completed, failed
    case unknown

    var label: String {
        switch self {
        case .notRequested: "未分析"
        case .quotaExceeded: "额度耗尽"
        case .queued: "排队中"
        case .running: "分析中"
        case .completed: "已完成"
        case .failed: "失败"
        case .unknown: "未知"
        }
    }
}

enum AgentActionType: String, TolerantEnum {
    case sendEmail = "send_email"
    case addFriend = "add_friend"
    case privateMessage = "private_message"
    case notifyUser = "notify_user"
    case unknown

    var label: String {
        switch self {
        case .sendEmail: "发送邮件"
        case .addFriend: "添加好友"
        case .privateMessage: "发送私信"
        case .notifyUser: "内部提醒"
        case .unknown: "未知动作"
        }
    }
}

// MARK: - 通用 JSON 值
// linkVerification / extractedContacts 在后端是无固定 schema 的 dict，按通用 JSON 渲染。

enum JSONValue: Codable, Sendable, Hashable {
    case string(String)
    case number(Double)
    case bool(Bool)
    case null
    case array([JSONValue])
    case object([String: JSONValue])

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() {
            self = .null
        } else if let value = try? container.decode(Bool.self) {
            self = .bool(value)
        } else if let value = try? container.decode(Double.self) {
            self = .number(value)
        } else if let value = try? container.decode(String.self) {
            self = .string(value)
        } else if let value = try? container.decode([JSONValue].self) {
            self = .array(value)
        } else {
            self = .object(try container.decode([String: JSONValue].self))
        }
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .string(let value): try container.encode(value)
        case .number(let value): try container.encode(value)
        case .bool(let value): try container.encode(value)
        case .null: try container.encodeNil()
        case .array(let value): try container.encode(value)
        case .object(let value): try container.encode(value)
        }
    }

    var displayText: String {
        switch self {
        case .string(let value): value
        case .number(let value): value == value.rounded() ? String(Int(value)) : String(value)
        case .bool(let value): value ? "是" : "否"
        case .null: "—"
        case .array(let values): values.map(\.displayText).joined(separator: "、")
        case .object(let dict):
            dict.sorted(by: { $0.key < $1.key })
                .map { "\($0.key): \($0.value.displayText)" }
                .joined(separator: "；")
        }
    }
}

// MARK: - DTO 镜像（backend/app/application/dto.py）

struct AgentAction: Codable, Sendable, Hashable {
    var actionType: AgentActionType
    var reason: String
    var target: String?
    var draft: String?
    var requiresApproval: Bool
}

/// 列表与详情共用：aiReplyDraft / finalReply / detectionReason 仅详情返回。
struct Opportunity: Codable, Sendable, Identifiable, Hashable {
    var id: UUID
    var platform: IMChannel
    var contactName: String
    var contactAvatar: String
    var summary: String
    var matchedKeywords: [String]
    var confidenceScore: Double
    var status: FrontendOpportunityStatus
    var internalStatus: OpportunityStatus
    var priority: Priority
    var lastMessagePreview: String
    var createdAt: Date
    var updatedAt: Date
    var sourceType: String
    var groupName: String?
    var groupMemberRole: String
    var rawMessageLinks: [String]
    var linkVerification: [String: JSONValue]
    var extractedContacts: [String: JSONValue]
    var friendRequestStatus: String
    var sopStage: String
    var trustScore: Int
    var agentActions: [AgentAction]
    var agentAnalysisStatus: AgentAnalysisStatus
    var agentAnalysisError: String?
    var agentAnalyzedAt: Date?
    var attentionRequired: Bool
    var aiReplyDraft: String?
    var finalReply: String?
    var detectionReason: String?
}

struct ChatMessage: Codable, Sendable, Identifiable, Hashable {
    var id: UUID
    var senderName: String
    var content: String
    var isFromContact: Bool
    var sentAt: Date
    var source: MessageSource?
}

struct ReplyTemplate: Codable, Sendable, Identifiable, Hashable {
    var id: UUID
    var title: String
    var content: String
    var category: String
}

struct AuthUser: Codable, Sendable, Identifiable, Hashable {
    var id: UUID
    var email: String
    var displayName: String
    var avatarUrl: String
    var isAdmin: Bool
}

struct AuthToken: Codable, Sendable {
    var accessToken: String
    var tokenType: String
    var user: AuthUser
}

struct AIDraft: Codable, Sendable {
    var opportunityId: UUID
    var draft: String

    enum CodingKeys: String, CodingKey {
        case opportunityId = "opportunity_id"
        case draft
    }
}

// MARK: - 请求体

struct ManualReplyRequest: Encodable, Sendable {
    var text: String
    var operatorId: String
    var markFollowing = true

    enum CodingKeys: String, CodingKey {
        case text
        case operatorId = "operator_id"
        case markFollowing = "mark_following"
    }
}

struct OpportunityStatusUpdate: Encodable, Sendable {
    var status: OpportunityStatus
}

/// 原生登录请求：后端 `POST /auth/oauth/{provider}/native` 待实现（P0 计划步骤 2），
/// 契约以本结构为准。
struct NativeLoginRequest: Encodable, Sendable {
    var idToken: String
}
