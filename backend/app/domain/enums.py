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


class ManualReplyDeliveryStatus(StrEnum):
    PENDING = "pending"
    SENDING = "sending"
    DELIVERED = "delivered"
    COMPLETED = "completed"
    FAILED = "failed"
    UNCERTAIN = "uncertain"


class InteractiveAgentApprovalStatus(StrEnum):
    DENIED = "denied"
    GRANTED = "granted"
    EXECUTING = "executing"
    CONSUMED = "consumed"
    FAILED = "failed"
    UNCERTAIN = "uncertain"
    EXPIRED = "expired"


class DevicePlatform(StrEnum):
    IOS = "ios"
    ANDROID = "android"


class DeviceStatus(StrEnum):
    ACTIVE = "active"
    REVOKED = "revoked"


class DeviceCredentialStatus(StrEnum):
    PENDING = "pending"
    ACTIVE = "active"
    ROTATED = "rotated"
    REVOKED = "revoked"
    REUSE_DETECTED = "reuse_detected"


class PushProvider(StrEnum):
    APNS = "apns"
    FCM = "fcm"


class PushEnvironment(StrEnum):
    SANDBOX = "sandbox"
    PRODUCTION = "production"


class PushRegistrationStatus(StrEnum):
    ACTIVE = "active"
    INVALIDATED = "invalidated"
    REVOKED = "revoked"


class SyncAggregateType(StrEnum):
    OPPORTUNITY = "opportunity"
    MESSAGE = "message"
    USER_DETECTION_PREFERENCE = "user_detection_preference"
    USER_WORK_SCHEDULE = "user_work_schedule"
    USER_NOTIFICATION_PREFERENCE = "user_notification_preference"
    REPLY_TEMPLATE = "reply_template"


class SyncOperation(StrEnum):
    UPSERT = "upsert"
    DELETE = "delete"


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


class OpportunityArchiveScope(StrEnum):
    ACTIVE = "active"
    ARCHIVED = "archived"
    ALL = "all"


class OpportunityArchiveAction(StrEnum):
    ARCHIVED = "archived"
    RESTORED = "restored"


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


class AnalysisRunStatus(StrEnum):
    CLAIMED = "claimed"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    EXPIRED = "expired"


class AnalysisRunExecutor(StrEnum):
    DEVICE = "device"
    SERVER = "server"


class AnalysisRunMode(StrEnum):
    PRIMARY = "primary"
    SHADOW = "shadow"


class AnalysisProviderRequestStatus(StrEnum):
    STARTED = "started"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class InteractiveAgentTurnStatus(StrEnum):
    CLAIMED = "claimed"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    EXPIRED = "expired"


class InteractiveAgentProviderRequestStatus(StrEnum):
    STARTED = "started"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


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


class BillingProvider(StrEnum):
    REVENUECAT = "revenuecat"


class BillingStore(StrEnum):
    APP_STORE = "app_store"
    PLAY_STORE = "play_store"
    PADDLE = "paddle"
    TEST_STORE = "test_store"
    UNKNOWN = "unknown"


class BillingInterval(StrEnum):
    MONTHLY = "monthly"
    ANNUAL = "annual"
    UNKNOWN = "unknown"


class BillingSubscriptionStatus(StrEnum):
    TRIALING = "trialing"
    ACTIVE = "active"
    GRACE_PERIOD = "grace_period"
    BILLING_RETRY = "billing_retry"
    CANCELED = "canceled"
    PAUSED = "paused"
    ON_HOLD = "on_hold"
    EXPIRED = "expired"
    REFUNDED = "refunded"
    REVOKED = "revoked"
    INACTIVE = "inactive"
    UNKNOWN = "unknown"


class BillingEventStatus(StrEnum):
    RECEIVED = "received"
    QUEUED = "queued"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"
    ORPHANED = "orphaned"


class UsageFeature(StrEnum):
    PI_AGENT_ANALYSIS = "pi_agent_analysis"
    INTERACTIVE_AGENT_TURN = "interactive_agent_turn"


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


class WeComConnectionType(StrEnum):
    INTERNAL_APP = "internal_app"
    MESSAGE_ARCHIVE = "message_archive"
    CUSTOMER_SERVICE = "customer_service"


class WeComConnectionStatus(StrEnum):
    PENDING = "pending"
    ACTIVE = "active"
    DISABLED = "disabled"
    ERROR = "error"


class WeComSourceType(StrEnum):
    PRIVATE = "private"
    INTERNAL_GROUP = "internal_group"
    EXTERNAL_GROUP = "external_group"
    CUSTOMER_SERVICE = "customer_service"


class WeComReceiveCapability(StrEnum):
    APP_CALLBACK = "app_callback"
    MESSAGE_ARCHIVE = "message_archive"
    CUSTOMER_SERVICE = "customer_service"


class WeComSendCapability(StrEnum):
    APP_MESSAGE = "app_message"
    CUSTOMER_SERVICE = "customer_service"
    MANUAL_ONLY = "manual_only"


class WeComEventStatus(StrEnum):
    RECEIVED = "received"
    QUEUED = "queued"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"
    IGNORED = "ignored"


class WeComDeliveryStatus(StrEnum):
    PENDING = "pending"
    SENDING = "sending"
    SENT = "sent"
    FAILED = "failed"
