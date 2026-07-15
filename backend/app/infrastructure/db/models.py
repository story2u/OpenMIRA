from datetime import datetime, timezone
from typing import Any
from uuid import UUID, uuid4

from sqlalchemy import (
    BigInteger,
    CheckConstraint,
    Column,
    DateTime,
    Index,
    Text,
    UniqueConstraint,
    text,
)
from sqlalchemy import Enum as SAEnum
from sqlalchemy.dialects.postgresql import JSONB
from sqlmodel import Field, SQLModel

from app.domain.enums import (
    AgentAnalysisStatus,
    AutoReplyDecisionReason,
    AutoReplyDeliveryStatus,
    BillingEventStatus,
    BillingInterval,
    BillingProvider,
    BillingStore,
    BillingSubscriptionStatus,
    IMChannel,
    MessageDirection,
    MessageSource,
    OpportunityArchiveAction,
    OpportunityStatus,
    PlanCode,
    Priority,
    RuleType,
    SubscriptionStatus,
    TelegramConnectionAttemptStatus,
    TelegramConnectionStatus,
    TelegramConnectionType,
    TelegramSourceType,
    UsageFeature,
    UsageStatus,
    WeComConnectionStatus,
    WeComConnectionType,
    WeComDeliveryStatus,
    WeComEventStatus,
    WeComReceiveCapability,
    WeComSendCapability,
    WeComSourceType,
)


def utc_now() -> datetime:
    return datetime.now(timezone.utc)


class TimestampMixin(SQLModel):
    created_at: datetime = Field(
        default_factory=utc_now,
        sa_type=DateTime(timezone=True),
        nullable=False,
    )
    updated_at: datetime = Field(
        default_factory=utc_now,
        sa_type=DateTime(timezone=True),
        nullable=False,
    )


class User(TimestampMixin, table=True):
    __tablename__ = "users"

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    email: str = Field(index=True, unique=True)
    display_name: str = Field(default="")
    avatar_url: str = Field(default="")
    password_hash: str | None = None
    auth_version: int = Field(default=0, nullable=False)
    is_active: bool = Field(default=True, index=True)
    is_admin: bool = Field(default=False, index=True)
    last_login_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class PasswordResetChallenge(TimestampMixin, table=True):
    __tablename__ = "password_reset_challenges"
    __table_args__ = (
        CheckConstraint("failed_attempts >= 0", name="ck_password_reset_failed_attempts"),
        Index("ix_password_reset_user_expires", "user_id", "expires_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    token_digest: str = Field(max_length=64, unique=True)
    code_digest: str = Field(max_length=64)
    expires_at: datetime = Field(
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True)
    )
    used_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    failed_attempts: int = Field(default=0, nullable=False)


class AuthAccount(TimestampMixin, table=True):
    __tablename__ = "auth_accounts"
    __table_args__ = (
        UniqueConstraint("provider", "provider_subject", name="uq_auth_accounts_provider_subject"),
        Index("ix_auth_accounts_user_provider", "user_id", "provider"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    provider: str = Field(index=True)
    provider_subject: str = Field(index=True)
    email: str | None = Field(default=None, index=True)


class SubscriptionAccount(TimestampMixin, table=True):
    __tablename__ = "subscription_accounts"
    __table_args__ = (
        UniqueConstraint("user_id", name="uq_subscription_accounts_user_id"),
        Index("ix_subscription_accounts_status_period_end", "status", "current_period_end"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    plan_code: PlanCode = Field(
        default=PlanCode.FREE,
        sa_column=Column(SAEnum(PlanCode, native_enum=False), nullable=False, index=True),
    )
    status: SubscriptionStatus = Field(
        default=SubscriptionStatus.INACTIVE,
        sa_column=Column(SAEnum(SubscriptionStatus, native_enum=False), nullable=False, index=True),
    )
    billing_provider: str | None = Field(default=None, max_length=32)
    effective_store: BillingStore | None = Field(
        default=None,
        sa_column=Column(SAEnum(BillingStore, native_enum=False), nullable=True, index=True),
    )
    billing_interval: BillingInterval | None = Field(
        default=None,
        sa_column=Column(SAEnum(BillingInterval, native_enum=False), nullable=True),
    )
    entitlement_started_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    entitlement_expires_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True, index=True)
    )
    provider_customer_id: str | None = Field(default=None, max_length=255, index=True)
    provider_subscription_id: str | None = Field(default=None, max_length=255, index=True)
    current_period_start: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    current_period_end: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    cancel_at_period_end: bool = Field(default=False)
    will_renew: bool = Field(default=False)
    billing_issue: bool = Field(default=False)
    multiple_active_subscriptions: bool = Field(default=False)
    last_synced_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True, index=True)
    )
    management_url_encrypted: str | None = None
    provider_event_version: str | None = Field(default=None, max_length=255)


class BillingProduct(TimestampMixin, table=True):
    __tablename__ = "billing_products"
    __table_args__ = (
        Index(
            "uq_billing_products_store_product_base_plan",
            "store",
            "external_product_id",
            text("COALESCE(external_base_plan_id, '')"),
            unique=True,
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    plan_code: PlanCode = Field(
        sa_column=Column(SAEnum(PlanCode, native_enum=False), nullable=False, index=True)
    )
    billing_interval: BillingInterval = Field(
        sa_column=Column(SAEnum(BillingInterval, native_enum=False), nullable=False, index=True)
    )
    store: BillingStore = Field(
        sa_column=Column(SAEnum(BillingStore, native_enum=False), nullable=False, index=True)
    )
    external_product_id: str = Field(max_length=255, index=True)
    external_base_plan_id: str | None = Field(default=None, max_length=255)
    revenuecat_entitlement_id: str = Field(max_length=64, index=True)
    revenuecat_package_id: str = Field(max_length=64, index=True)
    active: bool = Field(default=True, index=True)
    metadata_json: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column("metadata", JSONB, nullable=False),
    )


class BillingSubscription(TimestampMixin, table=True):
    __tablename__ = "billing_subscriptions"
    __table_args__ = (
        UniqueConstraint(
            "provider", "external_key", name="uq_billing_subscriptions_provider_external_key"
        ),
        Index(
            "ix_billing_subscriptions_user_status_end", "user_id", "status", "current_period_end"
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    provider: BillingProvider = Field(
        default=BillingProvider.REVENUECAT,
        sa_column=Column(SAEnum(BillingProvider, native_enum=False), nullable=False, index=True),
    )
    store: BillingStore = Field(
        sa_column=Column(SAEnum(BillingStore, native_enum=False), nullable=False, index=True)
    )
    environment: str = Field(default="production", max_length=32, index=True)
    external_key: str = Field(min_length=1, max_length=512)
    external_product_id: str = Field(max_length=255, index=True)
    external_transaction_id: str | None = Field(default=None, max_length=255)
    external_original_transaction_id: str | None = Field(default=None, max_length=255)
    external_subscription_id: str | None = Field(default=None, max_length=255)
    revenuecat_entitlement_id: str | None = Field(default=None, max_length=64, index=True)
    plan_code: PlanCode = Field(
        sa_column=Column(SAEnum(PlanCode, native_enum=False), nullable=False, index=True)
    )
    billing_interval: BillingInterval = Field(
        default=BillingInterval.UNKNOWN,
        sa_column=Column(SAEnum(BillingInterval, native_enum=False), nullable=False),
    )
    status: BillingSubscriptionStatus = Field(
        default=BillingSubscriptionStatus.UNKNOWN,
        sa_column=Column(
            SAEnum(BillingSubscriptionStatus, native_enum=False), nullable=False, index=True
        ),
    )
    current_period_start: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    current_period_end: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    grace_period_end: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    will_renew: bool = Field(default=False)
    cancel_at_period_end: bool = Field(default=False)
    billing_issue_detected_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    last_provider_event_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    last_synced_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )
    metadata_json: dict[str, Any] = Field(
        default_factory=dict, sa_column=Column("metadata", JSONB, nullable=False)
    )


class BillingEvent(TimestampMixin, table=True):
    __tablename__ = "billing_events"
    __table_args__ = (
        UniqueConstraint(
            "provider", "provider_event_id", name="uq_billing_events_provider_event_id"
        ),
        Index("ix_billing_events_status_received", "status", "received_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    provider: BillingProvider = Field(
        default=BillingProvider.REVENUECAT,
        sa_column=Column(SAEnum(BillingProvider, native_enum=False), nullable=False, index=True),
    )
    provider_event_id: str = Field(max_length=255)
    event_type: str = Field(max_length=128, index=True)
    app_user_id: str | None = Field(
        default=None, sa_column=Column(Text(), nullable=True, index=True)
    )
    environment: str | None = Field(default=None, max_length=32)
    payload_hash: str = Field(min_length=64, max_length=64)
    status: BillingEventStatus = Field(
        default=BillingEventStatus.RECEIVED,
        sa_column=Column(SAEnum(BillingEventStatus, native_enum=False), nullable=False, index=True),
    )
    attempt_count: int = Field(default=0, ge=0)
    received_at: datetime = Field(
        default_factory=utc_now, sa_column=Column(DateTime(timezone=True), nullable=False)
    )
    queued_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    processed_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    processing_error: str | None = Field(default=None, max_length=1000)


class Opportunity(TimestampMixin, table=True):
    __tablename__ = "opportunities"
    __table_args__ = (
        Index("ix_opportunities_channel_conversation", "channel", "conversation_id"),
        Index("ix_opportunities_status_created", "status", "created_at"),
        Index(
            "ix_opportunities_owner_archived_last_message",
            "owner_user_id",
            "archived_at",
            "last_message_at",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID | None = Field(default=None, foreign_key="users.id", index=True)
    channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    conversation_id: str = Field(index=True)
    customer_external_id: str | None = Field(default=None, index=True)
    contact_name: str = Field(default="未知联系人")
    contact_avatar: str = Field(default="")
    source_type: str = Field(default="private", index=True)
    group_name: str | None = None

    source_message_id: UUID | None = Field(default=None, index=True, unique=True)
    title: str
    summary: str | None = None
    matched_keywords: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    raw_message_links: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    trust_score: int = Field(default=70, ge=0, le=100)
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    priority: Priority = Field(
        default=Priority.NORMAL,
        sa_column=Column(SAEnum(Priority, native_enum=False), nullable=False, index=True),
    )
    status: OpportunityStatus = Field(
        default=OpportunityStatus.PENDING_HUMAN,
        sa_column=Column(SAEnum(OpportunityStatus, native_enum=False), nullable=False, index=True),
    )
    detection_reason: str | None = None

    link_verification: dict[str, Any] = Field(
        default_factory=lambda: {
            "status": "unverified",
            "verifiedAt": None,
            "riskReasons": [],
            "resolvedInfo": None,
        },
        sa_column=Column(JSONB, nullable=False),
    )
    extracted_contacts: dict[str, Any] = Field(
        default_factory=lambda: {
            "phone": None,
            "email": None,
            "telegramHandle": None,
            "wecomId": None,
            "extractionSource": None,
        },
        sa_column=Column(JSONB, nullable=False),
    )
    friend_request_status: str = Field(default="n/a")
    sop_stage: str = Field(default="detected")
    agent_actions: list[dict[str, Any]] = Field(
        default_factory=list,
        sa_column=Column(JSONB, nullable=False),
    )
    agent_analysis_status: AgentAnalysisStatus = Field(
        default=AgentAnalysisStatus.NOT_REQUESTED,
        sa_column=Column(
            SAEnum(AgentAnalysisStatus, native_enum=False), nullable=False, index=True
        ),
    )
    agent_analysis_error: str | None = None
    agent_analyzed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    attention_required: bool = Field(default=False, index=True)

    ai_reply_draft: str | None = None
    final_reply: str | None = None
    assigned_to: str | None = None
    follow_up_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    last_message_preview: str = Field(default="")
    last_message_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )
    archived_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True, index=True),
    )
    archived_by_user_id: UUID | None = Field(default=None, foreign_key="users.id", index=True)
    archive_reason: str | None = Field(default=None, max_length=500)


class OpportunityArchiveEvent(TimestampMixin, table=True):
    __tablename__ = "opportunity_archive_events"
    __table_args__ = (
        Index("ix_opportunity_archive_events_owner_created", "owner_user_id", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    opportunity_id: UUID = Field(foreign_key="opportunities.id", index=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    action: OpportunityArchiveAction = Field(
        sa_column=Column(
            SAEnum(OpportunityArchiveAction, native_enum=False), nullable=False, index=True
        )
    )
    reason: str | None = Field(default=None, max_length=500)


class AutoReplyDelivery(TimestampMixin, table=True):
    __tablename__ = "auto_reply_deliveries"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id", "idempotency_key", name="uq_auto_reply_deliveries_owner_key"
        ),
        Index(
            "ix_auto_reply_deliveries_conversation_status_created",
            "owner_user_id",
            "channel",
            "conversation_id",
            "status",
            "created_at",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    opportunity_id: UUID = Field(foreign_key="opportunities.id", index=True)
    source_message_id: UUID = Field(foreign_key="messages.id", index=True)
    channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    conversation_id: str = Field(max_length=255, index=True)
    idempotency_key: str = Field(max_length=255)
    status: AutoReplyDeliveryStatus = Field(
        default=AutoReplyDeliveryStatus.CANDIDATE,
        sa_column=Column(
            SAEnum(AutoReplyDeliveryStatus, native_enum=False), nullable=False, index=True
        ),
    )
    decision_reason: AutoReplyDecisionReason | None = Field(
        default=None,
        sa_column=Column(SAEnum(AutoReplyDecisionReason, native_enum=False), nullable=True),
    )
    content_hash: str | None = Field(default=None, max_length=64)
    provider_message_id: str | None = Field(default=None, max_length=255)
    attempt_count: int = Field(default=0, ge=0)
    ready_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    sending_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    sent_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    error: str | None = Field(default=None, max_length=500)


class Message(TimestampMixin, table=True):
    __tablename__ = "messages"
    __table_args__ = (
        UniqueConstraint("channel", "external_message_id", name="uq_message_channel_external"),
        Index("ix_messages_conversation_created", "conversation_id", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID | None = Field(default=None, foreign_key="users.id", index=True)
    channel: IMChannel = Field(
        sa_column=Column(SAEnum(IMChannel, native_enum=False), nullable=False, index=True)
    )
    external_message_id: str = Field(index=True)
    conversation_id: str = Field(index=True)
    sender_external_id: str | None = Field(default=None, index=True)
    sender_display_name: str | None = None
    direction: MessageDirection = Field(
        sa_column=Column(SAEnum(MessageDirection, native_enum=False), nullable=False, index=True)
    )
    source: MessageSource | None = Field(
        default=None,
        sa_column=Column(SAEnum(MessageSource, native_enum=False), nullable=True),
    )
    text: str | None = None
    source_type: str = Field(default="private", index=True)
    group_name: str | None = None
    raw_message_links: list[str] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    raw_payload: dict[str, Any] = Field(
        default_factory=dict, sa_column=Column(JSONB, nullable=False)
    )
    opportunity_id: UUID | None = Field(default=None, foreign_key="opportunities.id", index=True)
    agent_analysis_status: AgentAnalysisStatus = Field(
        default=AgentAnalysisStatus.NOT_REQUESTED,
        sa_column=Column(
            SAEnum(AgentAnalysisStatus, native_enum=False), nullable=False, index=True
        ),
    )
    agent_result: dict[str, Any] = Field(
        default_factory=dict, sa_column=Column(JSONB, nullable=False)
    )
    agent_error: str | None = None
    agent_started_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    agent_analyzed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    sent_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True),
    )
    processed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class UsageLedger(TimestampMixin, table=True):
    __tablename__ = "usage_ledger"
    __table_args__ = (
        UniqueConstraint(
            "user_id",
            "feature",
            "idempotency_key",
            name="uq_usage_ledger_user_feature_idempotency",
        ),
        CheckConstraint("quantity > 0", name="ck_usage_ledger_quantity_positive"),
        Index(
            "ix_usage_ledger_user_feature_period_status",
            "user_id",
            "feature",
            "period_start",
            "period_end",
            "status",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    feature: UsageFeature = Field(
        sa_column=Column(SAEnum(UsageFeature, native_enum=False), nullable=False, index=True)
    )
    quantity: int = Field(default=1, ge=1)
    period_start: datetime = Field(sa_column=Column(DateTime(timezone=True), nullable=False))
    period_end: datetime = Field(sa_column=Column(DateTime(timezone=True), nullable=False))
    idempotency_key: str = Field(max_length=255)
    source_message_id: UUID | None = Field(default=None, foreign_key="messages.id", index=True)
    status: UsageStatus = Field(
        default=UsageStatus.RESERVED,
        sa_column=Column(SAEnum(UsageStatus, native_enum=False), nullable=False, index=True),
    )
    consumed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    released_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    failure_reason: str | None = Field(default=None, max_length=500)


class Rule(TimestampMixin, table=True):
    __tablename__ = "rules"

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    name: str = Field(index=True)
    enabled: bool = Field(default=True, index=True)
    priority: int = Field(default=100, index=True)
    rule_type: RuleType = Field(
        sa_column=Column(SAEnum(RuleType, native_enum=False), nullable=False, index=True)
    )
    pattern: str
    score: float = Field(default=0.5, ge=0.0, le=1.0)
    extra_data: dict[str, Any] = Field(
        default_factory=dict, sa_column=Column(JSONB, nullable=False)
    )


class AppConfig(SQLModel, table=True):
    __tablename__ = "app_configs"

    key: str = Field(primary_key=True)
    value: dict[str, Any] = Field(default_factory=dict, sa_column=Column(JSONB, nullable=False))
    description: str | None = None
    updated_at: datetime = Field(
        default_factory=utc_now,
        sa_column=Column(DateTime(timezone=True), nullable=False),
    )


class UserDetectionPreference(TimestampMixin, table=True):
    """用户级商机识别偏好：自定义关键词 + AI 语义识别开关（叠加在全局规则之上）。"""

    __tablename__ = "user_detection_preferences"
    __table_args__ = (UniqueConstraint("user_id", name="uq_user_detection_preferences_user_id"),)

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    keywords: list[str] = Field(default_factory=list, sa_column=Column(JSONB, nullable=False))
    ai_semantics_enabled: bool = Field(default=True)


class UserWorkSchedule(TimestampMixin, table=True):
    """用户级工作时间：选中时段为人工审核，其余时段可 AI 自动回复；时区为 IANA 标识。"""

    __tablename__ = "user_work_schedules"
    __table_args__ = (UniqueConstraint("user_id", name="uq_user_work_schedules_user_id"),)

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    timezone: str = Field(default="Asia/Shanghai", max_length=64)
    # 每个元素 {"weekday": 1-7, "start": "HH:MM", "end": "HH:MM"}
    slots: list[dict[str, Any]] = Field(
        default_factory=list, sa_column=Column(JSONB, nullable=False)
    )
    auto_reply_outside_hours: bool = Field(default=False)


class UserNotificationPreference(TimestampMixin, table=True):
    """用户级通知偏好；推送通道落地前仅持久化，不代表已生效。"""

    __tablename__ = "user_notification_preferences"
    __table_args__ = (UniqueConstraint("user_id", name="uq_user_notification_preferences_user_id"),)

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    new_opportunity_enabled: bool = Field(default=True)
    ai_replied_enabled: bool = Field(default=True)
    daily_digest_enabled: bool = Field(default=False)
    urgent_only: bool = Field(default=False)


class TelegramUserConfig(TimestampMixin, table=True):
    __tablename__ = "telegram_user_configs"
    __table_args__ = (UniqueConstraint("user_id", name="uq_telegram_user_configs_user_id"),)

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    enabled: bool = Field(default=False, index=True)
    api_id: int | None = Field(default=None)
    api_hash_encrypted: str | None = None
    session_encrypted: str | None = None
    retention_limit: int | None = Field(default=None, ge=0)
    retention_selected_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class TelegramMonitor(TimestampMixin, table=True):
    __tablename__ = "telegram_monitors"
    __table_args__ = (
        UniqueConstraint("user_id", "chat_id", name="uq_telegram_monitors_user_chat"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    telegram_config_id: UUID = Field(foreign_key="telegram_user_configs.id", index=True)
    enabled: bool = Field(default=True, index=True)
    name: str = Field(default="Telegram 群监控")
    chat_id: str = Field(index=True)
    chat_title: str | None = None
    backfill_limit: int = Field(default=30, ge=0, le=500)
    quota_paused: bool = Field(default=False, index=True)
    quota_reason: str | None = Field(default=None, max_length=500)
    retention_priority: int = Field(default=0, ge=0)
    last_error: str | None = None


class TelegramConnection(TimestampMixin, table=True):
    """A user-owned Telegram identity or integration, never a plaintext secret store."""

    __tablename__ = "telegram_connections"
    __table_args__ = (
        UniqueConstraint(
            "provider_connection_id",
            name="uq_telegram_connections_provider_connection_id",
        ),
        Index(
            "ix_telegram_connections_owner_type_status",
            "owner_user_id",
            "connection_type",
            "status",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_type: TelegramConnectionType = Field(
        sa_column=Column(
            SAEnum(TelegramConnectionType, native_enum=False),
            nullable=False,
            index=True,
        )
    )
    status: TelegramConnectionStatus = Field(
        default=TelegramConnectionStatus.PENDING,
        sa_column=Column(
            SAEnum(TelegramConnectionStatus, native_enum=False),
            nullable=False,
            index=True,
        ),
    )
    enabled: bool = Field(default=True, index=True)
    label: str = Field(default="Telegram 连接", max_length=255)
    telegram_account_id: str | None = Field(default=None, max_length=128, index=True)
    provider_connection_id: str | None = Field(default=None, max_length=255, index=True)
    credential_encrypted: str | None = None
    connection_metadata: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column(JSONB, nullable=False),
    )
    capabilities: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column(JSONB, nullable=False),
    )
    last_error: str | None = Field(default=None, max_length=1000)
    last_checked_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )


class TelegramSource(TimestampMixin, table=True):
    """A group, channel, or private conversation selected through a connection."""

    __tablename__ = "telegram_sources"
    __table_args__ = (
        UniqueConstraint(
            "connection_id",
            "external_chat_id",
            name="uq_telegram_sources_connection_chat",
        ),
        Index(
            "ix_telegram_sources_owner_enabled",
            "owner_user_id",
            "enabled",
            "quota_paused",
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_id: UUID = Field(foreign_key="telegram_connections.id", index=True)
    source_type: TelegramSourceType = Field(
        sa_column=Column(
            SAEnum(TelegramSourceType, native_enum=False),
            nullable=False,
            index=True,
        )
    )
    external_chat_id: str = Field(index=True, max_length=128)
    display_name: str = Field(default="Telegram 来源", max_length=255)
    username: str | None = Field(default=None, max_length=255)
    enabled: bool = Field(default=True, index=True)
    auto_reply_enabled: bool = Field(default=False, index=True)
    quota_paused: bool = Field(default=False, index=True)
    quota_reason: str | None = Field(default=None, max_length=500)
    retention_priority: int = Field(default=0, ge=0)
    last_error: str | None = Field(default=None, max_length=1000)


class TelegramConnectionAttempt(TimestampMixin, table=True):
    """A short-lived, owner-bound handshake. The random token itself is never stored."""

    __tablename__ = "telegram_connection_attempts"
    __table_args__ = (
        UniqueConstraint("token_hash", name="uq_telegram_connection_attempts_token_hash"),
        UniqueConstraint(
            "group_request_id",
            name="uq_telegram_connection_attempts_group_request_id",
        ),
        UniqueConstraint(
            "channel_request_id",
            name="uq_telegram_connection_attempts_channel_request_id",
        ),
        Index(
            "ix_telegram_connection_attempts_owner_status_expires",
            "owner_user_id",
            "status",
            "expires_at",
        ),
        Index(
            "uq_telegram_connection_attempts_owner_pending_mtproto_qr",
            "owner_user_id",
            unique=True,
            postgresql_where=text("connection_type = 'MTPROTO_QR' AND status = 'PENDING'"),
        ),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_type: TelegramConnectionType = Field(
        sa_column=Column(
            SAEnum(TelegramConnectionType, native_enum=False),
            nullable=False,
            index=True,
        )
    )
    status: TelegramConnectionAttemptStatus = Field(
        default=TelegramConnectionAttemptStatus.PENDING,
        sa_column=Column(
            SAEnum(TelegramConnectionAttemptStatus, native_enum=False),
            nullable=False,
            index=True,
        ),
    )
    token_hash: str = Field(max_length=128)
    group_request_id: int | None = Field(default=None, index=True)
    channel_request_id: int | None = Field(default=None, index=True)
    telegram_account_id: str | None = Field(default=None, max_length=128, index=True)
    connection_id: UUID | None = Field(
        default=None,
        foreign_key="telegram_connections.id",
        index=True,
    )
    attempt_metadata: dict[str, Any] = Field(
        default_factory=dict,
        sa_column=Column(JSONB, nullable=False),
    )
    # QR URLs are bearer login grants. Keep them encrypted and only reveal them to the owner.
    qr_url_encrypted: str | None = None
    expires_at: datetime = Field(
        sa_column=Column(DateTime(timezone=True), nullable=False, index=True)
    )
    completed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    error: str | None = Field(default=None, max_length=1000)


class TelegramWebhookEvent(TimestampMixin, table=True):
    """Minimal webhook audit and idempotency record; raw payload stays out of persistence."""

    __tablename__ = "telegram_webhook_events"
    __table_args__ = (UniqueConstraint("update_id", name="uq_telegram_webhook_events_update_id"),)

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    update_id: int = Field(sa_column=Column(BigInteger, nullable=False, index=True))
    payload_hash: str = Field(max_length=128)
    event_type: str = Field(default="unknown", max_length=64, index=True)
    connection_id: UUID | None = Field(
        default=None,
        foreign_key="telegram_connections.id",
        index=True,
    )
    processed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    error: str | None = Field(default=None, max_length=1000)


class WeComConnection(TimestampMixin, table=True):
    __tablename__ = "wecom_connections"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id",
            "corp_id",
            "agent_id",
            name="uq_wecom_connections_owner_corp_agent",
        ),
        Index("ix_wecom_connections_owner_status", "owner_user_id", "status"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_type: WeComConnectionType = Field(
        default=WeComConnectionType.INTERNAL_APP,
        sa_column=Column(
            SAEnum(WeComConnectionType, native_enum=False), nullable=False, index=True
        ),
    )
    status: WeComConnectionStatus = Field(
        default=WeComConnectionStatus.PENDING,
        sa_column=Column(
            SAEnum(WeComConnectionStatus, native_enum=False), nullable=False, index=True
        ),
    )
    enabled: bool = Field(default=True, index=True)
    display_name: str = Field(default="企业微信自建应用", max_length=255)
    corp_id: str = Field(max_length=128, index=True)
    agent_id: str = Field(max_length=64)
    secret_encrypted: str
    token_encrypted: str
    aes_key_encrypted: str
    last_verified_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    last_error: str | None = Field(default=None, max_length=1000)


class WeComSource(TimestampMixin, table=True):
    __tablename__ = "wecom_sources"
    __table_args__ = (
        UniqueConstraint(
            "connection_id",
            "external_conversation_id",
            name="uq_wecom_sources_connection_conversation",
        ),
        UniqueConstraint(
            "archive_connection_id",
            "owner_user_id",
            "external_conversation_id",
            name="uq_wecom_sources_archive_owner_conversation",
        ),
        CheckConstraint(
            "(connection_id IS NOT NULL) <> (archive_connection_id IS NOT NULL)",
            name="ck_wecom_sources_exactly_one_connection",
        ),
        Index("ix_wecom_sources_owner_enabled", "owner_user_id", "enabled", "quota_paused"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_id: UUID | None = Field(default=None, foreign_key="wecom_connections.id", index=True)
    archive_connection_id: UUID | None = Field(
        default=None, foreign_key="wecom_archive_connections.id", index=True
    )
    external_conversation_id: str = Field(max_length=255, index=True)
    display_name: str = Field(default="企业微信成员", max_length=255)
    source_type: WeComSourceType = Field(
        default=WeComSourceType.PRIVATE,
        sa_column=Column(SAEnum(WeComSourceType, native_enum=False), nullable=False, index=True),
    )
    receive_capability: WeComReceiveCapability = Field(
        default=WeComReceiveCapability.APP_CALLBACK,
        sa_column=Column(SAEnum(WeComReceiveCapability, native_enum=False), nullable=False),
    )
    send_capability: WeComSendCapability = Field(
        default=WeComSendCapability.APP_MESSAGE,
        sa_column=Column(SAEnum(WeComSendCapability, native_enum=False), nullable=False),
    )
    enabled: bool = Field(default=True, index=True)
    quota_paused: bool = Field(default=False, index=True)
    quota_reason: str | None = Field(default=None, max_length=500)
    last_message_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True, index=True),
    )
    last_error: str | None = Field(default=None, max_length=1000)


class WeComWebhookEvent(TimestampMixin, table=True):
    __tablename__ = "wecom_webhook_events"
    __table_args__ = (
        UniqueConstraint(
            "connection_id",
            "provider_event_id",
            name="uq_wecom_webhook_events_connection_provider",
        ),
        Index("ix_wecom_webhook_events_status_created", "status", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    connection_id: UUID = Field(foreign_key="wecom_connections.id", index=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    provider_event_id: str = Field(max_length=255)
    event_type: str = Field(default="unknown", max_length=64, index=True)
    payload_hash: str = Field(max_length=64)
    normalized_payload_encrypted: str | None = None
    status: WeComEventStatus = Field(
        default=WeComEventStatus.RECEIVED,
        sa_column=Column(SAEnum(WeComEventStatus, native_enum=False), nullable=False, index=True),
    )
    attempt_count: int = Field(default=0, ge=0)
    queued_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    processed_at: datetime | None = Field(
        default=None,
        sa_column=Column(DateTime(timezone=True), nullable=True),
    )
    processing_error: str | None = Field(default=None, max_length=1000)


class WeComOutboundDelivery(TimestampMixin, table=True):
    __tablename__ = "wecom_outbound_deliveries"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id",
            "idempotency_key",
            name="uq_wecom_outbound_deliveries_owner_idempotency",
        ),
        Index("ix_wecom_outbound_deliveries_status_created", "status", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    connection_id: UUID = Field(foreign_key="wecom_connections.id", index=True)
    source_id: UUID = Field(foreign_key="wecom_sources.id", index=True)
    opportunity_id: UUID = Field(foreign_key="opportunities.id", index=True)
    idempotency_key: str = Field(max_length=128)
    content_hash: str = Field(max_length=64)
    status: WeComDeliveryStatus = Field(
        default=WeComDeliveryStatus.PENDING,
        sa_column=Column(
            SAEnum(WeComDeliveryStatus, native_enum=False), nullable=False, index=True
        ),
    )
    provider_message_id: str | None = Field(default=None, max_length=255)
    attempt_count: int = Field(default=0, ge=0)
    sent_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    error: str | None = Field(default=None, max_length=1000)


class WeComArchiveConnection(TimestampMixin, table=True):
    """Enterprise Finance SDK credentials managed by one local installer."""

    __tablename__ = "wecom_archive_connections"
    __table_args__ = (
        UniqueConstraint(
            "owner_user_id", "corp_id", name="uq_wecom_archive_connections_owner_corp"
        ),
        Index("ix_wecom_archive_connections_status_enabled", "status", "enabled"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    owner_user_id: UUID = Field(foreign_key="users.id", index=True)
    display_name: str = Field(default="企业微信会话存档", max_length=255)
    corp_id: str = Field(max_length=128, index=True)
    secret_encrypted: str
    private_key_encrypted: str
    public_key_version: int = Field(ge=1)
    status: WeComConnectionStatus = Field(
        default=WeComConnectionStatus.PENDING,
        sa_column=Column(
            SAEnum(WeComConnectionStatus, native_enum=False), nullable=False, index=True
        ),
    )
    enabled: bool = Field(default=True, index=True)
    last_verified_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    last_polled_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True, index=True)
    )
    last_error: str | None = Field(default=None, max_length=1000)


class WeComArchiveMemberBinding(TimestampMixin, table=True):
    """Maps one local user to the WeCom member whose conversations they may see."""

    __tablename__ = "wecom_archive_member_bindings"
    __table_args__ = (
        UniqueConstraint(
            "connection_id", "user_id", name="uq_wecom_archive_bindings_connection_user"
        ),
        UniqueConstraint(
            "connection_id",
            "wecom_user_id",
            name="uq_wecom_archive_bindings_connection_wecom_user",
        ),
        Index("ix_wecom_archive_bindings_user_enabled", "user_id", "enabled"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    connection_id: UUID = Field(foreign_key="wecom_archive_connections.id", index=True)
    user_id: UUID = Field(foreign_key="users.id", index=True)
    wecom_user_id: str = Field(max_length=128, index=True)
    display_name: str = Field(default="企业微信成员", max_length=255)
    enabled: bool = Field(default=True, index=True)
    last_matched_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )


class WeComArchiveCursor(TimestampMixin, table=True):
    __tablename__ = "wecom_archive_cursors"
    __table_args__ = (
        UniqueConstraint("connection_id", name="uq_wecom_archive_cursors_connection"),
        CheckConstraint("last_seq >= 0", name="ck_wecom_archive_cursors_last_seq_nonnegative"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    connection_id: UUID = Field(foreign_key="wecom_archive_connections.id", index=True)
    last_seq: int = Field(default=0, sa_column=Column(BigInteger, nullable=False))
    lease_expires_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True, index=True)
    )
    last_batch_size: int = Field(default=0, ge=0)


class WeComArchiveEvent(TimestampMixin, table=True):
    """Minimal provider audit. Decrypted message bodies are not stored here."""

    __tablename__ = "wecom_archive_events"
    __table_args__ = (
        UniqueConstraint(
            "connection_id",
            "provider_message_id",
            name="uq_wecom_archive_events_connection_message",
        ),
        Index("ix_wecom_archive_events_status_created", "status", "created_at"),
    )

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    connection_id: UUID = Field(foreign_key="wecom_archive_connections.id", index=True)
    provider_message_id: str = Field(max_length=255)
    sequence: int = Field(sa_column=Column(BigInteger, nullable=False, index=True))
    message_type: str = Field(default="unknown", max_length=64, index=True)
    payload_hash: str = Field(max_length=64)
    status: WeComEventStatus = Field(
        default=WeComEventStatus.RECEIVED,
        sa_column=Column(SAEnum(WeComEventStatus, native_enum=False), nullable=False, index=True),
    )
    matched_user_count: int = Field(default=0, ge=0)
    attempt_count: int = Field(default=0, ge=0)
    processed_at: datetime | None = Field(
        default=None, sa_column=Column(DateTime(timezone=True), nullable=True)
    )
    processing_error: str | None = Field(default=None, max_length=1000)


class ReplyTemplate(TimestampMixin, table=True):
    __tablename__ = "reply_templates"

    id: UUID = Field(default_factory=uuid4, primary_key=True)
    title: str = Field(index=True)
    content: str
    category: str = Field(default="通用", index=True)
    enabled: bool = Field(default=True, index=True)
