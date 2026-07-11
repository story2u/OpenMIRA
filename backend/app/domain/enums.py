from enum import StrEnum


class IMChannel(StrEnum):
    TELEGRAM = "telegram"
    WECOM = "wecom"


class MessageDirection(StrEnum):
    INCOMING = "incoming"
    OUTGOING = "outgoing"


class MessageSource(StrEnum):
    HUMAN = "human"
    AI = "ai"


class OpportunityStatus(StrEnum):
    PENDING_HUMAN = "pending_human"
    AI_AUTO_REPLY = "ai_auto_reply"
    REPLIED = "replied"
    FOLLOWING = "following"
    IGNORED = "ignored"
    CLOSED = "closed"


class FrontendOpportunityStatus(StrEnum):
    PENDING = "pending"
    REPLIED = "replied"
    IGNORED = "ignored"


class Priority(StrEnum):
    LOW = "low"
    NORMAL = "normal"
    HIGH = "high"
    URGENT = "urgent"


class RuleType(StrEnum):
    KEYWORD = "keyword"
    REGEX = "regex"
    AI_HINT = "ai_hint"


class AgentAnalysisStatus(StrEnum):
    NOT_REQUESTED = "not_requested"
    QUOTA_EXCEEDED = "quota_exceeded"
    QUEUED = "queued"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"


class AgentActionType(StrEnum):
    SEND_EMAIL = "send_email"
    ADD_FRIEND = "add_friend"
    PRIVATE_MESSAGE = "private_message"
    NOTIFY_USER = "notify_user"


class LinkSafetyStatus(StrEnum):
    UNVERIFIED = "unverified"
    VERIFYING = "verifying"
    SAFE = "safe"
    SUSPICIOUS = "suspicious"
    MALICIOUS = "malicious"


class PlanCode(StrEnum):
    FREE = "free"
    PLUS = "plus"
    PRO = "pro"
    MAX = "max"


class SubscriptionStatus(StrEnum):
    ACTIVE = "active"
    TRIALING = "trialing"
    PAST_DUE = "past_due"
    CANCELED = "canceled"
    INACTIVE = "inactive"


class UsageFeature(StrEnum):
    PI_AGENT_ANALYSIS = "pi_agent_analysis"


class UsageStatus(StrEnum):
    RESERVED = "reserved"
    CONSUMED = "consumed"
    RELEASED = "released"


class TelegramConnectionType(StrEnum):
    BOT_CHAT = "bot_chat"
    BUSINESS = "business"
    MTPROTO_QR = "mtproto_qr"


class TelegramConnectionStatus(StrEnum):
    PENDING = "pending"
    CONNECTED = "connected"
    DISABLED = "disabled"
    ERROR = "error"
    EXPIRED = "expired"


class TelegramConnectionAttemptStatus(StrEnum):
    PENDING = "pending"
    COMPLETED = "completed"
    CANCELLED = "cancelled"
    EXPIRED = "expired"
    FAILED = "failed"


class TelegramSourceType(StrEnum):
    GROUP = "group"
    CHANNEL = "channel"
    PRIVATE = "private"
